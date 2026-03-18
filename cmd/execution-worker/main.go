package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"astra/internal/codegen"
	"astra/internal/messaging"
	"astra/internal/tasks"
	"astra/internal/tools"
	"astra/internal/workers"
	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/logger"

	llmpb "astra/proto/llm"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logger.New(cfg.LogLevel))

	database, err := db.Connect(cfg.PostgresDSN())
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	bus := messaging.New(cfg.RedisAddr)
	defer bus.Close()

	taskStore := tasks.NewStore(database)
	registry := workers.NewRegistry(database)

	wsRuntime, legacyRuntime := newToolRuntimes()

	llmAddr := getEnv("LLM_GRPC_ADDR", "localhost:9093")
	llmConn, err := grpc.NewClient(llmAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Warn("could not connect to llm-router, codegen tasks will fail", "addr", llmAddr, "err", err)
	}
	var llmClient llmpb.LLMRouterClient
	if llmConn != nil {
		defer llmConn.Close()
		llmClient = llmpb.NewLLMRouterClient(llmConn)
	}

	hostname, _ := os.Hostname()
	w := workers.New(hostname, bus)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := registry.Register(ctx, w.ID.String(), hostname, []string{"general", "code_generate", "shell_exec"}); err != nil {
		slog.Error("failed to register worker", "err", err)
		os.Exit(1)
	}
	slog.Info("worker registered", "worker_id", w.ID, "hostname", hostname)

	go w.StartHeartbeat(ctx)
	go runRegistryHeartbeat(ctx, registry, w.ID.String())

	shardCount := getTaskShardCount()
	slog.Info("execution worker started", "worker_id", w.ID, "hostname", hostname, "shard_count", shardCount)

	handler := newTaskHandler(bus, taskStore, wsRuntime, legacyRuntime, llmClient, w.ID.String())
	for shard := 0; shard < shardCount; shard++ {
		s := shard
		go func() {
			_ = bus.Consume(ctx, taskStreamForShard(s), "worker-group", w.ID.String()+"-shard-"+strconv.Itoa(s), handler)
		}()
	}
	<-ctx.Done()

	_ = registry.MarkOffline(context.Background(), w.ID.String())
	slog.Info("execution worker stopped", "worker_id", w.ID)
}

func newTaskHandler(bus *messaging.Bus, taskStore *tasks.Store, wsRuntime *tools.WorkspaceRuntime, legacyRuntime tools.Runtime, llmClient llmpb.LLMRouterClient, workerID string) func(redis.XMessage) error {
	return func(msg redis.XMessage) error {
		taskID := extractTaskID(msg)
		if taskID == "" {
			return nil
		}
		return processTask(bus, taskStore, wsRuntime, legacyRuntime, llmClient, workerID, taskID)
	}
}

func extractTaskID(msg redis.XMessage) string {
	taskIDVal, ok := msg.Values["task_id"]
	if !ok {
		slog.Warn("message missing task_id", "msg_id", msg.ID)
		return ""
	}
	taskID, ok := taskIDVal.(string)
	if !ok || taskID == "" {
		slog.Warn("invalid task_id", "msg_id", msg.ID)
		return ""
	}
	return taskID
}

const deadLetterStream = "astra:dead_letter"

func getTaskShardCount() int {
	if s := os.Getenv("TASK_SHARD_COUNT"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return 1
}

func taskStreamForShard(shard int) string {
	return "astra:tasks:shard:" + strconv.Itoa(shard)
}

func publishDeadLetterIf(bus *messaging.Bus, ctx context.Context, taskID, goalID, errMsg string, movedToDeadLetter bool) {
	if !movedToDeadLetter || bus == nil {
		return
	}
	payload := map[string]interface{}{
		"task_id":   taskID,
		"error":     errMsg,
		"timestamp": time.Now().Unix(),
	}
	if goalID != "" {
		payload["goal_id"] = goalID
	}
	if err := bus.Publish(ctx, deadLetterStream, payload); err != nil {
		slog.Warn("publish to dead_letter stream failed", "task_id", taskID, "err", err)
	}
}

func processTask(bus *messaging.Bus, taskStore *tasks.Store, wsRuntime *tools.WorkspaceRuntime, legacyRuntime tools.Runtime, llmClient llmpb.LLMRouterClient, workerID, taskID string) error {
	runCtx := context.Background()

	if err := taskStore.Transition(runCtx, taskID, tasks.StatusQueued, tasks.StatusScheduled, nil); err != nil {
		slog.Error("transition queued->scheduled failed", "task_id", taskID, "err", err)
		return nil
	}
	if err := taskStore.SetWorkerID(runCtx, taskID, workerID); err != nil {
		slog.Error("set worker_id failed", "task_id", taskID, "err", err)
	}

	if err := taskStore.Transition(runCtx, taskID, tasks.StatusScheduled, tasks.StatusRunning, nil); err != nil {
		slog.Error("transition scheduled->running failed", "task_id", taskID, "err", err)
		return nil
	}
	slog.Info("task running", "task_id", taskID)

	task, err := taskStore.GetTask(runCtx, taskID)
	if err != nil {
		slog.Error("get task failed", "task_id", taskID, "err", err)
		moved, _ := taskStore.FailTask(runCtx, taskID, err.Error())
		publishDeadLetterIf(bus, runCtx, taskID, "", err.Error(), moved)
		return nil
	}
	if task == nil {
		slog.Warn("task not found", "task_id", taskID)
		moved, _ := taskStore.FailTask(runCtx, taskID, "task not found")
		publishDeadLetterIf(bus, runCtx, taskID, "", "task not found", moved)
		return nil
	}

	result, taskErr := executeTask(runCtx, task, wsRuntime, legacyRuntime, llmClient)
	if taskErr != nil {
		slog.Error("task execution failed", "task_id", taskID, "type", task.Type, "err", taskErr)
		moved, _ := taskStore.FailTask(runCtx, taskID, taskErr.Error())
		publishDeadLetterIf(bus, runCtx, taskID, task.GoalID.String(), taskErr.Error(), moved)
		return nil
	}

	if err := taskStore.CompleteTask(runCtx, taskID, result); err != nil {
		slog.Error("complete task failed", "task_id", taskID, "err", err)
		return nil
	}
	slog.Info("task completed", "task_id", taskID, "type", task.Type)
	return nil
}

func executeTask(ctx context.Context, task *tasks.Task, wsRuntime *tools.WorkspaceRuntime, legacyRuntime tools.Runtime, llmClient llmpb.LLMRouterClient) ([]byte, error) {
	switch task.Type {
	case "code_generate":
		return executeCodeGen(ctx, task, wsRuntime, llmClient)
	case "shell_exec":
		return executeShellExec(ctx, task, wsRuntime)
	default:
		return executeLegacy(ctx, task, legacyRuntime)
	}
}

func executeCodeGen(ctx context.Context, task *tasks.Task, wsRuntime *tools.WorkspaceRuntime, llmClient llmpb.LLMRouterClient) ([]byte, error) {
	if llmClient == nil {
		return nil, fmt.Errorf("llm-router not available for code_generate task")
	}
	var payload codegen.TaskPayload
	if task.Payload != nil {
		_ = json.Unmarshal(task.Payload, &payload)
	}
	result, err := codegen.Process(ctx, task.ID.String(), task.AgentID.String(), payload, wsRuntime, llmClient)
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(result)
	return out, nil
}

func executeShellExec(ctx context.Context, task *tasks.Task, wsRuntime *tools.WorkspaceRuntime) ([]byte, error) {
	var payload codegen.TaskPayload
	if task.Payload != nil {
		_ = json.Unmarshal(task.Payload, &payload)
	}
	result, err := codegen.ProcessShellExec(ctx, payload, wsRuntime)
	if err != nil {
		return nil, err
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}
	out, _ := json.Marshal(result)
	return out, nil
}

func executeLegacy(ctx context.Context, task *tasks.Task, runtime tools.Runtime) ([]byte, error) {
	payload := task.Payload
	if payload == nil {
		payload = []byte("{}")
	}
	toolReq := tools.ToolRequest{
		Name:        task.Type,
		Input:       payload,
		Timeout:     30 * time.Second,
		MemoryLimit: 256 * 1024 * 1024,
		CPULimit:    1.0,
	}
	toolResult, err := runtime.Execute(ctx, toolReq)
	if err != nil {
		return nil, err
	}
	if toolResult.ExitCode != 0 {
		errMsg := string(toolResult.Output)
		if errMsg == "" {
			errMsg = "tool exited with non-zero code"
		}
		return nil, fmt.Errorf("%s", errMsg)
	}
	result := toolResult.Output
	if result == nil {
		result = []byte("{}")
	}
	return result, nil
}

func newToolRuntimes() (*tools.WorkspaceRuntime, tools.Runtime) {
	workspaceRoot := getEnv("WORKSPACE_ROOT", "workspace")
	wsRuntime := tools.NewWorkspaceRuntime(workspaceRoot)

	rt := strings.ToLower(strings.TrimSpace(os.Getenv("TOOL_RUNTIME")))
	var legacy tools.Runtime
	switch rt {
	case "docker":
		legacy = tools.NewDockerRuntime()
	default:
		legacy = tools.NewNoopRuntime()
	}
	return wsRuntime, legacy
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runRegistryHeartbeat(ctx context.Context, registry *workers.Registry, workerID string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := registry.UpdateHeartbeat(ctx, workerID); err != nil {
				slog.Error("registry heartbeat failed", "worker_id", workerID, "err", err)
			}
		}
	}
}
