package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// newTestWSPair creates a connected WebSocket client/server pair for testing.
func newTestWSPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	var serverConn *websocket.Conn
	ready := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		serverConn, err = upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("server upgrade: %v", err)
			return
		}
		close(ready)
		// keep alive until client closes
		for {
			if _, _, err := serverConn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	clientConn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	t.Cleanup(func() { clientConn.Close() })

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server connection")
	}
	t.Cleanup(func() { serverConn.Close() })

	return serverConn, clientConn
}

func readJSON(t *testing.T, conn *websocket.Conn) map[string]interface{} {
	t.Helper()
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(msg, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return out
}

func TestClientFrameJSONFields(t *testing.T) {
	tests := []struct {
		name  string
		frame ClientFrame
	}{
		{"message type", ClientFrame{Type: "message", Content: "hello"}},
		{"auth type", ClientFrame{Type: "auth", Token: "tok123"}},
		{"ping type", ClientFrame{Type: "ping"}},
		{"with id", ClientFrame{Type: "message", Content: "hi", ID: "req-1"}},
		{"empty frame", ClientFrame{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.frame)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			var got ClientFrame
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if got.Type != tc.frame.Type {
				t.Errorf("Type: got %q, want %q", got.Type, tc.frame.Type)
			}
			if got.Content != tc.frame.Content {
				t.Errorf("Content: got %q, want %q", got.Content, tc.frame.Content)
			}
			if got.Token != tc.frame.Token {
				t.Errorf("Token: got %q, want %q", got.Token, tc.frame.Token)
			}
			if got.ID != tc.frame.ID {
				t.Errorf("ID: got %q, want %q", got.ID, tc.frame.ID)
			}
		})
	}
}

func TestWriteSession(t *testing.T) {
	serverConn, clientConn := newTestWSPair(t)
	sessionID := uuid.New()
	agentID := uuid.New()

	if err := WriteSession(serverConn, sessionID, agentID); err != nil {
		t.Fatalf("WriteSession: %v", err)
	}

	msg := readJSON(t, clientConn)
	if msg["type"] != "session" {
		t.Errorf("type: got %v, want session", msg["type"])
	}
	if msg["session_id"] != sessionID.String() {
		t.Errorf("session_id: got %v, want %v", msg["session_id"], sessionID.String())
	}
	if msg["agent_id"] != agentID.String() {
		t.Errorf("agent_id: got %v, want %v", msg["agent_id"], agentID.String())
	}
}

func TestWriteMessageStart(t *testing.T) {
	serverConn, clientConn := newTestWSPair(t)
	msgID := uuid.New()

	if err := WriteMessageStart(serverConn, msgID); err != nil {
		t.Fatalf("WriteMessageStart: %v", err)
	}

	msg := readJSON(t, clientConn)
	if msg["type"] != "message_start" {
		t.Errorf("type: got %v, want message_start", msg["type"])
	}
	if msg["message_id"] != msgID.String() {
		t.Errorf("message_id: got %v, want %v", msg["message_id"], msgID.String())
	}
}

func TestWriteChunk(t *testing.T) {
	serverConn, clientConn := newTestWSPair(t)
	msgID := uuid.New()

	if err := WriteChunk(serverConn, msgID, "hello world"); err != nil {
		t.Fatalf("WriteChunk: %v", err)
	}

	msg := readJSON(t, clientConn)
	if msg["type"] != "chunk" {
		t.Errorf("type: got %v, want chunk", msg["type"])
	}
	if msg["content"] != "hello world" {
		t.Errorf("content: got %v, want hello world", msg["content"])
	}
	if msg["message_id"] != msgID.String() {
		t.Errorf("message_id: got %v, want %v", msg["message_id"], msgID.String())
	}
}

func TestWriteMessageEnd(t *testing.T) {
	serverConn, clientConn := newTestWSPair(t)
	msgID := uuid.New()

	if err := WriteMessageEnd(serverConn, msgID, 100, 200); err != nil {
		t.Fatalf("WriteMessageEnd: %v", err)
	}

	msg := readJSON(t, clientConn)
	if msg["type"] != "message_end" {
		t.Errorf("type: got %v, want message_end", msg["type"])
	}
	usage, ok := msg["usage"].(map[string]interface{})
	if !ok {
		t.Fatalf("usage field missing or wrong type: %v", msg["usage"])
	}
	if usage["tokens_in"] != float64(100) {
		t.Errorf("tokens_in: got %v, want 100", usage["tokens_in"])
	}
	if usage["tokens_out"] != float64(200) {
		t.Errorf("tokens_out: got %v, want 200", usage["tokens_out"])
	}
}

func TestWriteToolCall(t *testing.T) {
	serverConn, clientConn := newTestWSPair(t)
	msgID := uuid.New()

	if err := WriteToolCall(serverConn, msgID, "call-1", "file_write", `{"path":"x.go"}`); err != nil {
		t.Fatalf("WriteToolCall: %v", err)
	}

	msg := readJSON(t, clientConn)
	if msg["type"] != "tool_call" {
		t.Errorf("type: got %v, want tool_call", msg["type"])
	}
	if msg["call_id"] != "call-1" {
		t.Errorf("call_id: got %v, want call-1", msg["call_id"])
	}
	if msg["name"] != "file_write" {
		t.Errorf("name: got %v, want file_write", msg["name"])
	}
	if msg["arguments"] != `{"path":"x.go"}` {
		t.Errorf("arguments: got %v", msg["arguments"])
	}
}

func TestWriteToolResult(t *testing.T) {
	serverConn, clientConn := newTestWSPair(t)
	msgID := uuid.New()

	if err := WriteToolResult(serverConn, msgID, "call-1", "ok"); err != nil {
		t.Fatalf("WriteToolResult: %v", err)
	}

	msg := readJSON(t, clientConn)
	if msg["type"] != "tool_result" {
		t.Errorf("type: got %v, want tool_result", msg["type"])
	}
	if msg["call_id"] != "call-1" {
		t.Errorf("call_id: got %v, want call-1", msg["call_id"])
	}
	if msg["result"] != "ok" {
		t.Errorf("result: got %v, want ok", msg["result"])
	}
	if msg["done"] != true {
		t.Errorf("done: got %v, want true", msg["done"])
	}
}

func TestWriteDone(t *testing.T) {
	serverConn, clientConn := newTestWSPair(t)
	msgID := uuid.New()

	if err := WriteDone(serverConn, msgID); err != nil {
		t.Fatalf("WriteDone: %v", err)
	}

	msg := readJSON(t, clientConn)
	if msg["type"] != "done" {
		t.Errorf("type: got %v, want done", msg["type"])
	}
	if msg["message_id"] != msgID.String() {
		t.Errorf("message_id: got %v, want %v", msg["message_id"], msgID.String())
	}
}

func TestWriteError(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		message string
		msgID   *uuid.UUID
	}{
		{"with message_id", "auth_failed", "bad token", func() *uuid.UUID { id := uuid.New(); return &id }()},
		{"without message_id", "internal_error", "oops", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			serverConn, clientConn := newTestWSPair(t)

			if err := WriteError(serverConn, tc.code, tc.message, tc.msgID); err != nil {
				t.Fatalf("WriteError: %v", err)
			}

			msg := readJSON(t, clientConn)
			if msg["type"] != "error" {
				t.Errorf("type: got %v, want error", msg["type"])
			}
			if msg["code"] != tc.code {
				t.Errorf("code: got %v, want %v", msg["code"], tc.code)
			}
			if msg["message"] != tc.message {
				t.Errorf("message: got %v, want %v", msg["message"], tc.message)
			}
			if tc.msgID != nil {
				if msg["message_id"] != tc.msgID.String() {
					t.Errorf("message_id: got %v, want %v", msg["message_id"], tc.msgID.String())
				}
			} else {
				if _, ok := msg["message_id"]; ok {
					t.Error("message_id should be absent when nil")
				}
			}
		})
	}
}

func TestWritePong(t *testing.T) {
	serverConn, clientConn := newTestWSPair(t)

	if err := WritePong(serverConn); err != nil {
		t.Fatalf("WritePong: %v", err)
	}

	msg := readJSON(t, clientConn)
	if msg["type"] != "pong" {
		t.Errorf("type: got %v, want pong", msg["type"])
	}
}

func TestWriteJSON(t *testing.T) {
	serverConn, clientConn := newTestWSPair(t)

	payload := map[string]string{"hello": "world"}
	if err := WriteJSON(serverConn, payload); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	msg := readJSON(t, clientConn)
	if msg["hello"] != "world" {
		t.Errorf("hello: got %v, want world", msg["hello"])
	}
}

func TestNeedsGoalWorkflow(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{"short message", "hi", false},
		{"simple question", "what is the capital of France?", false},
		{"create action", "create a REST API endpoint for users", true},
		{"build action", "build me a web app", true},
		{"write action", "write a python script to parse CSV", true},
		{"generate action", "generate a dockerfile for my app", true},
		{"implement action", "implement a Go program to read files", true},
		{"tech keyword tsx", "make a component that shows a dashboard.tsx", true},
		{"file extension py", "show me how main.py should look", true},
		{"file extension go", "here is main.go for reference", true},
		{"docker keyword", "i need a docker setup", true},
		{"sql keyword", "create a database schema for users", true},
		{"scaffold keyword", "scaffold a new react app", true},
		{"empty string", "", false},
		{"exactly 14 chars", "123456789012 x", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NeedsGoalWorkflow(tc.message)
			if got != tc.want {
				t.Errorf("NeedsGoalWorkflow(%q) = %v, want %v", tc.message, got, tc.want)
			}
		})
	}
}
