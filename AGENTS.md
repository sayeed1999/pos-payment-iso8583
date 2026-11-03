# Logging and errors

Match existing code. No new logger or error type.

## Logs

- `logger.Info` / `logger.Error` with the request `context.Context`. Never `slog` at a call site.
- `correlation_id` = one request in one process. Host sets it in `handler.one` when a request arrives. Terminal sets it in `main` for the run; `Terminal.ensure` sets one only if missing. The logger copies it from the context. `NewCorrelationID` returns an error when the random read fails.
- Log the step, then key-value fields. One JSON object per line. `Info` for an expected outcome. `Error` for a failure (dial, parse, timeout, panic).
- A full payment message is logged as `payload` after the caller masks it (`Request.Masked`). The logger does not mask.
- Panic on one connection: recover, log `panic` and `stack`, keep the listener and other connections up.

## Errors

- Handle every error. Never `_`.
- A failure returns `error`. A handled decision returns a value, not a Go error.
- A sentinel timeout: log it, return the sentinel, callers use `errors.Is`.
- `fmt.Errorf("step: %v", err)`. Use `%v`, not `%w`.
- Log once, where the failure is understood, then return. `main` logs the last one (`listen failed`, `terminal failed`).
- `io.EOF` ends a connection, no error log. `net.ErrClosed` ends `Serve`.
