package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestLogIsOneJSONObject(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	setDefault(&buf)
	t.Cleanup(func() { slog.SetDefault(old) })

	ctx := WithCorrelationID(context.Background(), "abc")
	Info(ctx, "request accepted", "payload", map[string]any{
		"amount": 100,
		"pan":    "411111******1111",
	})

	line := strings.TrimSpace(buf.String())
	if strings.Contains(line, "\n") {
		t.Fatalf("more than one line:\n%s", buf.String())
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["correlation_id"] != "abc" || rec["msg"] != "request accepted" {
		t.Fatalf("record %+v", rec)
	}
	payload, ok := rec["payload"].(map[string]any)
	if !ok || payload["pan"] != "411111******1111" || payload["amount"] != float64(100) {
		t.Fatalf("payload %#v", rec["payload"])
	}
}

func TestWithGroupNestsFields(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(contextHandler{Handler: slog.NewJSONHandler(&buf, nil)}).WithGroup("payment")
	l.Info("transaction completed", "id", "123")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	payment, ok := rec["payment"].(map[string]any)
	if !ok || payment["id"] != "123" {
		t.Fatalf("record %+v", rec)
	}
}
