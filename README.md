# POS payment PoC

A mock card-present sale over ISO 8583. Two processes talk TCP: a POS terminal sends a purchase, and a host answers. The same pair can void that sale, or reverse it when the host stays silent.

This is a stand-in. The amount picks the host's answer so each journey can be forced from the command line. There is no issuer, no real card, and no store that survives a restart.

## POS terminal

`cmd/pos-terminal` is the cashier side. It dials the host, builds one ISO 8583 request, and prints the reply as `MTI RC`.

It always sends a purchase first (`0200`, processing code `000000`). The card is the Visa test PAN `4111111111111111`, tapped (DE 22 = `071`). Terminal is `POS00001`, merchant is `MERCHANT0000001`, currency is `050`. Default host is `127.0.0.1:8583`. It waits 2 seconds.

`-void` then sends a void of that sale (`0200`, processing code `020000`). In ISO 8583 the transaction type `02` is an adjustment. POS hosts use that as a void of a purchase. A refund is type `20`.

`-reverse` then sends a reversal (`0400`). If the purchase itself times out, the terminal sends a `0400` on its own. If that `0400` also times out, it sends the same `0400` again.

`-amount` is minor units. The host, not ISO 8583, reads the special amounts below.

## Try it

Start the host in one terminal, then run one of these in another.

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

| Command | What you should see |
|---|---|
| `go run ./cmd/pos-terminal` | `0210 RC 00`. Amount `100000` is approved. |
| `-amount 5600` | `0210 RC 56`. This PoC calls 56 insufficient funds. ISO 8583 uses 51 for that. |
| `-amount 9600` | `0210 RC 96`. System failure. |
| `-void` | `0210 RC 00`, then a second `0210 RC 00` for the void. |
| `-amount 500` | Host stays silent on the `0200`. Terminal prints `purchase timed out, sending 0400`, then `0410 RC 00`. |
| `-reverse -amount 400` | Purchase is approved. Host stays silent on the first `0400`. Terminal resends it and prints `0410 RC 00`. |

`-addr` overrides the host address. `-timeout` overrides how long the terminal waits. Pass `-void` or `-reverse`, not both.

## Host

`cmd/host` listens on `:8583`. One goroutine per connection. It accepts `0200` and `0400`. Anything else is dropped. A request it cannot parse gets a format error (`RC 30`) when the MTI is one it knows how to answer.

It checks the terminal, then the merchant. Unknown or inactive terminal is `RC 58`. Unknown or inactive merchant is `RC 03`. Only `POS00001` and `MERCHANT0000001` are seeded.

`RC` is shorthand for DE 39. The letters `RC` are not on the wire.

| Request | Host returns |
|---|---|
| `0200` purchase, any amount except the three below | `0210` `RC 00`, plus an auth id and an RRN |
| `0200` purchase, amount `5600` | `0210` `RC 56` |
| `0200` purchase, amount `9600` | `0210` `RC 96` |
| `0200` purchase, amount `500` | nothing. The `0210` is withheld. The sale is still stored as approved. |
| `0200` purchase, same terminal + STAN + transmission again | the first `0210`, replayed |
| `0200` void (`020000`) | `0210` `RC 00`. Approved even when no sale matches the RRN. |
| `0200` any other processing code | `0210` `RC 30` |
| `0400` reversal | `0410` `RC 00`. Accepted even when the original sale never arrived. The RRN is the original sale's when that sale is still in memory. |
| `0400` reversal, amount `400`, first time | nothing. The `0410` is withheld so the terminal can resend. |
| `0400` reversal, same terminal + STAN + transmission again | the first `0410`, and this time it is written |
| bad frame, or a required field missing, on `0200` or `0400` | `0210` or `0410` with `RC 30` |

A duplicate key is terminal id + STAN + transmission date-time. Restarting the host forgets every sale, so a retried `0200` or `0400` looks new.

## Coding guidelines

Each host connection runs in its own goroutine. A panic is recovered and does not stop the listener or other connections. A connection pool is out of scope.

Logs are one JSON object per line. `correlation_id` is one request inside that process. STAN is the id that appears on both sides of the socket. A logged payment message is masked before it reaches the logger.

```sh
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
