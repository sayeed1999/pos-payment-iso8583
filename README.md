# POS payment PoC

Two processes. The terminal sends ISO 8583 over TCP. The host answers.

```bash
go test ./...
go run ./cmd/host
go run ./cmd/pos-terminal
go run ./cmd/pos-terminal -amount 5600
go run ./cmd/pos-terminal -amount 9600
go run ./cmd/pos-terminal -void
go run ./cmd/pos-terminal -amount 500
go run ./cmd/pos-terminal -reverse -amount 400
```

`-amount` is minor units. `100000` is approved (0210 RC 00). `5600` is RC 56. `9600` is RC 96. `500` makes the host stay silent, and the terminal then sends a 0400. `-void` purchases, then sends a void (0200, processing code `020000`). `-reverse` purchases, then sends a 0400, and sends that same 0400 again if the host stays silent. Amount `400` is what makes the host stay silent on the first reversal.

Void uses processing code `020000`. In ISO 8583 the transaction type `02` is an adjustment. POS hosts use that as a void of a purchase. A refund is type `20`.

RC 56 is what this PoC calls insufficient funds. The ISO 8583 standard uses 51 for that.

Each host connection runs in its own goroutine. A panic is recovered and does not stop the listener or other connections. A connection pool is out of scope.

Logs carry `correlation_id` for one request inside a process. STAN is the id that appears on both sides of the socket.

```
cmd/host
cmd/pos-terminal
config
internal/transport/iso8583          TCP frames, parse, handler, client
internal/transport/iso8583/spec     moov Spec87ASCII
internal/application/transaction    purchase, void, reversal
logger                              logs and correlation id
internal/infrastructure             terminal and merchant records
internal/domain                     processing codes, response codes
```

The host accepts terminal `POS00001` and merchant `MERCHANT0000001`. The card is the Visa test PAN `4111111111111111`, tapped (DE 22 = `071`). Currency is `050`.
