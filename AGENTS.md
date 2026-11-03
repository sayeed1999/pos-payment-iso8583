# Logging and errors

Match existing code. No new logger or error type.

## Logs

- `logger.Info` / `logger.Error` with the request `context.Context`. Never `slog`.
- Every line has `service_name` and `correlation_id` from that context. `cmd/host` and `cmd/pos-terminal` call `logger.WithServiceName` once from config (`host`, `terminal`) before any log. Derive later contexts from that one. `context.Background()` drops the name.
- `correlation_id` = one request in one process. Host sets it in `handler.one` when a request arrives. Terminal sets it in `main` for the run; `Terminal.ensure` sets one only if missing. Do not put the service name in the correlation id.
- Log the step, then key-value fields. `Info` for an expected outcome. `Error` for a failure (dial, parse, timeout, panic).
- Panic on one connection: recover, log `panic` and `stack`, keep the listener and other connections up.

## Errors

- Handle every error. Never `_`.
- A failure returns `error`. A handled decision returns a value, not a Go error.
- A sentinel timeout: log it, return the sentinel, callers use `errors.Is`.
- `fmt.Errorf("step: %v", err)`. Use `%v`, not `%w`.
- Log once, where the failure is understood, then return. `main` logs the last one (`listen failed`, `terminal failed`).
- `io.EOF` ends a connection, no error log. `net.ErrClosed` ends `Serve`.
