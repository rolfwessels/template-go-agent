package platform

import "context"

type Message struct {
	UserID    string
	ChannelID string
	Content   string
}

type MessagePlatform interface {
	Connect(ctx context.Context) error
	SendMessage(ctx context.Context, channelID string, msg string) error
	ReceiveMessages(ctx context.Context) (<-chan Message, error)
	Disconnect(ctx context.Context) error
}
