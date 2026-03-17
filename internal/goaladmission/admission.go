package goaladmission

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// AgentDailyTokenKey returns Redis key for accumulated tokens (UTC calendar day).
func AgentDailyTokenKey(agentID uuid.UUID) string {
	return "agent:" + agentID.String() + ":tokens:" + time.Now().UTC().Format("2006-01-02")
}

var (
	ErrDrainMode     = fmt.Errorf("agent is draining; no new goals accepted")
	ErrConcurrentCap = fmt.Errorf("agent concurrent goal limit reached")
	ErrTokenBudget   = fmt.Errorf("agent daily token budget exceeded")
)

// CheckBeforeNewGoal validates drain mode, concurrent goals, and optional daily token budget.
func CheckBeforeNewGoal(ctx context.Context, db *sql.DB, rdb *redis.Client, agentID uuid.UUID) error {
	var drain bool
	var maxConc sql.NullInt32
	var dailyBudget sql.NullInt64
	err := db.QueryRowContext(ctx,
		`SELECT drain_mode, max_concurrent_goals, daily_token_budget FROM agents WHERE id = $1`,
		agentID).Scan(&drain, &maxConc, &dailyBudget)
	if err != nil {
		return fmt.Errorf("goal admission: %w", err)
	}
	if drain {
		return ErrDrainMode
	}
	if maxConc.Valid && maxConc.Int32 > 0 {
		var n int
		_ = db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM goals WHERE agent_id = $1 AND status IN ('pending','active')`,
			agentID).Scan(&n)
		if n >= int(maxConc.Int32) {
			return ErrConcurrentCap
		}
	}
	if !dailyBudget.Valid || dailyBudget.Int64 <= 0 {
		return nil
	}
	var used int64
	if rdb != nil {
		v, err := rdb.Get(ctx, AgentDailyTokenKey(agentID)).Result()
		if err == nil {
			used, _ = strconv.ParseInt(v, 10, 64)
		} else {
			_ = db.QueryRowContext(ctx, `
				SELECT COALESCE(SUM(tokens_in + tokens_out), 0) FROM llm_usage
				WHERE agent_id = $1 AND created_at >= date_trunc('day', (now() AT TIME ZONE 'UTC'))`,
				agentID).Scan(&used)
		}
	} else {
		_ = db.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(tokens_in + tokens_out), 0) FROM llm_usage
			WHERE agent_id = $1 AND created_at >= date_trunc('day', (now() AT TIME ZONE 'UTC'))`,
			agentID).Scan(&used)
	}
	if used >= dailyBudget.Int64 {
		return ErrTokenBudget
	}
	return nil
}

// IncrAgentDailyTokens adds token count after an LLM completion (llm-router).
func IncrAgentDailyTokens(ctx context.Context, rdb *redis.Client, agentID uuid.UUID, tokens int64) error {
	if rdb == nil || tokens <= 0 || agentID == uuid.Nil {
		return nil
	}
	key := AgentDailyTokenKey(agentID)
	pipe := rdb.TxPipeline()
	pipe.IncrBy(ctx, key, tokens)
	now := time.Now().UTC()
	nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	pipe.Expire(ctx, key, nextMidnight.Sub(now)+72*time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}
