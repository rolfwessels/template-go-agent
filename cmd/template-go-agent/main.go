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
	"github.com/rolfwessels/template-go-agent/internal/platform/discord"
	"github.com/rolfwessels/template-go-agent/internal/scheduler"
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

	fileStore := memory.NewFileStore(".storage")
	sessions := memory.NewSessionStore(".storage")

	distiller, err := memory.NewLLMDistiller(ctx, cfg.OpenAIAPIKey, cfg.OpenAIModel)
	if err != nil {
		return fmt.Errorf("creating distiller: %w", err)
	}

	vectorStore := memory.NewChromemStoreOrWarn(ctx, cfg.OllamaBaseURL, ".storage")

	sweeper := memory.NewSweeper(fileStore, vectorStore, distiller, sessions)

	var platformInstructions string
	if cfg.DiscordToken != "" {
		platformInstructions = discord.Instructions()
	}

	var (
		pool *agent.AgentPool
		sched *scheduler.Scheduler
	)
	pool = agent.NewPool(
		func(ctx context.Context, userID, channelID string, history []*schema.Message) (*agent.Agent, error) {
			memCtx, _ := fileStore.AllAsContext(ctx, userID)
			opts := []agent.Option{
				agent.WithMemoryContext(memCtx),
				agent.WithInitialHistory(history),
				agent.WithResetCallback(func(ctx context.Context) error {
					return pool.Reset(ctx, userID)
				}),
			}
			if platformInstructions != "" {
				opts = append(opts, agent.WithExtraInstructions(platformInstructions))
			}
			if sched != nil {
				opts = append(opts, agent.WithScheduler(sched, userID, channelID))
			}
			return agent.New(ctx, cfg, opts...)
		},
		time.Duration(cfg.SessionTimeoutMinutes)*time.Minute,
		sweeper.OnDestroy,
		agent.WithSessionProvider(sessions, cfg.ConversationHistoryWindowSize),
		agent.WithSessionCreator(sessions),
		agent.WithRecordHook(func(userID, sessionID, role, content string) {
			if err := sessions.Append(userID, sessionID, role, content); err != nil {
				slog.Warn("session record failed", "err", err)
			}
		}),
	)

	var adapter platform.MessagePlatform
	if cfg.DiscordToken != "" {
		slog.Info("discord token set — using Discord adapter")
		adapter = discord.New(cfg.DiscordToken)
	} else {
		fmt.Printf("template-go-agent v%s — type your question and press Enter (Ctrl+C to quit)\n", version)
		adapter = cli.New()
	}
	sched = scheduler.New(scheduler.NewStore(".storage"), pool, adapter)

	result := runPlatform(ctx, pool, adapter, sched)
	slog.Info("shutting down")
	if err := pool.Shutdown(context.Background()); err != nil {
		slog.Error("shutdown completed with errors", "err", err)
	}
	return result
}

func runPlatform(ctx context.Context, pool *agent.AgentPool, p platform.MessagePlatform, sched *scheduler.Scheduler) error {
	if err := p.Connect(ctx); err != nil {
		return fmt.Errorf("connecting platform: %w", err)
	}
	defer func() { _ = p.Disconnect(ctx) }()

	if err := sched.Start(ctx); err != nil {
		return fmt.Errorf("starting scheduler: %w", err)
	}
	defer sched.Stop()

	msgs, err := p.ReceiveMessages(ctx)
	if err != nil {
		return fmt.Errorf("receiving messages: %w", err)
	}

	for msg := range msgs {
		answer, err := pool.Send(ctx, msg.UserID, msg.ChannelID, msg.Content)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		if err := p.SendMessage(ctx, msg.ChannelID, answer); err != nil {
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
