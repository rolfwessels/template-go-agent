package platform

import "context"

type MessagePlatform interface {
	Connect(ctx context.Context) error
	SendMessage(ctx context.Context, msg string) error
	ReceiveMessages(ctx context.Context) (<-chan string, error)
	Disconnect(ctx context.Context) error
}
