package logger

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

type correlationKey struct{}
type serviceKey struct{}

func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey{}, id)
}

func WithServiceName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, serviceKey{}, name)
}

func ServiceName(ctx context.Context) string {
	name, ok := ctx.Value(serviceKey{}).(string)
	if !ok {
		return ""
	}
	return name
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

func init() {
	slog.SetDefault(slog.New(newHandler(os.Stderr)))
}

func Info(ctx context.Context, msg string, args ...any) {
	slog.InfoContext(ctx, msg, withID(ctx, args)...)
}

func Error(ctx context.Context, msg string, args ...any) {
	slog.ErrorContext(ctx, msg, withID(ctx, args)...)
}

func withID(ctx context.Context, args []any) []any {
	return append([]any{
		"service_name", ServiceName(ctx),
		"correlation_id", CorrelationID(ctx),
	}, args...)
}

// handler is the process logger. A value passed as "payload" is indented JSON.
// Every other field stays on the slog text line.
type handler struct {
	w   io.Writer
	mu  *sync.Mutex
	pre []slog.Attr
}

func newHandler(w io.Writer) *handler {
	return &handler{w: w, mu: &sync.Mutex{}}
}

func (h *handler) Enabled(context.Context, slog.Level) bool { return true }

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	n := *h
	n.pre = append(append([]slog.Attr{}, h.pre...), attrs...)
	return &n
}

func (h *handler) WithGroup(string) slog.Handler { return h }

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	var payloads []string
	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	nr.AddAttrs(h.pre...)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "payload" {
			payloads = append(payloads, formatPayload(a.Value.Resolve().Any()))
			return true
		}
		nr.AddAttrs(a)
		return true
	})

	var line bytes.Buffer
	if err := slog.NewTextHandler(&line, nil).Handle(context.Background(), nr); err != nil {
		return err
	}
	out := bytes.TrimRight(line.Bytes(), "\n")
	for _, p := range payloads {
		out = append(out, " payload="...)
		out = append(out, p...)
	}
	out = append(out, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(out)
	return err
}

func formatPayload(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
