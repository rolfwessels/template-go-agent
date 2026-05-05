package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rolfwessels/template-go-agent/internal/agent"
	"github.com/rolfwessels/template-go-agent/internal/config"
	"github.com/rolfwessels/template-go-agent/internal/memory"
	"github.com/rolfwessels/template-go-agent/internal/platform"
	"github.com/rolfwessels/template-go-agent/internal/platform/cli"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGHUP, os.Interrupt)
	defer stop()

	fileStore := memory.NewFileStore("memory")

	distiller, err := memory.NewLLMDistiller(ctx, cfg.OpenAIAPIKey, cfg.OpenAIModel)
	if err != nil {
		return fmt.Errorf("creating distiller: %w", err)
	}

	vectorStore := memory.NewChromemStoreOrWarn(ctx, cfg.OllamaBaseURL, "memory")

	sweeper := memory.NewSweeper(fileStore, vectorStore, distiller)

	pool := agent.NewPool(
		func(ctx context.Context, userID string) (*agent.Agent, error) {
			memCtx, _ := fileStore.AllAsContext(ctx, userID)
			return agent.New(ctx, cfg, agent.WithMemoryContext(memCtx))
		},
		time.Duration(cfg.SessionTimeoutMinutes)*time.Minute,
		sweeper.OnDestroy,
	)

	result := runPlatform(ctx, pool, cli.New())
	pool.Shutdown(context.Background())
	return result
}

func runPlatform(ctx context.Context, pool *agent.AgentPool, p platform.MessagePlatform) error {
	if err := p.Connect(ctx); err != nil {
		return fmt.Errorf("connecting platform: %w", err)
	}
	defer func() { _ = p.Disconnect(ctx) }()

	msgs, err := p.ReceiveMessages(ctx)
	if err != nil {
		return fmt.Errorf("receiving messages: %w", err)
	}

	fmt.Printf("template-go-agent v%s — type your question and press Enter (Ctrl+C to quit)\n", version)

	for msg := range msgs {
		answer, err := pool.Send(ctx, "cli-user", msg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		if err := p.SendMessage(ctx, answer); err != nil {
			fmt.Fprintf(os.Stderr, "error sending message: %v\n", err)
		}
	}

	return nil
}
