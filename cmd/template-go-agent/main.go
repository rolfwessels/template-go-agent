package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/rolfwessels/template-go-agent/internal/agent"
	"github.com/rolfwessels/template-go-agent/internal/config"
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

	fmt.Printf("template-go-agent v%s — type your question and press Enter (Ctrl+C to quit)\n", version)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		question := strings.TrimSpace(scanner.Text())
		if question == "" {
			continue
		}

		answer, err := a.Generate(ctx, question)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}

		fmt.Println(answer)
	}

	return scanner.Err()
}
