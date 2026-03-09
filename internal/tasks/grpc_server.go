package tasks

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tasks_pb "astra/proto/tasks"
)

const errTaskIDRequired = "task_id required"

type GRPCServer struct {
	tasks_pb.UnimplementedTaskServiceServer
	store TaskStore
}

func NewGRPCServer(store TaskStore) *GRPCServer {
	return &GRPCServer{store: store}
}

func (s *GRPCServer) CreateTask(ctx context.Context, req *tasks_pb.CreateTaskRequest) (*tasks_pb.CreateTaskResponse, error) {
	if req == nil || req.GraphId == "" || req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "graph_id and agent_id required")
	}
	graphID, err := uuid.Parse(req.GraphId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid graph_id: %v", err)
	}
	agentID, err := uuid.Parse(req.AgentId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}

	taskID := uuid.New()
	t := &Task{
		ID:         taskID,
		GraphID:    graphID,
		GoalID:     uuid.Nil,
		AgentID:    agentID,
		Type:       req.Type,
		Status:     StatusCreated,
		Payload:    req.Payload,
		Priority:   int(req.Priority),
		Retries:    0,
		MaxRetries: 5,
	}
	if err := s.store.CreateTask(ctx, t); err != nil {
		return nil, status.Errorf(codes.Internal, "create task: %v", err)
	}

	if len(req.DependsOn) > 0 {
		if err := s.store.AddDependencies(ctx, taskID.String(), req.DependsOn); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "add dependencies: %v", err)
		}
	}

	return &tasks_pb.CreateTaskResponse{TaskId: taskID.String()}, nil
}

func (s *GRPCServer) ScheduleTask(ctx context.Context, req *tasks_pb.ScheduleTaskRequest) (*tasks_pb.ScheduleTaskResponse, error) {
	if req == nil || req.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, errTaskIDRequired)
	}
	err := s.store.Transition(ctx, req.TaskId, StatusCreated, StatusPending, nil)
	if err == ErrInvalidTransition {
		return nil, status.Error(codes.FailedPrecondition, "invalid state transition (task may not be in created state)")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "schedule task: %v", err)
	}
	return &tasks_pb.ScheduleTaskResponse{}, nil
}

func (s *GRPCServer) CompleteTask(ctx context.Context, req *tasks_pb.CompleteTaskRequest) (*tasks_pb.CompleteTaskResponse, error) {
	if req == nil || req.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, errTaskIDRequired)
	}
	if err := s.store.CompleteTask(ctx, req.TaskId, req.Result); err != nil {
		if err == ErrInvalidTransition {
			return nil, status.Error(codes.FailedPrecondition, "invalid state transition")
		}
		return nil, status.Errorf(codes.Internal, "complete task: %v", err)
	}
	return &tasks_pb.CompleteTaskResponse{}, nil
}

func (s *GRPCServer) FailTask(ctx context.Context, req *tasks_pb.FailTaskRequest) (*tasks_pb.FailTaskResponse, error) {
	if req == nil || req.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, errTaskIDRequired)
	}
	if err := s.store.FailTask(ctx, req.TaskId, req.Error); err != nil {
		if err == ErrInvalidTransition {
			return nil, status.Error(codes.FailedPrecondition, "invalid state transition")
		}
		return nil, status.Errorf(codes.Internal, "fail task: %v", err)
	}
	return &tasks_pb.FailTaskResponse{}, nil
}

func (s *GRPCServer) GetTask(ctx context.Context, req *tasks_pb.GetTaskRequest) (*tasks_pb.GetTaskResponse, error) {
	if req == nil || req.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, errTaskIDRequired)
	}
	t, err := s.store.GetTask(ctx, req.TaskId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get task: %v", err)
	}
	if t == nil {
		return nil, status.Error(codes.NotFound, "task not found")
	}
	return taskToProto(t), nil
}

func (s *GRPCServer) GetGraph(ctx context.Context, req *tasks_pb.GetGraphRequest) (*tasks_pb.GetGraphResponse, error) {
	if req == nil || req.GraphId == "" {
		return nil, status.Error(codes.InvalidArgument, "graph_id required")
	}
	graph, _, err := s.store.GetGraph(ctx, req.GraphId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get graph: %v", err)
	}
	if graph == nil {
		return nil, status.Error(codes.NotFound, "graph not found")
	}

	resp := &tasks_pb.GetGraphResponse{}
	for i := range graph.Tasks {
		resp.Tasks = append(resp.Tasks, taskToProto(&graph.Tasks[i]))
	}
	for _, d := range graph.Dependencies {
		resp.Dependencies = append(resp.Dependencies, &tasks_pb.TaskDependency{
			TaskId:    d.TaskID.String(),
			DependsOn: d.DependsOn.String(),
		})
	}
	return resp, nil
}

func taskToProto(t *Task) *tasks_pb.GetTaskResponse {
	if t == nil {
		return nil
	}
	return &tasks_pb.GetTaskResponse{
		Id:        t.ID.String(),
		GraphId:   t.GraphID.String(),
		AgentId:   t.AgentID.String(),
		Type:      t.Type,
		Status:    string(t.Status),
		Payload:   t.Payload,
		Result:    t.Result,
		Priority:  int32(t.Priority),
		Retries:   int32(t.Retries),
		CreatedAt: t.CreatedAt.Unix(),
		UpdatedAt: t.UpdatedAt.Unix(),
	}
}
