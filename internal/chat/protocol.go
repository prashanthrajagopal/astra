package chat

import (
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Client frame types
type ClientFrame struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	ID      string `json:"id,omitempty"`
	Token   string `json:"token,omitempty"`
}

// WriteJSON writes v as JSON to the connection.
func WriteJSON(conn *websocket.Conn, v interface{}) error {
	return conn.WriteJSON(v)
}

// WriteSession sends the session frame to the client.
func WriteSession(conn *websocket.Conn, sessionID, agentID uuid.UUID) error {
	return conn.WriteJSON(map[string]string{
		"type":       "session",
		"session_id": sessionID.String(),
		"agent_id":   agentID.String(),
	})
}

// WriteMessageStart sends a message_start frame.
func WriteMessageStart(conn *websocket.Conn, msgID uuid.UUID) error {
	return conn.WriteJSON(map[string]string{
		"type":       "message_start",
		"message_id": msgID.String(),
	})
}

// WriteChunk sends a content chunk.
func WriteChunk(conn *websocket.Conn, msgID uuid.UUID, content string) error {
	return conn.WriteJSON(map[string]string{
		"type":       "chunk",
		"content":    content,
		"message_id": msgID.String(),
	})
}

// WriteMessageEnd sends a message_end frame with token usage.
func WriteMessageEnd(conn *websocket.Conn, msgID uuid.UUID, tokensIn, tokensOut int) error {
	return conn.WriteJSON(map[string]interface{}{
		"type":       "message_end",
		"message_id": msgID.String(),
		"usage":      map[string]int{"tokens_in": tokensIn, "tokens_out": tokensOut},
	})
}

// WriteToolCall sends a tool_call frame.
func WriteToolCall(conn *websocket.Conn, msgID uuid.UUID, callID, name, arguments string) error {
	return conn.WriteJSON(map[string]string{
		"type":       "tool_call",
		"call_id":    callID,
		"name":       name,
		"arguments":  arguments,
		"message_id": msgID.String(),
	})
}

// WriteToolResult sends a tool_result frame.
func WriteToolResult(conn *websocket.Conn, msgID uuid.UUID, callID, result string) error {
	return conn.WriteJSON(map[string]interface{}{
		"type":       "tool_result",
		"call_id":    callID,
		"result":     result,
		"message_id": msgID.String(),
		"done":       true,
	})
}

// WriteDone sends a done frame.
func WriteDone(conn *websocket.Conn, msgID uuid.UUID) error {
	return conn.WriteJSON(map[string]string{
		"type":       "done",
		"message_id": msgID.String(),
	})
}

// WriteError sends an error frame.
func WriteError(conn *websocket.Conn, code, message string, msgID *uuid.UUID) error {
	m := map[string]string{"type": "error", "code": code, "message": message}
	if msgID != nil {
		m["message_id"] = msgID.String()
	}
	return conn.WriteJSON(m)
}

// WritePong sends a pong frame.
func WritePong(conn *websocket.Conn) error {
	return conn.WriteJSON(map[string]string{"type": "pong"})
}
