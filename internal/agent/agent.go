package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"astra/internal/actors"
	"astra/internal/kernel"
	"astra/internal/planner"
	"astra/internal/tasks"

	"github.com/google/uuid"
)

type Agent struct {
	ID        uuid.UUID
	Name      string
	Status    string
	actor     *actors.BaseActor
	kernel    *kernel.Kernel
	planner   *planner.Planner
	taskStore *tasks.Store
	db        *sql.DB
}

func New(name string, k *kernel.Kernel, p *planner.Planner, store *tasks.Store, db *sql.DB) *Agent {
	id := uuid.New()
	a := &Agent{
		ID:        id,
		Name:      name,
		Status:    "active",
		actor:     actors.NewBaseActor(id.String()),
		kernel:    k,
		planner:   p,
		taskStore: store,
		db:        db,
	}

	a.actor.Start(a.handleMessage)
	k.Spawn(a.actor)
	return a
}

type createGoalPayload struct {
	GoalText string `json:"goal_text"`
}

func (a *Agent) handleMessage(ctx context.Context, msg actors.Message) error {
	if msg.Type == "CreateGoal" {
		var p createGoalPayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			slog.Error("agent CreateGoal: invalid payload", "agent_id", a.ID, "err", err)
			return err
		}
		goalText := p.GoalText
		if goalText == "" {
			slog.Error("agent CreateGoal: missing goal_text", "agent_id", a.ID)
			return nil
		}

		goalID := uuid.New()
		_, err := a.db.ExecContext(ctx,
			`INSERT INTO goals (id, agent_id, goal_text, priority, status) VALUES ($1, $2, $3, 100, 'active')`,
			goalID, a.ID, goalText)
		if err != nil {
			slog.Error("agent CreateGoal: insert goal failed", "agent_id", a.ID, "err", err)
			return err
		}

		graph, err := a.planner.Plan(ctx, goalID, goalText, a.ID)
		if err != nil {
			slog.Error("agent CreateGoal: Plan failed", "agent_id", a.ID, "err", err)
			return err
		}

		if err := a.taskStore.CreateGraph(ctx, &graph); err != nil {
			slog.Error("agent CreateGoal: CreateGraph failed", "agent_id", a.ID, "err", err)
			return err
		}

		slog.Info("goal created and tasks persisted", "agent_id", a.ID, "goal_id", goalID, "goal_text", goalText, "task_count", len(graph.Tasks))
		return nil
	}

	slog.Info("agent received message", "agent_id", a.ID, "msg_type", msg.Type)
	return nil
}

func (a *Agent) Stop() error {
	a.Status = "stopped"
	return a.kernel.Stop(a.ID.String())
}
