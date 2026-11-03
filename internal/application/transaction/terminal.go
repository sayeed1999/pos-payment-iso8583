package transaction

import (
	"context"
	"fmt"
	"sync"
	"time"

	"pos-payment-poc/internal/domain"
	"pos-payment-poc/logger"
)

// Terminal is the mock POS. It builds the business request. The Wire sends it.
type Terminal struct {
	Host       Wire
	TerminalID string
	MerchantID string
	Currency   string
	PAN        string
	EntryMode  string

	mu   sync.Mutex
	stan int
}

func NewTerminal(host Wire, terminalID, merchantID, currency string) *Terminal {
	return &Terminal{
		Host:       host,
		TerminalID: terminalID,
		MerchantID: merchantID,
		Currency:   currency,
		// TODO(poc): replace with the PAN read from the tapped card. This is a Visa test number.
		PAN: "4111111111111111",
		// 07 contactless, 1 means the terminal can take a PIN.
		EntryMode: "071",
	}
}

func (t *Terminal) nextSTAN() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stan++
	if t.stan > 999999 {
		t.stan = 1
	}
	return fmt.Sprintf("%06d", t.stan)
}

func (t *Terminal) stamp() (transmission, localTime, localDate string) {
	now := time.Now()
	local := now.In(time.FixedZone("BDT", 6*3600))
	return now.UTC().Format("0102150405"), local.Format("150405"), local.Format("0102")
}

func (t *Terminal) base(mti, proc string, amount int64) Request {
	transmission, localTime, localDate := t.stamp()
	return Request{
		MTI:            mti,
		ProcessingCode: proc,
		Amount:         amount,
		STAN:           t.nextSTAN(),
		Transmission:   transmission,
		LocalTime:      localTime,
		LocalDate:      localDate,
		TerminalID:     t.TerminalID,
		MerchantID:     t.MerchantID,
		PAN:            t.PAN,
		Currency:       t.Currency,
		EntryMode:      t.EntryMode,
	}
}

func ensure(ctx context.Context) (context.Context, error) {
	if logger.CorrelationID(ctx) != "" {
		return ctx, nil
	}
	id, err := logger.NewCorrelationID()
	if err != nil {
		return ctx, err
	}
	return logger.WithCorrelationID(ctx, id), nil
}

// Purchase sends a 0200 with processing code 000000.
func (t *Terminal) Purchase(ctx context.Context, amount int64) (Outcome, error) {
	ctx, err := ensure(ctx)
	if err != nil {
		return Outcome{}, err
	}
	req := t.base("0200", domain.ProcPurchase, amount)
	logger.Info(ctx, "sending purchase", "payload", req.Masked())
	reply, err := t.Host.Exchange(ctx, req)
	return Outcome{Request: req, Reply: reply}, err
}

// Void sends a 0200 with processing code 020000 for a sale the terminal completed.
func (t *Terminal) Void(ctx context.Context, sale Outcome) (Outcome, error) {
	ctx, err := ensure(ctx)
	if err != nil {
		return Outcome{}, err
	}
	req := t.base("0200", domain.ProcVoid, sale.Request.Amount)
	req.RRN = sale.Reply.RRN
	logger.Info(ctx, "sending void", "payload", req.Masked())
	reply, err := t.Host.Exchange(ctx, req)
	return Outcome{Request: req, Reply: reply}, err
}

// Reverse sends a new 0400 that points at original through DE 90.
func (t *Terminal) Reverse(ctx context.Context, original Request) (Outcome, error) {
	ctx, err := ensure(ctx)
	if err != nil {
		return Outcome{}, err
	}
	req := t.base("0400", original.ProcessingCode, original.Amount)
	req.OriginalMTI = original.MTI
	req.OriginalSTAN = original.STAN
	req.OriginalTransmission = original.Transmission
	logger.Info(ctx, "sending reversal", "payload", req.Masked())
	reply, err := t.Host.Exchange(ctx, req)
	return Outcome{Request: req, Reply: reply}, err
}
