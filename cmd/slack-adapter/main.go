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

	srvHandler := &server{signingSecret: signingSecret, store: store, bus: bus}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /slack/events", srvHandler.handleSlackEvents)
	mux.HandleFunc("POST /", srvHandler.handleSlackEvents)

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
	agentID := getStr(msg.Values, "agent_id")
	astraUserID := getStr(msg.Values, "astra_user_id")
	sessionID := getStr(msg.Values, "session_id")
	text := getStr(msg.Values, "text")
	channelID := getStr(msg.Values, "channel_id")
	threadTs := getStr(msg.Values, "thread_ts")
	botTokenRef := getStr(msg.Values, "bot_token_ref")

	if agentID == "" || astraUserID == "" || text == "" {
		return fmt.Errorf("missing fields")
	}

	reqBody := map[string]interface{}{
		"agent_id": agentID, "user_id": astraUserID, "message": text,
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
		chatSessionUUID, _ := uuid.Parse(result.SessionID)
		_ = store.CreateSlackSession(ctx, chatSessionUUID, getStr(msg.Values, "workspace_id"), channelID, getStr(msg.Values, "user_id"), threadTs)
	}

	// Post reply to Slack; on 401/token_expired refresh token and retry once
	botToken := botTokenRef
	workspaceID := getStr(msg.Values, "workspace_id")
	refreshTokenRef := getStr(msg.Values, "refresh_token_ref")
	if botToken == "" {
		slog.Warn("slack worker no bot token")
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
