package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"astra/internal/messaging"
	"astra/internal/slack"
	"astra/pkg/config"
	"astra/pkg/db"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	streamSlackIncoming = "astra:slack:incoming"
	streamGroup         = "slack-worker"
	consumerName        = "slack-adapter-1"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}
	signingSecret := os.Getenv("SLACK_SIGNING_SECRET")
	if signingSecret == "" {
		// Try from DB (slack_app_config) at startup
		dbConn, _ := db.Connect(cfg.PostgresDSN())
		if dbConn != nil {
			store := slack.NewStore(dbConn)
			signingSecret, _ = store.GetConfig(context.Background(), slack.ConfigKeySigningSecret)
			dbConn.Close()
		}
	}
	if signingSecret == "" {
		slog.Warn("SLACK_SIGNING_SECRET not set; request verification will fail")
	}

	database, err := db.Connect(cfg.PostgresDSN())
	if err != nil {
		slog.Error("database connect failed", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Error("redis connect failed", "err", err)
		os.Exit(1)
	}
	defer rdb.Close()

	bus := messaging.New(cfg.RedisAddr)
	store := slack.NewStore(database)

	gatewayURL := strings.TrimSuffix(getEnv("GATEWAY_INTERNAL_URL", "http://localhost:8080"), "/")
	internalSecret := os.Getenv("ASTRA_SLACK_INTERNAL_SECRET")

	// Worker: consume stream, call gateway, post reply to Slack
	go runWorker(context.Background(), rdb, store, gatewayURL, internalSecret)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /slack/events", handleSlackEvents(signingSecret, store, bus))
	mux.HandleFunc("POST /", handleSlackEvents(signingSecret, store, bus))

	port := getEnv("SLACK_ADAPTER_PORT", "8095")
	srv := &http.Server{Addr: ":" + port, Handler: mux}
	go func() {
		slog.Info("slack-adapter listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func handleSlackEvents(signingSecret string, store *slack.Store, bus *messaging.Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := readBody(r)
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}

		if signingSecret != "" {
			if err := slack.VerifyRequest(signingSecret, string(body), r.Header.Get("X-Slack-Signature"), r.Header.Get("X-Slack-Request-Timestamp")); err != nil {
				slog.Warn("slack verify failed", "err", err)
				http.Error(w, "invalid signature", http.StatusUnauthorized)
				return
			}
		}

		var envelope struct {
			Type      string          `json:"type"`
			Challenge string          `json:"challenge"`
			Event     json.RawMessage `json:"event"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if envelope.Type == "url_verification" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"challenge": envelope.Challenge})
			return
		}

		if envelope.Type != "event_callback" {
			w.WriteHeader(http.StatusOK)
			return
		}

		var evt struct {
			Type      string `json:"type"`
			TeamID    string `json:"team_id"`
			ChannelID string `json:"channel"`
			UserID    string `json:"user"`
			Text      string `json:"text"`
			ThreadTs  string `json:"thread_ts"`
			BotID     string `json:"bot_id"`
		}
		if err := json.Unmarshal(envelope.Event, &evt); err != nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		if evt.BotID != "" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if evt.Type != "message" && evt.Type != "app_mention" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if evt.TeamID == "" || evt.ChannelID == "" || evt.UserID == "" {
			w.WriteHeader(http.StatusOK)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		workspace, err := store.GetWorkspaceBySlackID(ctx, evt.TeamID)
		if err != nil || workspace == nil {
			slog.Debug("slack workspace not linked", "team_id", evt.TeamID)
			w.WriteHeader(http.StatusOK)
			return
		}

		agentID := workspace.DefaultAgentID
		if agentID == nil {
			chAgent, _ := store.GetChannelBinding(ctx, workspace.OrgID, evt.ChannelID)
			if chAgent != nil {
				agentID = chAgent
			}
		}
		if agentID == nil {
			slog.Debug("slack no agent for channel", "channel", evt.ChannelID)
			w.WriteHeader(http.StatusOK)
			return
		}

		userID := "slack:" + evt.UserID
		if astraUser, _ := store.GetUserMapping(ctx, workspace.OrgID, evt.UserID); astraUser != nil {
			userID = astraUser.String()
		}

		sessionID := ""
		if existing, _ := store.GetSlackSessionByThread(ctx, evt.TeamID, evt.ChannelID, evt.UserID, evt.ThreadTs); existing != nil {
			sessionID = existing.String()
		}

		payload := map[string]interface{}{
			"workspace_id":     evt.TeamID,
			"channel_id":      evt.ChannelID,
			"user_id":         evt.UserID,
			"thread_ts":       evt.ThreadTs,
			"text":            evt.Text,
			"org_id":          workspace.OrgID.String(),
			"agent_id":        agentID.String(),
			"astra_user_id":   userID,
			"bot_token_ref":   workspace.BotTokenRef,
			"refresh_token_ref": workspace.RefreshTokenRef,
		}
		if sessionID != "" {
			payload["session_id"] = sessionID
		}
		if err := bus.Publish(ctx, streamSlackIncoming, payload); err != nil {
			slog.Error("slack enqueue failed", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func readBody(r *http.Request) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func runWorker(ctx context.Context, rdb *redis.Client, store *slack.Store, gatewayURL, internalSecret string) {
	client := &http.Client{Timeout: 90 * time.Second}
	for {
		err := rdb.XGroupCreateMkStream(ctx, streamSlackIncoming, streamGroup, "0").Err()
		if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
			slog.Warn("slack worker create group", "err", err)
			time.Sleep(time.Second)
			continue
		}
		streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    streamGroup,
			Consumer: consumerName,
			Streams:  []string{streamSlackIncoming, ">"},
			Count:    5,
			Block:    5 * time.Second,
		}).Result()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		for _, s := range streams {
			for _, msg := range s.Messages {
				if processMessage(ctx, store, client, gatewayURL, internalSecret, msg) == nil {
					rdb.XAck(ctx, streamSlackIncoming, streamGroup, msg.ID)
				}
			}
		}
	}
}

func processMessage(ctx context.Context, store *slack.Store, client *http.Client, gatewayURL, internalSecret string, msg redis.XMessage) error {
	orgID := getStr(msg.Values, "org_id")
	agentID := getStr(msg.Values, "agent_id")
	astraUserID := getStr(msg.Values, "astra_user_id")
	sessionID := getStr(msg.Values, "session_id")
	text := getStr(msg.Values, "text")
	channelID := getStr(msg.Values, "channel_id")
	threadTs := getStr(msg.Values, "thread_ts")
	botTokenRef := getStr(msg.Values, "bot_token_ref")

	if orgID == "" || agentID == "" || astraUserID == "" || text == "" {
		return fmt.Errorf("missing fields")
	}

	reqBody := map[string]interface{}{
		"org_id": orgID, "agent_id": agentID, "user_id": astraUserID, "message": text,
	}
	if sessionID != "" {
		reqBody["session_id"] = sessionID
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gatewayURL+"/internal/slack/chat/message", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Slack-Internal-Secret", internalSecret)
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("slack worker gateway call failed", "err", err)
		return err
	}
	defer resp.Body.Close()
	var result struct {
		SessionID        string `json:"session_id"`
		AssistantContent string `json:"assistant_content"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if resp.StatusCode != http.StatusOK {
		result.AssistantContent = "Sorry, I'm having trouble responding right now."
	}

	// Persist session link for thread continuity
	if result.SessionID != "" {
		orgUUID, _ := uuid.Parse(orgID)
		chatSessionUUID, _ := uuid.Parse(result.SessionID)
		_ = store.CreateSlackSession(ctx, chatSessionUUID, orgUUID, getStr(msg.Values, "workspace_id"), channelID, getStr(msg.Values, "user_id"), threadTs)
	}

	// Post reply to Slack; on 401/token_expired refresh token and retry once
	botToken := botTokenRef
	workspaceID := getStr(msg.Values, "workspace_id")
	refreshTokenRef := getStr(msg.Values, "refresh_token_ref")
	if botToken == "" {
		slog.Warn("slack worker no bot token", "org_id", orgID)
		return nil
	}
	postURL := "https://slack.com/api/chat.postMessage"
	postBody := map[string]interface{}{
		"channel": channelID,
		"text":    result.AssistantContent,
	}
	if threadTs != "" {
		postBody["thread_ts"] = threadTs
	}
	postJSON, _ := json.Marshal(postBody)
	postReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(postJSON))
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Authorization", "Bearer "+botToken)
	postResp, err := client.Do(postReq)
	if err != nil {
		slog.Error("slack worker postMessage failed", "err", err)
		return err
	}
	bodyBytes, _ := io.ReadAll(postResp.Body)
	postResp.Body.Close()
	if postResp.StatusCode == http.StatusOK {
		return nil
	}
	// Token expired or invalid: try refresh and retry once
	if (postResp.StatusCode == http.StatusUnauthorized || strings.Contains(string(bodyBytes), "token_expired") || strings.Contains(string(bodyBytes), "invalid_auth")) && refreshTokenRef != "" && workspaceID != "" {
		newAccess, newRefresh, refErr := slack.RefreshToken(ctx, store, client, workspaceID, refreshTokenRef)
		if refErr != nil {
			slog.Error("slack token refresh failed", "err", refErr)
			return nil
		}
		if newAccess != "" {
			if err := store.UpdateWorkspaceTokens(ctx, workspaceID, newAccess, newRefresh); err != nil {
				slog.Error("slack UpdateWorkspaceTokens failed", "err", err)
			}
			postReq2, _ := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(postJSON))
			postReq2.Header.Set("Content-Type", "application/json")
			postReq2.Header.Set("Authorization", "Bearer "+newAccess)
			postResp2, err2 := client.Do(postReq2)
			if err2 == nil {
				postResp2.Body.Close()
				if postResp2.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
	}
	slog.Error("slack chat.postMessage non-200", "status", postResp.StatusCode, "body", string(bodyBytes))
	return nil
}

func getStr(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
