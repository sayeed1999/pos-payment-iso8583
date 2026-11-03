package transaction

import "context"

// Request is one 0200 or 0400 after it has been parsed off the wire.
type Request struct {
	MTI                  string
	ProcessingCode       string
	Amount               int64
	STAN                 string
	Transmission         string
	LocalTime            string
	LocalDate            string
	TerminalID           string
	MerchantID           string
	PAN                  string
	Currency             string
	EntryMode            string
	RRN                  string
	OriginalMTI          string
	OriginalSTAN         string
	OriginalTransmission string
}

// Reply is the host answer.
//
// TODO(poc): Silent exists only to fake a host that never writes its answer.
// Amount 500 sets it on a purchase, amount 400 sets it on the first reversal.
// Delete the field once a real host always sends the response it decided.
type Reply struct {
	MTI            string
	ProcessingCode string
	Amount         int64
	Transmission   string
	STAN           string
	LocalTime      string
	LocalDate      string
	RRN            string
	AuthID         string
	RC             string
	TerminalID     string
	MerchantID     string
	Currency       string
	Silent         bool // TODO: remove this for PROD. only for POC timeout simulation.
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
