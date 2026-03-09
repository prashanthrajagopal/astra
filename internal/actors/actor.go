package actors

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

var ErrMailboxFull = fmt.Errorf("actor mailbox full")

type Message struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Source    string            `json:"source"`
	Target    string            `json:"target"`
	Payload   json.RawMessage   `json:"payload"`
	Meta      map[string]string `json:"meta"`
	Timestamp time.Time         `json:"timestamp"`
}

type Actor interface {
	ID() string
	Receive(ctx context.Context, msg Message) error
	Stop() error
}

type BaseActor struct {
	id      string
	mailbox chan Message
	stop    chan struct{}
	wg      sync.WaitGroup
}

func NewBaseActor(id string) *BaseActor {
	return &BaseActor{
		id:      id,
		mailbox: make(chan Message, 1024),
		stop:    make(chan struct{}),
	}
}

func (a *BaseActor) ID() string { return a.id }

func (a *BaseActor) Start(handler func(context.Context, Message) error) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		for {
			select {
			case msg := <-a.mailbox:
				if err := handler(context.Background(), msg); err != nil {
					slog.Error("actor handler error", "actor", a.id, "msg_type", msg.Type, "err", err)
				}
			case <-a.stop:
				return
			}
		}
	}()
}

func (a *BaseActor) Receive(_ context.Context, msg Message) error {
	select {
	case a.mailbox <- msg:
		return nil
	default:
		return ErrMailboxFull
	}
}

func (a *BaseActor) Stop() error {
	close(a.stop)
	a.wg.Wait()
	return nil
}
