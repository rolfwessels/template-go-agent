package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rolfwessels/template-go-agent/internal/agent"
	"github.com/rolfwessels/template-go-agent/internal/config"
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

	ctx := context.Background()

	a, err := agent.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initializing agent: %w", err)
	}

	return runPlatform(ctx, a, cli.New())
}

func runPlatform(ctx context.Context, a *agent.Agent, p platform.MessagePlatform) error {
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
		answer, err := a.Generate(ctx, msg)
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
