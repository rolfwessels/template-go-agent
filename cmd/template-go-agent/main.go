package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudwego/eino/schema"
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
	logClose, err := initLogger(".storage/logs/app.log")
	if err != nil {
		return err
	}
	defer logClose()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGHUP, os.Interrupt)
	defer stop()

	slog.Info("starting", "version", version)

	fileStore := memory.NewFileStore(".storage/memory")
	sessions := memory.NewSessionStore(".storage/memory")

	distiller, err := memory.NewLLMDistiller(ctx, cfg.OpenAIAPIKey, cfg.OpenAIModel)
	if err != nil {
		return fmt.Errorf("creating distiller: %w", err)
	}

	vectorStore := memory.NewChromemStoreOrWarn(ctx, cfg.OllamaBaseURL, ".storage/memory")

	sweeper := memory.NewSweeper(fileStore, vectorStore, distiller)

	pool := agent.NewPool(
		func(ctx context.Context, userID string, history []*schema.Message) (*agent.Agent, error) {
			memCtx, _ := fileStore.AllAsContext(ctx, userID)
			return agent.New(ctx, cfg, agent.WithMemoryContext(memCtx), agent.WithInitialHistory(history))
		},
		time.Duration(cfg.SessionTimeoutMinutes)*time.Minute,
		sweeper.OnDestroy,
		agent.WithSessionProvider(sessions, cfg.ConversationHistoryWindowSize),
		agent.WithRecordHook(func(userID, sessionID, role, content string) {
			if err := sessions.Append(userID, sessionID, role, content); err != nil {
				slog.Warn("session record failed", "err", err)
			}
		}),
	)

	result := runPlatform(ctx, pool, cli.New())
	slog.Info("shutting down")
	if err := pool.Shutdown(context.Background()); err != nil {
		slog.Error("shutdown completed with errors", "err", err)
	}
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

func initLogger(path string) (func(), error) {
	if err := os.MkdirAll(".storage/logs", 0750); err != nil {
		return nil, fmt.Errorf("creating logs dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return func() { _ = f.Close() }, nil
}
