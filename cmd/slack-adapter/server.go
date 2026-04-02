package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"astra/internal/messaging"
	"astra/internal/slack"
)

type server struct {
	signingSecret string
	store         *slack.Store
	bus           *messaging.Bus
}

func (s *server) handleSlackEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := readBody(r)
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}

	if s.signingSecret != "" {
		if err := slack.VerifyRequest(s.signingSecret, string(body), r.Header.Get("X-Slack-Signature"), r.Header.Get("X-Slack-Request-Timestamp")); err != nil {
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
		_ = json.NewEncoder(w).Encode(map[string]string{"challenge": envelope.Challenge})
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
	workspace, err := s.store.GetWorkspaceBySlackID(ctx, evt.TeamID)
	if err != nil || workspace == nil {
		slog.Debug("slack workspace not linked", "team_id", evt.TeamID)
		w.WriteHeader(http.StatusOK)
		return
	}

	agentID := workspace.DefaultAgentID
	if agentID == nil {
		chAgent, _ := s.store.GetChannelBinding(ctx, evt.ChannelID)
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
	if astraUser, _ := s.store.GetUserMapping(ctx, evt.UserID); astraUser != nil {
		userID = astraUser.String()
	}

	sessionID := ""
	if existing, _ := s.store.GetSlackSessionByThread(ctx, evt.TeamID, evt.ChannelID, evt.UserID, evt.ThreadTs); existing != nil {
		sessionID = existing.String()
	}

	payload := map[string]interface{}{
		"workspace_id":      evt.TeamID,
		"channel_id":        evt.ChannelID,
		"user_id":           evt.UserID,
		"thread_ts":         evt.ThreadTs,
		"text":              evt.Text,
		"agent_id":          agentID.String(),
		"astra_user_id":     userID,
		"bot_token_ref":     workspace.BotTokenRef,
		"refresh_token_ref": workspace.RefreshTokenRef,
	}
	if sessionID != "" {
		payload["session_id"] = sessionID
	}
	if err := s.bus.Publish(ctx, streamSlackIncoming, payload); err != nil {
		slog.Error("slack enqueue failed", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
