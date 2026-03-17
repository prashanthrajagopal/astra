package agentdocs

import (
	"context"

	"github.com/google/uuid"
)

// InvalidateAgentCache clears Redis profile/docs cache for an agent.
func (s *Store) InvalidateAgentCache(ctx context.Context, agentID uuid.UUID) {
	if s.rdb == nil {
		return
	}
	_ = s.rdb.Del(ctx, profileKeyPrefix+agentID.String()).Err()
	_ = s.rdb.Del(ctx, docsKeyPrefix+agentID.String()).Err()
}
