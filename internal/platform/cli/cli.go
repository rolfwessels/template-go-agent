package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

type Adapter struct {
	in   io.Reader
	out  io.Writer
	msgs chan string
}

func New() *Adapter {
	return NewWithIO(os.Stdin, os.Stdout)
}

func NewWithIO(in io.Reader, out io.Writer) *Adapter {
	return &Adapter{in: in, out: out}
}

func (a *Adapter) Connect(ctx context.Context) error {
	a.msgs = make(chan string)
	go a.scan(ctx)
	return nil
}

func (a *Adapter) scan(ctx context.Context) {
	defer close(a.msgs)
	scanner := bufio.NewScanner(a.in)
	for scanner.Scan() {
		msg := strings.TrimSpace(scanner.Text())
		if msg == "" {
			continue
		}
		select {
		case a.msgs <- msg:
		case <-ctx.Done():
			return
		}
	}
}

func (a *Adapter) SendMessage(_ context.Context, msg string) error {
	_, err := fmt.Fprintln(a.out, msg)
	return err
}

func (a *Adapter) ReceiveMessages(_ context.Context) (<-chan string, error) {
	return a.msgs, nil
}

func (a *Adapter) Disconnect(_ context.Context) error {
	return nil
}
