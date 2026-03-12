package chat

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"astra/internal/llm"
	"astra/internal/memory"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	chunkSize      = 100
	chunkDelayMs   = 30
	identityDefault = "http://localhost:8085"
)

// toolCallPattern matches [TOOL:name(args)] in LLM response.
var toolCallPattern = regexp.MustCompile(`\[TOOL:([^\s\]]+)\s*\(([^)]*)\)\]`)

// TODO(metrics): When Prometheus client is wired in, add metrics:
// - chat_sessions_active (gauge)
// - chat_messages_total (counter)
// - chat_tool_calls_total (counter)

// rateLimitEntry tracks per-session rate limit state.
type rateLimitEntry struct {
	count   int
	resetAt time.Time
}

var sessionRateLimits sync.Map // map[sessionID string]*rateLimitEntry

// checkSessionRateLimit returns (allowed, retrySecs). If allowed is false, retrySecs is seconds until reset.
func checkSessionRateLimit(sessionID string, maxPerMin int) (allowed bool, retrySecs int) {
	now := time.Now()
	windowEnd := now.Add(time.Minute)
	v, _ := sessionRateLimits.LoadOrStore(sessionID, &rateLimitEntry{count: 0, resetAt: windowEnd})
	ent := v.(*rateLimitEntry)
	if now.After(ent.resetAt) {
		ent.count = 0
		ent.resetAt = now.Add(time.Minute)
	}
	if ent.count >= maxPerMin {
		retrySecs = int(time.Until(ent.resetAt).Seconds())
		if retrySecs < 0 {
			retrySecs = 0
		}
		return false, retrySecs
	}
	ent.count++
	return true, 0
}

// processOpts holds options for processMessage.
type processOpts struct {
	maxMsgLength int
	tokenCap     int
	memoryStore  *memory.Store
}

// HandlerConfig configures the WebSocket chat handler.
type HandlerConfig struct {
	MaxMsgLength int
	RateLimit    int // max messages per minute per session
	TokenCap     int // max in+out tokens per session
	MemoryStore  *memory.Store
}

// NewWebSocketHandler returns an HTTP handler that upgrades to WebSocket and runs the chat loop.
// If cfg is nil, defaults are used (RateLimit 30, TokenCap 100000, MemoryStore nil).
func NewWebSocketHandler(chatStore *Store, db *sql.DB, llmBackend *llm.EndpointBackend, cfg *HandlerConfig) http.HandlerFunc {
	maxMsgLength := 65536
	rateLimit := 30
	tokenCap := 100000
	var memoryStore *memory.Store
	if cfg != nil {
		if cfg.MaxMsgLength > 0 {
			maxMsgLength = cfg.MaxMsgLength
		}
		if cfg.RateLimit > 0 {
			rateLimit = cfg.RateLimit
		}
		if cfg.TokenCap > 0 {
			tokenCap = cfg.TokenCap
		}
		memoryStore = cfg.MemoryStore
	}
	identityAddr := os.Getenv("IDENTITY_ADDR")
	if identityAddr == "" {
		identityAddr = identityDefault
	}
	identityAddr = strings.TrimSuffix(identityAddr, "/")

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}

	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Warn("chat ws upgrade failed", "err", err)
			return
		}
		defer conn.Close()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// Extract session_id from query
		sessionIDStr := r.URL.Query().Get("session_id")
		if sessionIDStr == "" {
			_ = WriteError(conn, "invalid_request", "session_id query param required", nil)
			return
		}
		sessionID, err := uuid.Parse(sessionIDStr)
		if err != nil {
			_ = WriteError(conn, "invalid_request", "invalid session_id", nil)
			return
		}

		// Token from query or first "auth" frame
		token := r.URL.Query().Get("token")
		if token == "" {
			conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			var frame ClientFrame
			if err := conn.ReadJSON(&frame); err != nil {
				_ = WriteError(conn, "auth_required", "token required (query or auth frame)", nil)
				return
			}
			conn.SetReadDeadline(time.Time{})
			if frame.Type != "auth" || frame.Token == "" {
				_ = WriteError(conn, "auth_required", "send auth frame with token first", nil)
				return
			}
			token = frame.Token
		}

		// Validate JWT
		subject, err := validateToken(ctx, httpClient, identityAddr, token)
		if err != nil {
			slog.Warn("chat jwt validate failed", "err", err)
			_ = WriteError(conn, "auth_failed", "invalid or expired token", nil)
			return
		}

		// Load session
		se, err := chatStore.GetSession(ctx, sessionID, subject)
		if err != nil {
			slog.Error("chat GetSession failed", "err", err)
			_ = WriteError(conn, "internal_error", "failed to load session", nil)
			return
		}
		if se == nil {
			_ = WriteError(conn, "not_found", "session not found", nil)
			return
		}

		// Check agent chat_capable and get system_prompt
		var chatCapable bool
		var systemPrompt string
		err = db.QueryRowContext(ctx, `SELECT chat_capable, COALESCE(system_prompt,'') FROM agents WHERE id = $1`, se.AgentID).Scan(&chatCapable, &systemPrompt)
		if err == sql.ErrNoRows || !chatCapable {
			_ = WriteError(conn, "forbidden", "agent not chat capable", nil)
			return
		}
		if err != nil {
			slog.Error("chat agent query failed", "err", err)
			_ = WriteError(conn, "internal_error", "failed to load agent", nil)
			return
		}

		// Send session frame
		if err := WriteSession(conn, se.ID, se.AgentID); err != nil {
			slog.Warn("chat WriteSession failed", "err", err)
			return
		}

		slog.Info("chat ws connected", "session_id", se.ID, "agent_id", se.AgentID, "user_id", subject)
		defer func() {
			slog.Info("chat ws disconnected", "session_id", se.ID, "agent_id", se.AgentID, "user_id", subject)
		}()

		// Message loop
		for {
			var frame ClientFrame
			if err := conn.ReadJSON(&frame); err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					return
				}
				slog.Error("chat ws read error", "err", err, "session_id", se.ID)
				return
			}

			switch frame.Type {
			case "ping":
				if err := WritePong(conn); err != nil {
					return
				}
			case "message":
				if ok, retrySecs := checkSessionRateLimit(sessionIDStr, rateLimit); !ok {
					msg := fmt.Sprintf("Too many messages. Try again in %d seconds.", retrySecs)
					_ = WriteError(conn, "rate_limited", msg, nil)
					continue
				}
				processMessage(ctx, conn, chatStore, llmBackend, se, subject, systemPrompt, processOpts{
					maxMsgLength: maxMsgLength,
					tokenCap:    tokenCap,
					memoryStore: memoryStore,
				}, frame.Content, frame.ID)
			default:
				// Ignore unknown frame types
			}
		}
	}
}

func validateToken(ctx context.Context, client *http.Client, identityAddr, token string) (string, error) {
	body, _ := json.Marshal(map[string]string{"token": token})
	req, err := http.NewRequestWithContext(ctx, "POST", identityAddr+"/validate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("identity returned status %d", resp.StatusCode)
	}
	var val struct {
		Valid   bool   `json:"valid"`
		Subject string `json:"subject"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&val); err != nil {
		return "", fmt.Errorf("decode validate response: %w", err)
	}
	if !val.Valid {
		return "", fmt.Errorf("token invalid or expired")
	}
	return val.Subject, nil
}

func processMessage(ctx context.Context, conn *websocket.Conn, chatStore *Store, llmBackend *llm.EndpointBackend, se *Session, subject, systemPrompt string, opts processOpts, content, clientMsgID string) {
	msgID := uuid.New()
	slog.Info("chat message processing", "session_id", se.ID, "message_id", msgID)

	if len(content) > opts.maxMsgLength {
		slog.Error("chat message too long", "session_id", se.ID, "message_id", msgID, "length", len(content))
		_ = WriteError(conn, "invalid_request", "message too long", nil)
		return
	}

	// Token cap check (before persisting user message, check existing session total)
	if opts.tokenCap > 0 {
		total, err := chatStore.GetSessionTokenTotal(ctx, se.ID)
		if err != nil {
			slog.Warn("chat GetSessionTokenTotal failed", "err", err)
		} else if total >= opts.tokenCap {
			_ = WriteError(conn, "token_limit", "Session token limit exceeded. Start a new session.", nil)
			return
		}
	}

	// Persist user message
	_, err := chatStore.AppendMessage(ctx, se.ID, subject, "user", content, nil, nil, 0, 0)
	if err != nil {
		slog.Error("chat AppendMessage user failed", "err", err)
		_ = WriteError(conn, "internal_error", "failed to save message", nil)
		return
	}

	// Load last 20 messages for context
	msgs, err := chatStore.GetMessages(ctx, se.ID, subject)
	if err != nil {
		slog.Error("chat GetMessages failed", "err", err)
		_ = WriteError(conn, "internal_error", "failed to load context", nil)
		return
	}
	lastN := 20
	if len(msgs) > lastN {
		msgs = msgs[len(msgs)-lastN:]
	}

	// Optional memory context: query relevant memories if store is available and message is substantial
	if opts.memoryStore != nil && len(content) > 10 {
		memories, err := opts.memoryStore.Search(ctx, se.AgentID, nil, 3)
		if err != nil {
			slog.Warn("chat memory search failed", "err", err, "agent_id", se.AgentID)
		} else if len(memories) > 0 {
			var memB strings.Builder
			memB.WriteString("[Relevant context from memory]\n")
			for _, m := range memories {
				memB.WriteString("- ")
				memB.WriteString(m.Content)
				memB.WriteString("\n")
			}
			memB.WriteString("\n")
			systemPrompt = memB.String() + systemPrompt
		}
	}

	// Build prompt: system + history + user message
	var b strings.Builder
	if systemPrompt != "" {
		b.WriteString("System: ")
		b.WriteString(systemPrompt)
		b.WriteString("\n\n")
	}
	for _, m := range msgs {
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	prompt := b.String()

	// Call LLM (model hint empty = use default)
	contentResp, tokensIn, tokensOut, err := llmBackend.Complete(ctx, "", prompt)
	if err != nil {
		slog.Error("chat LLM Complete failed", "err", err)
		_ = WriteError(conn, "llm_error", "LLM request failed", nil)
		return
	}

	// Send message_start
	if err := WriteMessageStart(conn, msgID); err != nil {
		return
	}

	// Chunk and send content (skip parts that are tool calls)
	remaining := contentResp
	for len(remaining) > 0 {
		// Check for tool call at current position
		loc := toolCallPattern.FindStringIndex(remaining)
		if loc != nil && loc[0] == 0 {
			// Emit text before tool call (empty if tool call at start)
			fullMatch := remaining[loc[0]:loc[1]]
			subs := toolCallPattern.FindStringSubmatch(fullMatch)
			if len(subs) >= 3 {
				name := subs[1]
				args := subs[2]
				callID := uuid.New().String()
				_ = WriteToolCall(conn, msgID, callID, name, args)
				// For now, emit tool_call without execution; no WorkspaceRuntime in api-gateway
				_ = WriteToolResult(conn, msgID, callID, `{"status":"emitted","note":"tool execution not available in chat"}`)
			}
			remaining = remaining[loc[1]:]
			continue
		}

		chunkLen := chunkSize
		if loc != nil && loc[0] < chunkSize {
			chunkLen = loc[0]
		}
		if chunkLen > len(remaining) {
			chunkLen = len(remaining)
		}
		if chunkLen > 0 {
			chunk := remaining[:chunkLen]
			remaining = remaining[chunkLen:]
			if err := WriteChunk(conn, msgID, chunk); err != nil {
				return
			}
			time.Sleep(chunkDelayMs * time.Millisecond)
		}
	}

	// Send message_end, done
	_ = WriteMessageEnd(conn, msgID, tokensIn, tokensOut)
	_ = WriteDone(conn, msgID)

	// Persist assistant message
	_, err = chatStore.AppendMessage(ctx, se.ID, subject, "assistant", contentResp, nil, nil, tokensIn, tokensOut)
	if err != nil {
		slog.Error("chat AppendMessage assistant failed", "err", err)
	}

	_ = clientMsgID // reserved for request correlation
}
