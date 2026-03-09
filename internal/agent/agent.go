package agent

import (
	"context"
	"log/slog"

	"astra/internal/actors"
	"astra/internal/kernel"

	"github.com/google/uuid"
)

type Agent struct {
	ID     uuid.UUID
	Name   string
	Status string
	actor  *actors.BaseActor
	kernel *kernel.Kernel
}

func New(name string, k *kernel.Kernel) *Agent {
	id := uuid.New()
	a := &Agent{
		ID:     id,
		Name:   name,
		Status: "active",
		actor:  actors.NewBaseActor(id.String()),
		kernel: k,
	}

	a.actor.Start(a.handleMessage)
	k.Spawn(a.actor)
	return a
}

func (a *Agent) handleMessage(ctx context.Context, msg actors.Message) error {
	slog.Info("agent received message", "agent_id", a.ID, "msg_type", msg.Type)
	return nil
}

func (a *Agent) Stop() error {
	a.Status = "stopped"
	return a.kernel.Stop(a.ID.String())
}
