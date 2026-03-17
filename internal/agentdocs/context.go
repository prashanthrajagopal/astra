package agentdocs

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/google/uuid"
)

// AgentContext holds the assembled system prompt and documents for an agent.
type AgentContext struct {
	SystemPrompt string     `json:"system_prompt"`
	Rules        []Document `json:"rules"`
	Skills       []Document `json:"skills"`
	ContextDocs  []Document `json:"context_docs"`
}

// AssembleContext builds the full agent context (profile + global and goal-scoped documents).
func (s *Store) AssembleContext(ctx context.Context, agentID uuid.UUID, goalID *uuid.UUID) (*AgentContext, error) {
	profile, err := s.GetProfile(ctx, agentID)
	if err != nil {
		return nil, err
	}
	ac := &AgentContext{}
	if profile != nil {
		ac.SystemPrompt = profile.SystemPrompt
	}

	globalOpts := ListOptions{GlobalOnly: true}
	globalDocs, err := s.ListDocuments(ctx, agentID, globalOpts)
	if err != nil {
		return nil, err
	}

	var goalDocs []Document
	if goalID != nil {
		goalOpts := ListOptions{GoalID: goalID}
		goalDocs, err = s.ListDocuments(ctx, agentID, goalOpts)
		if err != nil {
			return nil, err
		}
	}

	allDocs := append(globalDocs, goalDocs...)

	for _, d := range allDocs {
		switch d.DocType {
		case DocTypeRule:
			ac.Rules = append(ac.Rules, d)
		case DocTypeSkill:
			ac.Skills = append(ac.Skills, d)
		case DocTypeContextDoc, DocTypeReference:
			ac.ContextDocs = append(ac.ContextDocs, d)
		}
	}

	sort.Slice(ac.Rules, func(i, j int) bool {
		if ac.Rules[i].Priority != ac.Rules[j].Priority {
			return ac.Rules[i].Priority < ac.Rules[j].Priority
		}
		return ac.Rules[i].CreatedAt.Before(ac.Rules[j].CreatedAt)
	})
	sort.Slice(ac.Skills, func(i, j int) bool {
		if ac.Skills[i].Priority != ac.Skills[j].Priority {
			return ac.Skills[i].Priority < ac.Skills[j].Priority
		}
		return ac.Skills[i].CreatedAt.Before(ac.Skills[j].CreatedAt)
	})
	sort.Slice(ac.ContextDocs, func(i, j int) bool {
		if ac.ContextDocs[i].Priority != ac.ContextDocs[j].Priority {
			return ac.ContextDocs[i].Priority < ac.ContextDocs[j].Priority
		}
		return ac.ContextDocs[i].CreatedAt.Before(ac.ContextDocs[j].CreatedAt)
	})

	if revPayload, err := s.effectiveRevisionPayload(ctx, agentID); err == nil && len(revPayload) > 0 {
		mergeRevisionIntoContext(ac, revPayload)
	}

	return ac, nil
}

// SerializeContext marshals AgentContext to JSON for inclusion in task payloads.
func SerializeContext(ac *AgentContext) (json.RawMessage, error) {
	if ac == nil {
		return nil, nil
	}
	return json.Marshal(ac)
}
