package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"titp-poc/config"
	"titp-poc/internal/application/transaction"
	"titp-poc/internal/infrastructure"
	"titp-poc/internal/transport/iso8583"
)

func main() {
	cfg := config.DefaultTerminal()
	// TODO(poc): -amount, -void, and -reverse drive the demo. A real terminal
	// takes the amount from the sale. It voids because the cashier voided, and
	// it reverses because the 0210 did not arrive. The special amounts are
	// interpreted by the host stand-in, not by ISO 8583:
	//   5600 insufficient funds, 9600 system failure,
	//   500 host stays silent on the 0200, 400 host stays silent on the first 0400.
	amount := flag.Int64("amount", 100000, "minor units: 100000 approved, 5600 insufficient funds, 9600 system failure, 500 host stays silent, 400 host stays silent on the first reversal")
	doVoid := flag.Bool("void", false, "after the purchase, send a void for that sale")
	doReverse := flag.Bool("reverse", false, "after the purchase, send a reversal; resend it if the host stays silent")
	flag.StringVar(&cfg.HostAddr, "addr", cfg.HostAddr, "host address")
	flag.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "how long to wait for a host response")
	flag.Parse()

	if *doVoid && *doReverse {
		fmt.Fprintln(os.Stderr, "pass -void or -reverse, not both")
		os.Exit(2)
	}

	client := iso8583.NewClient(cfg.HostAddr, cfg.Timeout)
	if err := client.Dial(); err != nil {
		infrastructure.Error(context.Background(), "dial failed", "addr", cfg.HostAddr, "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := client.Close(); err != nil {
			infrastructure.Error(context.Background(), "close failed", "error", err)
		}
	}()

	term := transaction.NewTerminal(client, cfg.TerminalID, cfg.MerchantID, cfg.Currency)
	ctx := infrastructure.WithCorrelationID(context.Background(), infrastructure.NewCorrelationID())
	if err := run(ctx, term, *amount, *doVoid, *doReverse); err != nil {
		infrastructure.Error(ctx, "terminal failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, term *transaction.Terminal, amount int64, doVoid, doReverse bool) error {
	sale, err := term.Purchase(ctx, amount)
	if err != nil {
		if !errors.Is(err, transaction.ErrTimeout) {
			return err
		}
		fmt.Println("purchase timed out, sending 0400")
		return finishReversal(ctx, term, sale.Request)
	}
	fmt.Printf("%s RC %s\n", sale.Reply.MTI, sale.Reply.RC)
	if doVoid {
		out, err := term.Void(ctx, sale)
		if err != nil {
			return err
		}
		fmt.Printf("%s RC %s\n", out.Reply.MTI, out.Reply.RC)
	}
	if doReverse {
		return finishReversal(ctx, term, sale.Request)
	}
	return nil
}

func finishReversal(ctx context.Context, term *transaction.Terminal, original transaction.Request) error {
	out, err := term.Reverse(ctx, original)
	if err != nil {
		if !errors.Is(err, transaction.ErrTimeout) {
			return err
		}
		fmt.Println("reversal timed out, resending 0400")
		reply, err := term.Host.Exchange(ctx, out.Request)
		if err != nil {
			return err
		}
		fmt.Printf("%s RC %s\n", reply.MTI, reply.RC)
		return nil
	}
	fmt.Printf("%s RC %s\n", out.Reply.MTI, out.Reply.RC)
	return nil
}
