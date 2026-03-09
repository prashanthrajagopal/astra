# Go Patterns

Project-specific Go conventions for the Astra Autonomous Agent OS. Read this before writing Go code.

## Architecture: Microkernel

```
cmd/<service>/main.go  →  internal/<package>/  →  pkg/<shared>/
                       →  proto/ (generated)   →  migrations/ (SQL)
```

Kernel packages (`actors`, `tasks`, `scheduler`, `messaging`, `events`) are the foundation. Service packages (`agent`, `planner`, `memory`, `workers`, `tools`, `evaluation`) build on kernel primitives.

## Actor Pattern

```go
type Message struct {
    ID        string
    Type      string
    Source    string
    Target    string
    Payload   []byte
    Meta      map[string]string
    Timestamp time.Time
}

type Actor interface {
    ID() string
    Receive(ctx context.Context, msg Message) error
    Stop() error
}
```

### BaseActor with non-blocking mailbox

```go
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

func (a *BaseActor) Receive(ctx context.Context, msg Message) error {
    select {
    case a.mailbox <- msg:
        return nil
    default:
        return ErrMailboxFull
    }
}
```

### Supervision tree

```go
type SupervisorPolicy int
const (
    RestartImmediate SupervisorPolicy = iota
    RestartBackoff
    Escalate
    Terminate
)

type Supervisor struct {
    children map[string]Actor
    policy   SupervisorPolicy
    restarts int
    maxRestarts int
}
```

## Task State Machine

Valid transitions:
```
created → queued → scheduled → running → completed
                                       → failed → dead-letter (after max retries)
```

Transition function (transactional):
```go
func (s *TaskStore) Transition(ctx context.Context, taskID string, from, to Status) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("tasks.Transition: begin: %w", err)
    }
    defer tx.Rollback()

    res, err := tx.ExecContext(ctx,
        `UPDATE tasks SET status=$1, updated_at=now() WHERE id=$2 AND status=$3`,
        to, taskID, from)
    if err != nil {
        return fmt.Errorf("tasks.Transition: update: %w", err)
    }
    if rows, _ := res.RowsAffected(); rows == 0 {
        return ErrInvalidTransition
    }

    _, err = tx.ExecContext(ctx,
        `INSERT INTO events (event_type, actor_id, payload) VALUES ($1, $2, $3)`,
        "Task"+string(to), taskID, payload)
    if err != nil {
        return fmt.Errorf("tasks.Transition: event: %w", err)
    }

    return tx.Commit()
}
```

## Redis Streams Pattern

### Publishing
```go
func (b *Bus) Publish(ctx context.Context, stream string, fields map[string]interface{}) error {
    return b.client.XAdd(ctx, &redis.XAddArgs{
        Stream: stream,
        Values: fields,
    }).Err()
}
```

### Consuming with consumer groups
```go
func (b *Bus) Consume(ctx context.Context, stream, group, consumer string, handler func(redis.XMessage) error) error {
    for {
        msgs, err := b.client.XReadGroup(ctx, &redis.XReadGroupArgs{
            Group:    group,
            Consumer: consumer,
            Streams:  []string{stream, ">"},
            Count:    10,
            Block:    5 * time.Second,
        }).Result()
        if err != nil {
            continue
        }
        for _, msg := range msgs[0].Messages {
            if err := handler(msg); err != nil {
                continue
            }
            b.client.XAck(ctx, stream, group, msg.ID)
        }
    }
}
```

## gRPC Service Pattern

```go
type KernelServer struct {
    pb.UnimplementedKernelServiceServer
    actors  *actors.Runtime
    tasks   *tasks.Store
    bus     *messaging.Bus
}

func (s *KernelServer) SpawnActor(ctx context.Context, req *pb.SpawnActorRequest) (*pb.SpawnActorResponse, error) {
    ctx, span := tracer.Start(ctx, "KernelServer.SpawnActor")
    defer span.End()

    actor := actors.NewBaseActor(uuid.New().String())
    s.actors.Register(actor)

    return &pb.SpawnActorResponse{ActorId: actor.ID()}, nil
}
```

## Error Handling

Always wrap errors with context:
```go
if err != nil {
    return fmt.Errorf("scheduler.findReady: %w", err)
}
```

Never swallow errors:
```go
// WRONG
handler(ctx, msg)

// CORRECT
if err := handler(ctx, msg); err != nil {
    slog.Error("handler failed", "err", err, "msg_id", msg.ID)
}
```

## Structured Logging

```go
slog.Info("task scheduled",
    "task_id", task.ID,
    "graph_id", task.GraphID,
    "shard", shardNum,
)
```

## Context Propagation

Every function doing I/O takes `context.Context` as first parameter:
```go
func (s *Store) Get(ctx context.Context, id string) (*Task, error)
func (b *Bus) Publish(ctx context.Context, stream string, fields map[string]interface{}) error
func (a *Actor) Receive(ctx context.Context, msg Message) error
```

## Testing

Table-driven tests:
```go
func TestTransition(t *testing.T) {
    tests := []struct {
        name    string
        from    Status
        to      Status
        wantErr bool
    }{
        {"pending to queued", Pending, Queued, false},
        {"pending to completed", Pending, Completed, true},
        {"completed to running", Completed, Running, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := store.Transition(ctx, taskID, tt.from, tt.to)
            if (err != nil) != tt.wantErr {
                t.Errorf("got err=%v, wantErr=%v", err, tt.wantErr)
            }
        })
    }
}
```

## Linting

After every change:
```bash
go vet ./...
golangci-lint run <changed_packages>
```
