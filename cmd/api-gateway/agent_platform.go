package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"astra/internal/agentdocs"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func registerAgentPlatformRoutes(mux *http.ServeMux, auth *authMiddleware, store *agentdocs.Store, database *sql.DB, rdb *redis.Client) {
	if store == nil || database == nil {
		return
	}
	mux.Handle("GET /superadmin/api/dashboard/agents/{id}/revisions", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aid, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid agent id")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		list, err := store.ListAgentRevisions(ctx, aid)
		if err != nil {
			slog.Error("ListAgentRevisions", "err", err)
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(map[string]any{"revisions": list})
	})))
	mux.Handle("POST /superadmin/api/dashboard/agents/{id}/revisions", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aid, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid agent id")
			return
		}
		var body struct {
			Payload   json.RawMessage `json:"payload"`
			CreatedBy string          `json:"created_by"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rev, err := store.SaveAgentRevision(ctx, aid, body.Payload, body.CreatedBy)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(rev)
	})))
	mux.Handle("POST /superadmin/api/dashboard/agents/{id}/revisions/{rev}/activate", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aid, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid agent id")
			return
		}
		rev, err := strconv.Atoi(r.PathValue("rev"))
		if err != nil || rev < 1 {
			writeJSONError(w, http.StatusBadRequest, "invalid revision")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := store.ActivateConfigRevision(ctx, aid, rev); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	mux.Handle("PATCH /superadmin/api/dashboard/agents/{id}/platform", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleAgentPlatformPatch(w, r, database, store, rdb)
	})))

	mux.Handle("GET /superadmin/api/dashboard/tools", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, err := database.QueryContext(ctx, `SELECT name, version, risk_tier, sandbox, COALESCE(description,''), metadata FROM tool_definitions ORDER BY name, version`)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		var tools []map[string]any
		for rows.Next() {
			var name, ver, tier, desc string
			var sandbox bool
			var meta []byte
			if err := rows.Scan(&name, &ver, &tier, &sandbox, &desc, &meta); err != nil {
				continue
			}
			tools = append(tools, map[string]any{"name": name, "version": ver, "risk_tier": tier, "sandbox": sandbox, "description": desc, "metadata": json.RawMessage(meta)})
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(map[string]any{"tools": tools})
	})))

	mux.Handle("GET /superadmin/api/dashboard/agents/{id}/events", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleAgentEvents(w, r, database)
	})))

	mux.Handle("GET /superadmin/api/dashboard/audit.ndjson", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleAuditNDJSON(w, r, database)
	})))

	mux.Handle("POST /superadmin/api/dashboard/agents/{id}/forget", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleAgentForget(w, r, database, store)
	})))

	if rdb != nil {
		mux.Handle("GET /superadmin/api/dashboard/llm-saturation", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			maxI, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("ASTRA_LLM_MAX_INFLIGHT")))
			var inflight int64
			if v, err := rdb.Get(ctx, "astra:llm:inflight").Int64(); err == nil {
				inflight = v
			}
			saturated := maxI > 0 && inflight >= int64(maxI)
			w.Header().Set(headerContentType, contentTypeJSON)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"inflight": inflight, "max_inflight": maxI, "saturated": saturated,
			})
		})))
	}
}

func handleAgentPlatformPatch(w http.ResponseWriter, r *http.Request, database *sql.DB, store *agentdocs.Store, _ *redis.Client) {
	aid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	var body struct {
		DrainMode          *bool           `json:"drain_mode"`
		MaxConcurrentGoals *int            `json:"max_concurrent_goals"`
		DailyTokenBudget   *int64          `json:"daily_token_budget"`
		Priority           *int            `json:"priority"`
		AllowedTools       json.RawMessage `json:"allowed_tools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var parts []string
	var vals []interface{}
	i := 1
	if body.DrainMode != nil {
		parts = append(parts, fmt.Sprintf("drain_mode = $%d", i))
		vals = append(vals, *body.DrainMode)
		i++
		if *body.DrainMode {
			parts = append(parts, "drain_started_at = COALESCE(drain_started_at, now())")
		} else {
			parts = append(parts, "drain_started_at = NULL")
		}
	}
	if body.MaxConcurrentGoals != nil {
		parts = append(parts, fmt.Sprintf("max_concurrent_goals = $%d", i))
		vals = append(vals, *body.MaxConcurrentGoals)
		i++
	}
	if body.DailyTokenBudget != nil {
		parts = append(parts, fmt.Sprintf("daily_token_budget = $%d", i))
		vals = append(vals, *body.DailyTokenBudget)
		i++
	}
	if body.Priority != nil {
		parts = append(parts, fmt.Sprintf("priority = $%d", i))
		vals = append(vals, *body.Priority)
		i++
	}
	if body.AllowedTools != nil {
		parts = append(parts, fmt.Sprintf("allowed_tools = $%d::jsonb", i))
		vals = append(vals, body.AllowedTools)
		i++
	}
	if len(parts) == 0 {
		writeJSONError(w, http.StatusBadRequest, "no fields")
		return
	}
	parts = append(parts, "updated_at = now()")
	vals = append(vals, aid)
	q := fmt.Sprintf("UPDATE agents SET %s WHERE id = $%d", strings.Join(parts, ", "), i)
	if _, err := database.ExecContext(ctx, q, vals...); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if store != nil {
		store.InvalidateAgentCache(ctx, aid)
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleAgentEvents(w http.ResponseWriter, r *http.Request, database *sql.DB) {
	aid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	since := r.URL.Query().Get("since")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	q := `
		SELECT e.id, e.event_type, e.actor_id::text, e.payload, e.created_at
		FROM events e
		WHERE EXISTS (SELECT 1 FROM goals g WHERE g.agent_id = $1 AND g.id = e.actor_id)
		   OR EXISTS (SELECT 1 FROM tasks t JOIN goals g ON t.goal_id = g.id WHERE g.agent_id = $1 AND t.id = e.actor_id)
		   OR EXISTS (SELECT 1 FROM phase_runs pr JOIN goals g ON pr.goal_id = g.id WHERE g.agent_id = $1 AND pr.id = e.actor_id)
		   OR (e.payload->>'agent_id') = $2
		   OR EXISTS (SELECT 1 FROM goals g WHERE g.agent_id = $1 AND g.id::text = e.payload->>'goal_id')
	`
	args := []interface{}{aid, aid.String()}
	if since != "" {
		q += " AND e.created_at >= $3::timestamptz"
		args = append(args, since)
	}
	q += " ORDER BY e.created_at DESC LIMIT " + strconv.Itoa(limit)
	rows, err := database.QueryContext(ctx, q, args...)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var evts []map[string]any
	for rows.Next() {
		var id int64
		var etype string
		var actor sql.NullString
		var payload []byte
		var created time.Time
		if err := rows.Scan(&id, &etype, &actor, &payload, &created); err != nil {
			continue
		}
		m := map[string]any{"id": id, "event_type": etype, "created_at": created.UTC().Format(time.RFC3339)}
		if actor.Valid {
			m["actor_id"] = actor.String
		}
		if len(payload) > 0 {
			m["payload"] = json.RawMessage(payload)
		}
		evts = append(evts, m)
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]any{"events": evts})
}

func handleAuditNDJSON(w http.ResponseWriter, r *http.Request, database *sql.DB) {
	since := r.URL.Query().Get("since")
	until := r.URL.Query().Get("until")
	if since == "" {
		since = time.Now().AddDate(0, 0, -7).Format(time.RFC3339)
	}
	if until == "" {
		until = time.Now().Format(time.RFC3339)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	rows, err := database.QueryContext(ctx,
		`SELECT id, event_type, actor_id::text, payload, created_at FROM events WHERE created_at >= $1::timestamptz AND created_at <= $2::timestamptz ORDER BY id`,
		since, until)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	for rows.Next() {
		var id int64
		var etype, actor string
		var payload []byte
		var created time.Time
		if err := rows.Scan(&id, &etype, &actor, &payload, &created); err != nil {
			break
		}
		_ = enc.Encode(map[string]any{
			"id": id, "event_type": etype, "actor_id": actor,
			"payload": json.RawMessage(payload), "created_at": created.UTC().Format(time.RFC3339),
		})
	}
}

func handleAgentForget(w http.ResponseWriter, r *http.Request, database *sql.DB, store *agentdocs.Store) {
	aid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() { _ = tx.Rollback() }()
	_, _ = tx.ExecContext(ctx, `DELETE FROM chat_messages WHERE session_id IN (SELECT id FROM chat_sessions WHERE agent_id = $1)`, aid)
	_, _ = tx.ExecContext(ctx, `DELETE FROM chat_sessions WHERE agent_id = $1`, aid)
	_, _ = tx.ExecContext(ctx, `DELETE FROM memories WHERE agent_id = $1`, aid)
	if err := tx.Commit(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if store != nil {
		store.InvalidateAgentCache(ctx, aid)
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
