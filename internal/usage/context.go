package usage

import "context"

type contextKey struct{}

func WithContext(ctx context.Context, userID, sessionID string) context.Context {
	return context.WithValue(ctx, contextKey{}, [2]string{userID, sessionID})
}

func FromContext(ctx context.Context) (userID, sessionID string) {
	if v, ok := ctx.Value(contextKey{}).([2]string); ok {
		return v[0], v[1]
	}
	return "", ""
}
