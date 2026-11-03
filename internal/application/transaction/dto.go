package transaction

import (
	"context"
	"encoding/json"
	"strings"
)

// Request is one 0200 or 0400 after it has been parsed off the wire.
type Request struct {
	MTI                  string `json:"mti,omitempty"`
	ProcessingCode       string `json:"processing_code,omitempty"`
	Amount               int64  `json:"amount"`
	STAN                 string `json:"stan,omitempty"`
	Transmission         string `json:"transmission,omitempty"`
	LocalTime            string `json:"local_time,omitempty"`
	LocalDate            string `json:"local_date,omitempty"`
	TerminalID           string `json:"terminal_id,omitempty"`
	MerchantID           string `json:"merchant_id,omitempty"`
	PAN                  string `json:"pan,omitempty"`
	Currency             string `json:"currency,omitempty"`
	EntryMode            string `json:"entry_mode,omitempty"`
	RRN                  string `json:"rrn,omitempty"`
	OriginalMTI          string `json:"original_mti,omitempty"`
	OriginalSTAN         string `json:"original_stan,omitempty"`
	OriginalTransmission string `json:"original_transmission,omitempty"`
}

func (r Request) MarshalJSON() ([]byte, error) {
	type alias Request
	cp := alias(r)
	cp.PAN = maskPAN(cp.PAN)
	return json.Marshal(cp)
}

func maskPAN(pan string) string {
	if pan == "" {
		return ""
	}
	if len(pan) < 10 {
		return strings.Repeat("*", len(pan))
	}
	return pan[:6] + strings.Repeat("*", len(pan)-10) + pan[len(pan)-4:]
}

// Reply is the host answer.
//
// TODO(poc): Silent exists only to fake a host that never writes its answer.
// Amount 500 sets it on a purchase, amount 400 sets it on the first reversal.
// Delete the field once a real host always sends the response it decided.
type Reply struct {
	MTI            string `json:"mti,omitempty"`
	ProcessingCode string `json:"processing_code,omitempty"`
	Amount         int64  `json:"amount"`
	Transmission   string `json:"transmission,omitempty"`
	STAN           string `json:"stan,omitempty"`
	LocalTime      string `json:"local_time,omitempty"`
	LocalDate      string `json:"local_date,omitempty"`
	RRN            string `json:"rrn,omitempty"`
	AuthID         string `json:"auth_id,omitempty"`
	RC             string `json:"rc,omitempty"`
	TerminalID     string `json:"terminal_id,omitempty"`
	MerchantID     string `json:"merchant_id,omitempty"`
	Currency       string `json:"currency,omitempty"`
	Silent         bool   `json:"silent,omitempty"` // TODO: remove this for PROD. only for POC timeout simulation.
}

// Outcome is what the terminal keeps after one attempt.
type Outcome struct {
	Request Request
	Reply   Reply
}

// Wire is the TCP boundary. The transport implementation packs ISO 8583.
type Wire interface {
	Exchange(ctx context.Context, req Request) (Reply, error)
}
