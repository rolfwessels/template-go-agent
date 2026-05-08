package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rolfwessels/template-go-agent/internal/platform"
)

type Adapter struct {
	in   io.Reader
	out  io.Writer
	msgs chan platform.Message
}

func New() *Adapter {
	return NewWithIO(os.Stdin, os.Stdout)
}

func NewWithIO(in io.Reader, out io.Writer) *Adapter {
	return &Adapter{in: in, out: out}
}

func (a *Adapter) Connect(ctx context.Context) error {
	a.msgs = make(chan platform.Message)
	go a.scan(ctx)
	return nil
}

func (a *Adapter) scan(ctx context.Context) {
	defer close(a.msgs)
	go func() {
		<-ctx.Done()
		if f, ok := a.in.(*os.File); ok {
			_ = f.Close()
		}
	}()
	scanner := bufio.NewScanner(a.in)
	for scanner.Scan() {
		content := strings.TrimSpace(scanner.Text())
		if content == "" {
			continue
		}
		msg := platform.Message{UserID: "cli", ChannelID: "", Content: content}
		select {
		case a.msgs <- msg:
		case <-ctx.Done():
			return
		}
	}
}

func (a *Adapter) SendMessage(_ context.Context, _ string, msg string) error {
	_, err := fmt.Fprintln(a.out, msg)
	return err
}

func (a *Adapter) ReceiveMessages(_ context.Context) (<-chan platform.Message, error) {
	return a.msgs, nil
}

func (a *Adapter) Disconnect(_ context.Context) error {
	return nil
}
