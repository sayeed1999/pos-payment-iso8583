# PoC Scope

The application will be in pragmatic Clean Arch with DDD.
The project will contain two services: one pos terminal, one host server.

## Functional Requirements

1. Customer will tap their Visa card on pos terminal, terminal sends payment request to host, will send a iso8583 message over tcp to host containing 0200 message, host will responsd 0210 with RC 00 as payment succeeded.
2. Customer sends payment request to host, but host responds insufficient fund in 0210 with RC 56.
3. Customer sends 0200 but host fails with 0210 RC 96 with system failure.
4. Terminal sends void request with 0200 with processing type as void (check what processing type real payment systems use in this case), host replies 0210 that void succeeded.
5. Terminal sends 0200, host times out. So Terminal sends a 0400 (reversal request), host replies 0410.
6. Terminal sends a 0400 (reversal request), host times out. Terminal re-sends a 0400.

## Non-Functional Requirements

1. Terminal (Pos machine) will send iso8583 data over tcp using net.Dial package in go, and Host server will keep listening on a port using net.Listen() and accept connections using ln.Accept(). // TODO: is it part of NFC? if not, where??
2. Each connection should be independent and should not block other connections.
3. Each connection runs on a separate goroutine. By any chance if the goroutine crashes, the program should not crash entirely.
4. The program must be running all the time. A crashed goroutine must heal/recover itself & not affect other goroutines task by any means.
5. In future if so many terminals hit at the same time, we maintain a connection pool of goroutines. [KEEP IT AS OUT OF SCOPE FOR CURRENT POC.]

### Coding Guidelines

- Error Handling: Code should always handle all errors, not ignore a single error with _.
- Logging: Every happy path & error path should be logged properly. Log should be enough to tell the journey, debugging code should not be necessary.
- Logging: Logs should traverse from the start of a request downstream to every flow of the journey back to the response being sent.
- Log Context: Log should pass important request info through context down to all levels. A correlation_id should be enough to track all logs related to a single request.

## Project Structure

```sh
cmd/
    pos-terminal/ - contains entry point for mock pos terminal
    host/ - contains entry point for mock host server
        main.go
config/ - centralized entry point for config (host address, port, database, etc.)
internal/
    transport/
        http/
        iso8583/
            handler - parses iso8583 message from tcp connection, converts it into golang payload and sends to service layer
            spec/ - contain spec for https://github.com/moov-io/iso8583/blob/master/specs/spec87ascii.go
    application/
        transaction/
            service - contains logic for payment, void, reversal, contains logic for insufficient funds, system error, wrong pin, etc.
            dto - contains request response payload
            error - contains errors used by service layer
    infrastructure/
        - contain mock db could be sqlite, db contains terminal (terminal_id, active, terminal_name), merchant (merchant_id, active, merchant_name), etc.
        - 
    domain
        - contains pure domain logic if any
```
