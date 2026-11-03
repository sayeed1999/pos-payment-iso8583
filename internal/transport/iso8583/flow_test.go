package iso8583

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"titp-poc/internal/application/transaction"
	"titp-poc/internal/infrastructure"
	"titp-poc/logger"
)

func TestPurchaseAmounts(t *testing.T) {
	term := startTerminal(t, time.Second)
	ctx := context.Background()
	cases := []struct {
		amount int64
		rc     string
	}{
		{100000, "00"},
		{5600, "56"},
		{9600, "96"},
	}
	for _, tc := range cases {
		out, err := term.Purchase(ctx, tc.amount)
		if err != nil {
			t.Fatal(err)
		}
		if out.Reply.MTI != "0210" || out.Reply.RC != tc.rc {
			t.Fatalf("amount %d: %s RC %s", tc.amount, out.Reply.MTI, out.Reply.RC)
		}
	}
}

func TestVoid(t *testing.T) {
	term := startTerminal(t, time.Second)
	ctx := context.Background()
	sale, err := term.Purchase(ctx, 100000)
	if err != nil {
		t.Fatal(err)
	}
	out, err := term.Void(ctx, sale)
	if err != nil {
		t.Fatal(err)
	}
	if out.Reply.MTI != "0210" || out.Reply.RC != "00" || out.Reply.RRN != sale.Reply.RRN {
		t.Fatalf("%s RC %s rrn %s", out.Reply.MTI, out.Reply.RC, out.Reply.RRN)
	}
}

func TestSaleTimeoutThenReversal(t *testing.T) {
	term := startTerminal(t, time.Second)
	ctx := context.Background()
	sale, err := term.Purchase(ctx, 500)
	if !errors.Is(err, transaction.ErrTimeout) {
		t.Fatalf("err %v reply %+v", err, sale.Reply)
	}
	out, err := term.Reverse(ctx, sale.Request)
	if err != nil {
		t.Fatal(err)
	}
	if out.Reply.MTI != "0410" || out.Reply.RC != "00" {
		t.Fatalf("%s RC %s", out.Reply.MTI, out.Reply.RC)
	}
}

func TestReversalTimeoutThenResend(t *testing.T) {
	term := startTerminal(t, time.Second)
	ctx := context.Background()
	sale, err := term.Purchase(ctx, 400)
	if err != nil {
		t.Fatal(err)
	}
	first, err := term.Reverse(ctx, sale.Request)
	if !errors.Is(err, transaction.ErrTimeout) {
		t.Fatalf("err %v reply %+v", err, first.Reply)
	}
	reply, err := term.Host.Exchange(ctx, first.Request)
	if err != nil {
		t.Fatal(err)
	}
	if reply.MTI != "0410" || reply.RC != "00" || reply.RRN != sale.Reply.RRN {
		t.Fatalf("%s RC %s rrn %s", reply.MTI, reply.RC, reply.RRN)
	}
}

func TestTwoConnections(t *testing.T) {
	ln := listen(t)
	var wg sync.WaitGroup
	errc := make(chan error, 4)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := NewClient(ln.Addr().String(), time.Second)
			if err := client.Dial(); err != nil {
				errc <- err
				return
			}
			defer func() {
				if err := client.Close(); err != nil {
					errc <- err
				}
			}()
			term := transaction.NewTerminal(client, "POS00001", "MERCHANT0000001", "050")
			ctx := logger.WithCorrelationID(context.Background(), logger.NewCorrelationID())
			out, err := term.Purchase(ctx, 100000)
			if err != nil {
				errc <- err
				return
			}
			if out.Reply.RC != "00" || out.Reply.MTI != "0210" {
				errc <- fmt.Errorf("got %s RC %s", out.Reply.MTI, out.Reply.RC)
			}
		}()
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		t.Error(err)
	}
}

func startTerminal(t *testing.T, timeout time.Duration) *transaction.Terminal {
	t.Helper()
	ln := listen(t)
	client := NewClient(ln.Addr().String(), timeout)
	if err := client.Dial(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	return transaction.NewTerminal(client, "POS00001", "MERCHANT0000001", "050")
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan error, 1)
	go func() {
		svc := transaction.NewService(infrastructure.NewDirectory())
		stopped <- NewServer(svc).Serve(context.Background(), ln)
	}()
	t.Cleanup(func() {
		if err := ln.Close(); err != nil {
			t.Error(err)
		}
		err := <-stopped
		if err != nil && !errors.Is(err, net.ErrClosed) {
			t.Error(err)
		}
	})
	return ln
}
