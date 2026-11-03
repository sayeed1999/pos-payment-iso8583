package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestPayloadIsIndentedJSON(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(newHandler(&buf)))
	t.Cleanup(func() { slog.SetDefault(old) })

	ctx := WithServiceName(context.Background(), "host")
	ctx = WithCorrelationID(ctx, "abc")
	Info(ctx, "request accepted", "payload", map[string]any{"mti": "0200", "amount": 100})

	got := buf.String()
	if !strings.Contains(got, "service_name=host") || !strings.Contains(got, "correlation_id=abc") {
		t.Fatalf("log line missing ids:\n%s", got)
	}
	want := "payload={\n  \"amount\": 100,\n  \"mti\": \"0200\"\n}\n"
	if !strings.Contains(got, want) {
		t.Fatalf("payload not indented JSON:\n%s", got)
	}
}
