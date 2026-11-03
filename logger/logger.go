package logger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"
)

type correlationKey struct{}

func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey{}, id)
}

func CorrelationID(ctx context.Context) string {
	id, ok := ctx.Value(correlationKey{}).(string)
	if !ok {
		return ""
	}
	return id
}

func NewCorrelationID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("time-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func Info(ctx context.Context, msg string, args ...any) {
	slog.InfoContext(ctx, msg, withID(ctx, args)...)
}

func Error(ctx context.Context, msg string, args ...any) {
	slog.ErrorContext(ctx, msg, withID(ctx, args)...)
}

func withID(ctx context.Context, args []any) []any {
	return append([]any{"correlation_id", CorrelationID(ctx)}, args...)
}
