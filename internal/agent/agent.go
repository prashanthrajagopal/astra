package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"astra/internal/actors"
	"astra/internal/kernel"
	"astra/internal/planner"
	"astra/internal/tasks"

	"github.com/google/uuid"
)

type Agent struct {
	ID          uuid.UUID
	Name        string
	Status      string
	actor       *actors.BaseActor
	kernel      *kernel.Kernel
	planner     *planner.Planner
	taskStore   *tasks.Store
	db          *sql.DB
	supervisor  *actors.Supervisor
	onTerminate func(agentID string)
}

// AgentOption configures an agent (e.g. supervisor wiring).
type AgentOption func(*Agent)

// WithSupervisor wires the agent to a supervisor: on handler panic or error, HandleFailure is called;
// if the policy is Terminate, onTerminate(agentID) is invoked (e.g. kernel.Stop).
func WithSupervisor(supervisor *actors.Supervisor, onTerminate func(agentID string)) AgentOption {
	return func(a *Agent) {
		a.supervisor = supervisor
		a.onTerminate = onTerminate
	}
}

func New(name string, k *kernel.Kernel, p *planner.Planner, store *tasks.Store, db *sql.DB, opts ...AgentOption) *Agent {
	id := uuid.New()
	return newAgent(id, name, k, p, store, db, opts...)
}

// NewFromExisting builds an agent with an existing ID and name (e.g. from DB on restore).
// The agent is started and spawned into the kernel; no DB insert is performed.
func NewFromExisting(id uuid.UUID, name string, k *kernel.Kernel, p *planner.Planner, store *tasks.Store, db *sql.DB, opts ...AgentOption) *Agent {
	return newAgent(id, name, k, p, store, db, opts...)
}

func newAgent(id uuid.UUID, name string, k *kernel.Kernel, p *planner.Planner, store *tasks.Store, db *sql.DB, opts ...AgentOption) *Agent {
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
	for _, opt := range opts {
		opt(a)
	}
	handler := a.handleMessage
	if a.supervisor != nil {
		a.supervisor.Watch(a.actor)
		handler = a.wrappedHandler()
	}
	a.actor.Start(handler)
	k.Spawn(a.actor)
	return a
}

func (a *Agent) wrappedHandler() func(context.Context, actors.Message) error {
	return func(ctx context.Context, msg actors.Message) error {
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("agent handler panic", "agent_id", a.ID, "panic", r)
					err = fmt.Errorf("panic: %v", r)
				}
			}()
			err = a.handleMessage(ctx, msg)
		}()
		if err != nil && a.supervisor != nil {
			policy := a.supervisor.HandleFailure(a.ID.String())
			if policy == actors.Terminate && a.onTerminate != nil {
				a.onTerminate(a.ID.String())
			}
		}
		return err
	}
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

		graph, err := a.planner.Plan(ctx, goalID, goalText, a.ID, nil)
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
