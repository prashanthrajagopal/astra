
package actors

import (
    "context"
    "sync"
    "time"
)

type Message struct {
    ID string
    Type string
    Source string
    Target string
    Payload []byte
    Timestamp time.Time
}

type Actor interface {
    ID() string
    Receive(ctx context.Context, msg Message) error
    Stop() error
}

type BaseActor struct {
    id string
    mailbox chan Message
    stop chan struct{}
    wg sync.WaitGroup
}

func NewBaseActor(id string) *BaseActor {
    return &BaseActor{
        id: id,
        mailbox: make(chan Message, 1024),
        stop: make(chan struct{}),
    }
}

func (a *BaseActor) ID() string { return a.id }

func (a *BaseActor) Start(handler func(context.Context, Message) error) {
    a.wg.Add(1)
    go func() {
        defer a.wg.Done()
        ctx := context.Background()
        for {
            select {
            case msg := <-a.mailbox:
                handler(ctx, msg)
            case <-a.stop:
                return
            }
        }
    }()
}

func (a *BaseActor) Receive(ctx context.Context, msg Message) error {
    a.mailbox <- msg
    return nil
}

func (a *BaseActor) Stop() error {
    close(a.stop)
    a.wg.Wait()
    return nil
}
