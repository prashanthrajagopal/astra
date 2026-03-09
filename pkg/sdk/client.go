package sdk

import (
	"context"
	"fmt"
	"time"

	kernelpb "astra/proto/kernel"
	memorypb "astra/proto/memory"
	taskspb "astra/proto/tasks"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Config struct {
	AgentID             string
	KernelGRPCAddr      string
	TaskGRPCAddr        string
	MemoryGRPCAddr      string
	ToolRuntimeHTTPAddr string
	RequestTimeout      time.Duration
	ActorType           string
}

type DefaultAgentContext struct {
	agentID      string
	kernelConn   *grpc.ClientConn
	taskConn     *grpc.ClientConn
	memoryConn   *grpc.ClientConn
	kernelClient kernelpb.KernelServiceClient
	taskClient   taskspb.TaskServiceClient
	memoryClient MemoryClient
	toolClient   ToolClient
	timeout      time.Duration
}

func DefaultConfig() Config {
	return Config{
		KernelGRPCAddr:      "localhost:9091",
		TaskGRPCAddr:        "localhost:9090",
		MemoryGRPCAddr:      "localhost:9092",
		ToolRuntimeHTTPAddr: "http://localhost:8083",
		RequestTimeout:      5 * time.Second,
		ActorType:           "sdk-agent",
	}
}

func NewAgentContext(cfg Config) (*DefaultAgentContext, error) {
	if cfg.KernelGRPCAddr == "" {
		cfg.KernelGRPCAddr = DefaultConfig().KernelGRPCAddr
	}
	if cfg.TaskGRPCAddr == "" {
		cfg.TaskGRPCAddr = DefaultConfig().TaskGRPCAddr
	}
	if cfg.MemoryGRPCAddr == "" {
		cfg.MemoryGRPCAddr = DefaultConfig().MemoryGRPCAddr
	}
	if cfg.ToolRuntimeHTTPAddr == "" {
		cfg.ToolRuntimeHTTPAddr = DefaultConfig().ToolRuntimeHTTPAddr
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = DefaultConfig().RequestTimeout
	}
	if cfg.ActorType == "" {
		cfg.ActorType = DefaultConfig().ActorType
	}

	kernelConn, err := grpc.NewClient(cfg.KernelGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("sdk.NewAgentContext kernel dial: %w", err)
	}
	taskConn, err := grpc.NewClient(cfg.TaskGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		kernelConn.Close()
		return nil, fmt.Errorf("sdk.NewAgentContext task dial: %w", err)
	}
	memoryConn, err := grpc.NewClient(cfg.MemoryGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		kernelConn.Close()
		taskConn.Close()
		return nil, fmt.Errorf("sdk.NewAgentContext memory dial: %w", err)
	}

	ctx := &DefaultAgentContext{
		agentID:      cfg.AgentID,
		kernelConn:   kernelConn,
		taskConn:     taskConn,
		memoryConn:   memoryConn,
		kernelClient: kernelpb.NewKernelServiceClient(kernelConn),
		taskClient:   taskspb.NewTaskServiceClient(taskConn),
		memoryClient: newMemoryClient(memorypb.NewMemoryServiceClient(memoryConn)),
		toolClient:   newToolClient(cfg.ToolRuntimeHTTPAddr, cfg.RequestTimeout),
		timeout:      cfg.RequestTimeout,
	}

	if ctx.agentID == "" {
		cctx, cancel := context.WithTimeout(context.Background(), cfg.RequestTimeout)
		defer cancel()
		resp, err := ctx.kernelClient.SpawnActor(cctx, &kernelpb.SpawnActorRequest{ActorType: cfg.ActorType, Config: []byte("{}")})
		if err != nil {
			ctx.Close()
			return nil, fmt.Errorf("sdk.NewAgentContext spawn actor: %w", err)
		}
		ctx.agentID = resp.GetActorId()
	}

	return ctx, nil
}

func (c *DefaultAgentContext) Close() error {
	var firstErr error
	if c.memoryConn != nil {
		if err := c.memoryConn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if c.taskConn != nil {
		if err := c.taskConn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if c.kernelConn != nil {
		if err := c.kernelConn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (c *DefaultAgentContext) ID() string {
	return c.agentID
}

func (c *DefaultAgentContext) Memory() MemoryClient {
	return c.memoryClient
}

func (c *DefaultAgentContext) CreateTask(ctx context.Context, task TaskSpec) (string, error) {
	graphID := task.GraphID
	if graphID == "" {
		graphID = uuid.New().String()
	}
	if task.Type == "" {
		task.Type = "sdk-task"
	}
	resp, err := c.taskClient.CreateTask(ctx, &taskspb.CreateTaskRequest{
		GraphId:   graphID,
		AgentId:   c.agentID,
		Type:      task.Type,
		Payload:   task.Payload,
		Priority:  task.Priority,
		DependsOn: task.DependsOn,
	})
	if err != nil {
		return "", fmt.Errorf("sdk.AgentContext.CreateTask: %w", err)
	}
	return resp.GetTaskId(), nil
}

func (c *DefaultAgentContext) PublishEvent(ctx context.Context, event Event) (string, error) {
	stream := event.StreamName
	if stream == "" {
		stream = "astra:events"
	}
	actorID := event.ActorID
	if actorID == "" {
		actorID = c.agentID
	}
	resp, err := c.kernelClient.PublishEvent(ctx, &kernelpb.PublishEventRequest{
		StreamName: stream,
		EventType:  event.EventType,
		ActorId:    actorID,
		Payload:    event.Payload,
	})
	if err != nil {
		return "", fmt.Errorf("sdk.AgentContext.PublishEvent: %w", err)
	}
	return resp.GetEventId(), nil
}

func (c *DefaultAgentContext) CallTool(ctx context.Context, name string, input []byte) (ToolExecutionResult, error) {
	return c.toolClient.Execute(ctx, name, input)
}
