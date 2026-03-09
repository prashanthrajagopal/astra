package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"astra/pkg/sdk"
)

func main() {
	cfg := sdk.DefaultConfig()
	ctxClient, err := sdk.NewAgentContext(cfg)
	if err != nil {
		log.Fatalf("new agent context: %v", err)
	}
	defer ctxClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg := []byte(`{"message":"hello from echo-agent"}`)
	res, err := ctxClient.CallTool(ctx, "echo echo-agent", msg)
	if err != nil {
		log.Fatalf("call tool: %v", err)
	}
	fmt.Printf("agent=%s exit=%d output=%s\n", ctxClient.ID(), res.ExitCode, string(res.Output))
}
