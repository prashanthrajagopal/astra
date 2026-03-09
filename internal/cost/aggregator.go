package cost

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type DailyCostRow struct {
	Day         string
	AgentID     sql.NullString
	Model       string
	TokensIn    int64
	TokensOut   int64
	CostDollars float64
}

type Aggregator struct {
	db *sql.DB
}

func NewAggregator(db *sql.DB) *Aggregator {
	return &Aggregator{db: db}
}

func (a *Aggregator) DailyByAgentModel(ctx context.Context, since time.Time) ([]DailyCostRow, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT
			to_char(date_trunc('day', created_at), 'YYYY-MM-DD') AS day,
			agent_id::text,
			model,
			COALESCE(SUM(tokens_in), 0) AS tokens_in,
			COALESCE(SUM(tokens_out), 0) AS tokens_out,
			COALESCE(SUM(cost_dollars), 0) AS cost_dollars
		FROM llm_usage
		WHERE created_at >= $1
		GROUP BY day, agent_id::text, model
		ORDER BY day DESC, cost_dollars DESC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("cost.DailyByAgentModel: query: %w", err)
	}
	defer rows.Close()

	var out []DailyCostRow
	for rows.Next() {
		var r DailyCostRow
		if err := rows.Scan(&r.Day, &r.AgentID, &r.Model, &r.TokensIn, &r.TokensOut, &r.CostDollars); err != nil {
			return nil, fmt.Errorf("cost.DailyByAgentModel: scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cost.DailyByAgentModel: rows: %w", err)
	}
	return out, nil
}
