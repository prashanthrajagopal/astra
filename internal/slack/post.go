package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	chatPostMessageURL = "https://slack.com/api/chat.postMessage"
	oauthAccessURL     = "https://slack.com/api/oauth.v2.access"
)

// PostMessage posts a message to Slack (single-platform: uses default workspace). If channelID is empty,
// workspace.NotificationChannelID is used. On 401 or token_expired/invalid_auth, refreshes token and retries once.
func PostMessage(ctx context.Context, store *Store, client *http.Client, channelID, text, threadTs string) error {
	if text == "" {
		return fmt.Errorf("text is required")
	}
	workspace, err := store.GetDefaultWorkspace(ctx)
	if err != nil {
		return fmt.Errorf("get workspace: %w", err)
	}
	if workspace == nil || workspace.BotTokenRef == "" {
		return fmt.Errorf("no Slack workspace or bot token configured")
	}
	if channelID == "" {
		channelID = workspace.NotificationChannelID
	}
	if channelID == "" {
		return fmt.Errorf("channel_id required or set org default notification_channel_id")
	}

	body := map[string]interface{}{
		"channel": channelID,
		"text":    text,
	}
	if threadTs != "" {
		body["thread_ts"] = threadTs
	}
	bodyBytes, _ := json.Marshal(body)

	doPost := func(token string) (int, []byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatPostMessageURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b, nil
	}

	status, respBody, err := doPost(workspace.BotTokenRef)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}

	needRefresh := status == http.StatusUnauthorized ||
		strings.Contains(string(respBody), "token_expired") ||
		strings.Contains(string(respBody), "invalid_auth")
	if needRefresh && workspace.RefreshTokenRef != "" {
		newAccess, newRefresh, refErr := RefreshToken(ctx, store, client, workspace.SlackWorkspaceID, workspace.RefreshTokenRef)
		if refErr != nil {
			return fmt.Errorf("token refresh: %w", refErr)
		}
		if newAccess != "" {
			if err := store.UpdateWorkspaceTokens(ctx, workspace.SlackWorkspaceID, newAccess, newRefresh); err != nil {
				return fmt.Errorf("update tokens: %w", err)
			}
			status2, _, err2 := doPost(newAccess)
			if err2 == nil && status2 == http.StatusOK {
				return nil
			}
		}
	}
	return fmt.Errorf("slack postMessage failed: status=%d body=%s", status, string(respBody))
}

// RefreshToken exchanges a refresh_token for new access and refresh tokens via Slack oauth.v2.access.
func RefreshToken(ctx context.Context, store *Store, client *http.Client, workspaceID, refreshToken string) (newAccess, newRefresh string, err error) {
	clientID, _ := store.GetConfig(ctx, ConfigKeyClientID)
	clientSecret, _ := store.GetConfig(ctx, ConfigKeyClientSecret)
	if clientID == "" || clientSecret == "" {
		return "", "", fmt.Errorf("missing client_id or client_secret in slack config")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthAccessURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var result struct {
		OK           bool   `json:"ok"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}
	if !result.OK || result.AccessToken == "" {
		return "", "", fmt.Errorf("slack refresh failed: %s", result.Error)
	}
	return result.AccessToken, result.RefreshToken, nil
}
