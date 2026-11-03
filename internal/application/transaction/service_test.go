package transaction

import (
	"context"
	"fmt"
	"testing"

	"pos-payment-poc/internal/domain"
	"pos-payment-poc/internal/infrastructure"
	"pos-payment-poc/logger"
)

func TestPurchaseDecisions(t *testing.T) {
	svc := NewService(infrastructure.NewDirectory())
	ctx := logger.WithCorrelationID(context.Background(), "purchase")
	cases := []struct {
		amount int64
		rc     string
		silent bool
		auth   bool
	}{
		{100000, domain.RCApproved, false, true},
		{5600, domain.RCInsufficientFunds, false, false},
		{9600, domain.RCSystemFailure, false, false},
		{500, domain.RCApproved, true, true},
		{400, domain.RCApproved, false, true},
	}
	for i, tc := range cases {
		got := svc.Authorize(ctx, purchaseReq(tc.amount, i+1))
		if got.MTI != "0210" || got.RC != tc.rc || got.Silent != tc.silent {
			t.Fatalf("amount %d: %+v", tc.amount, got)
		}
		if tc.auth && got.AuthID == "" {
			t.Fatalf("amount %d: missing auth id", tc.amount)
		}
		if !tc.auth && got.AuthID != "" {
			t.Fatalf("amount %d: unexpected auth id %s", tc.amount, got.AuthID)
		}
	}
}

func TestDuplicatePurchaseReplays(t *testing.T) {
	svc := NewService(infrastructure.NewDirectory())
	ctx := context.Background()
	req := purchaseReq(100000, 1)
	first := svc.Authorize(ctx, req)
	second := svc.Authorize(ctx, req)
	if first.RRN == "" || first.RRN != second.RRN || first.AuthID != second.AuthID || second.RC != domain.RCApproved {
		t.Fatalf("first %+v second %+v", first, second)
	}
}

func TestVoidApproved(t *testing.T) {
	svc := NewService(infrastructure.NewDirectory())
	ctx := context.Background()
	sale := svc.Authorize(ctx, purchaseReq(100000, 1))
	voidReq := purchaseReq(sale.Amount, 2)
	voidReq.ProcessingCode = domain.ProcVoid
	voidReq.RRN = sale.RRN
	got := svc.Authorize(ctx, voidReq)
	if got.MTI != "0210" || got.RC != domain.RCApproved || got.ProcessingCode != domain.ProcVoid || got.AuthID == "" {
		t.Fatalf("%+v", got)
	}
	if got.RRN != sale.RRN {
		t.Fatalf("void reply RRN %s, sale RRN %s", got.RRN, sale.RRN)
	}
}

func TestReversalTimeoutThenReplay(t *testing.T) {
	svc := NewService(infrastructure.NewDirectory())
	ctx := context.Background()
	sale := svc.Authorize(ctx, purchaseReq(400, 1))
	rev := Request{
		MTI:                  "0400",
		ProcessingCode:       domain.ProcPurchase,
		Amount:               400,
		STAN:                 "000010",
		Transmission:         "1103120001",
		TerminalID:           "POS00001",
		MerchantID:           "MERCHANT0000001",
		Currency:             "050",
		OriginalMTI:          "0200",
		OriginalSTAN:         sale.STAN,
		OriginalTransmission: sale.Transmission,
	}
	first := svc.Reverse(ctx, rev)
	if !first.Silent || first.MTI != "0410" || first.RC != domain.RCApproved || first.RRN != sale.RRN {
		t.Fatalf("first %+v sale RRN %s", first, sale.RRN)
	}
	second := svc.Reverse(ctx, rev)
	if second.Silent || second.RC != domain.RCApproved || second.RRN != first.RRN || second.STAN != rev.STAN {
		t.Fatalf("second %+v", second)
	}
}

func TestUnknownTerminal(t *testing.T) {
	svc := NewService(infrastructure.NewDirectory())
	req := purchaseReq(100000, 1)
	req.TerminalID = "POS00999"
	got := svc.Authorize(context.Background(), req)
	if got.RC != domain.RCTerminal {
		t.Fatalf("rc %s", got.RC)
	}
}

func TestRequestMaskedHidesPAN(t *testing.T) {
	req := purchaseReq(100000, 1)
	got := req.Masked()
	if req.PAN != "4111111111111111" {
		t.Fatalf("original pan %s", req.PAN)
	}
	if got.PAN != "411111******1111" {
		t.Fatalf("masked pan %s", got.PAN)
	}
}

func purchaseReq(amount int64, stan int) Request {
	return Request{
		MTI:            "0200",
		ProcessingCode: domain.ProcPurchase,
		Amount:         amount,
		STAN:           fmt.Sprintf("%06d", stan),
		Transmission:   "1103120000",
		LocalTime:      "180000",
		LocalDate:      "1103",
		TerminalID:     "POS00001",
		MerchantID:     "MERCHANT0000001",
		PAN:            "4111111111111111",
		Currency:       "050",
		EntryMode:      "071",
	}
}
