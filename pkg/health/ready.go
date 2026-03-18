package health

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// ReadyHandler returns an http.HandlerFunc for GET /ready that returns 200 when
// dependencies (db, redis) are reachable and 503 otherwise. If db or rdb is nil
// that check is skipped. Use READINESS_CHECKS=db,redis (default both) to enable.
// DBPinger is satisfied by *sql.DB.
type DBPinger interface {
	PingContext(ctx context.Context) error
}

func ReadyHandler(db DBPinger, rdb *redis.Client) http.HandlerFunc {
	checks := os.Getenv("READINESS_CHECKS")
	if checks == "" {
		checks = "db,redis"
	}
	checkDB := strings.Contains(checks, "db") && db != nil
	checkRedis := strings.Contains(checks, "redis") && rdb != nil

	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		var reason string
		if checkDB {
			if err := db.PingContext(ctx); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"ready":  false,
					"reason": "db: " + err.Error(),
				})
				return
			}
		}
		if checkRedis {
			if err := rdb.Ping(ctx).Err(); err != nil {
				reason = "redis: " + err.Error()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"ready":  false,
					"reason": reason,
				})
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ready":true}`))
	}
}
