package kernelserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"astra/internal/actors"
	"astra/internal/agent"
	"astra/internal/kernel"
	"astra/internal/messaging"

	kernel_pb "astra/proto/kernel"

	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type KernelGRPCServer struct {
	kernel_pb.UnimplementedKernelServiceServer
	kernel       *kernel.Kernel
	bus          *messaging.Bus
	db           *sql.DB
	agentFactory func(name string) *agent.Agent
}

func NewKernelGRPCServer(k *kernel.Kernel, bus *messaging.Bus, db *sql.DB, agentFactory func(name string) *agent.Agent) *KernelGRPCServer {
	return &KernelGRPCServer{
		kernel:       k,
		bus:          bus,
		db:           db,
		agentFactory: agentFactory,
	}
}

func (s *KernelGRPCServer) SpawnActor(ctx context.Context, req *kernel_pb.SpawnActorRequest) (*kernel_pb.SpawnActorResponse, error) {
	name := req.GetActorType()
	if name == "" {
		name = "agent"
	}
	a := s.agentFactory(name)
	if a == nil {
		return nil, status.Error(codes.Internal, "agent factory returned nil")
	}
	actorID := a.ID.String()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agents (id, name, actor_type, status) VALUES ($1, $2, $3, 'active')
		 ON CONFLICT (id) DO UPDATE SET status = 'active', updated_at = now()`,
		actorID, name, name)
	if err != nil {
		slog.Error("SpawnActor: insert agent failed", "actor_id", actorID, "err", err)
		return nil, status.Errorf(codes.Internal, "insert agent: %v", err)
	}

	return &kernel_pb.SpawnActorResponse{ActorId: actorID}, nil
}

func (s *KernelGRPCServer) SendMessage(ctx context.Context, req *kernel_pb.SendMessageRequest) (*kernel_pb.SendMessageResponse, error) {
	target := req.GetTargetActorId()
	msg := actors.Message{
		ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		Type:      req.GetMessageType(),
		Source:    req.GetSource(),
		Target:    target,
		Payload:   req.GetPayload(),
		Timestamp: time.Now(),
	}
	if err := s.kernel.Send(ctx, target, msg); err != nil {
		if errors.Is(err, actors.ErrMailboxFull) {
			const retryAfterSec = 5
			gogrpc.SetTrailer(ctx, metadata.Pairs("retry-after", strconv.Itoa(retryAfterSec)))
			return nil, status.Errorf(codes.ResourceExhausted, "actor mailbox full: %v", err)
		}
		return nil, status.Errorf(codes.NotFound, "send: %v", err)
	}
	return &kernel_pb.SendMessageResponse{}, nil
}

func (s *KernelGRPCServer) QueryState(ctx context.Context, req *kernel_pb.QueryStateRequest) (*kernel_pb.QueryStateResponse, error) {
	entityType := req.GetEntityType()
	filters := req.GetFilters()
	var results [][]byte

	switch entityType {
	case "agents":
		query := `SELECT id, name, COALESCE(actor_type, name) AS actor_type, status FROM agents LIMIT 100`
		rows, err := s.db.QueryContext(ctx, query)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "query agents: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id, name, actorType, statusVal string
			if err := rows.Scan(&id, &name, &actorType, &statusVal); err != nil {
				continue
			}
			b, _ := json.Marshal(map[string]string{
				"id":         id,
				"name":       name,
				"actor_type": actorType,
				"status":     statusVal,
			})
			results = append(results, b)
		}
	case "tasks":
		query := `SELECT id, graph_id, goal_id, agent_id, type, status, priority FROM tasks LIMIT 100`
		args := []interface{}{}
		if agentID, ok := filters["agent_id"]; ok && agentID != "" {
			query = `SELECT id, graph_id, goal_id, agent_id, type, status, priority FROM tasks WHERE agent_id = $1 LIMIT 100`
			args = append(args, agentID)
		}
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "query tasks: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id, graphID, goalID, agentID, taskType, taskStatus string
			var priority int
			if err := rows.Scan(&id, &graphID, &goalID, &agentID, &taskType, &taskStatus, &priority); err != nil {
				continue
			}
			b, _ := json.Marshal(map[string]interface{}{
				"id": id, "graph_id": graphID, "goal_id": goalID, "agent_id": agentID,
				"type": taskType, "status": taskStatus, "priority": priority,
			})
			results = append(results, b)
		}
	default:
		return &kernel_pb.QueryStateResponse{Results: nil}, nil
	}

	return &kernel_pb.QueryStateResponse{Results: results}, nil
}

func (s *KernelGRPCServer) SubscribeStream(_ *kernel_pb.SubscribeStreamRequest, _ kernel_pb.KernelService_SubscribeStreamServer) error {
	return status.Error(codes.Unimplemented, "SubscribeStream not implemented")
}

func (s *KernelGRPCServer) PublishEvent(ctx context.Context, req *kernel_pb.PublishEventRequest) (*kernel_pb.PublishEventResponse, error) {
	stream := req.GetStreamName()
	if stream == "" {
		stream = "astra:events"
	}
	fields := map[string]interface{}{
		"event_type": req.GetEventType(),
		"actor_id":   req.GetActorId(),
		"payload":    string(req.GetPayload()),
		"timestamp":  time.Now().Unix(),
	}
	id, err := s.bus.PublishReturnID(ctx, stream, fields)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "publish: %v", err)
	}
	return &kernel_pb.PublishEventResponse{EventId: id}, nil
}
