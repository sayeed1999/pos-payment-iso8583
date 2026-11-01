// Package application is the purchase and reversal use cases, on both sides
// of the socket: the acquirer host, and the mock terminal that drives it.
package application

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"titp-poc/internal/domain"
)

type Host struct {
	Logf func(format string, args ...any)

	mu      sync.Mutex
	txns    map[string]*txn
	rrnSeq  int
	authSeq int
}

type txn struct {
	resp     Reply
	amount   string
	reversed bool
}

// Purchase is one 0200, already pulled out of the ISO message.
type Purchase struct {
	TID, STAN, Transmission string
	Amount                  string
	PAN, Expiry             string
	PINBlock                []byte // nil when DE 52 is absent
	ICC                     string
	HasICC                  bool
	ProcessingCode          string
	LocalTime, LocalDate    string
	MID, Currency           string
}

// Reversal is one 0400.
type Reversal struct {
	TID            string
	RRN            string
	Original       string // DE 90
	ProcessingCode string
	Amount         string
	Transmission   string
	STAN           string
	LocalTime      string
	LocalDate      string
	MID, Currency  string
}

// Reply is the host answer, independent of how it is packed onto the wire.
type Reply struct {
	MTI                    string
	ProcessingCode, Amount string
	Transmission, STAN     string
	LocalTime, LocalDate   string
	RRN, AuthID, RC        string
	TID, MID, Currency     string
	ICC                    string
	Drop                   bool
}

func NewHost() *Host {
	return &Host{Logf: log.Printf, txns: map[string]*txn{}}
}

func txnKey(tid, stan, de7 string) string {
	return strings.TrimSpace(tid) + "|" + stan + "|" + de7
}

func (s *Host) Purchase(peer string, in Purchase) Reply {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := txnKey(in.TID, in.STAN, in.Transmission)
	if t, ok := s.txns[key]; ok {
		s.Logf("[%s] duplicate of STAN %s, replaying the original response", peer, in.STAN)
		return t.resp
	}

	rc := domain.Authorise(in.PAN, in.Expiry, in.Amount, in.PINBlock)
	resp := Reply{
		MTI:            "0210",
		ProcessingCode: in.ProcessingCode,
		Amount:         in.Amount,
		Transmission:   in.Transmission,
		STAN:           in.STAN,
		LocalTime:      in.LocalTime,
		LocalDate:      in.LocalDate,
		RRN:            s.nextRRN(),
		RC:             rc,
		TID:            in.TID,
		MID:            in.MID,
		Currency:       in.Currency,
	}
	if rc == "00" {
		s.authSeq++
		resp.AuthID = fmt.Sprintf("%06d", 100000+s.authSeq)
	}
	if rc == "00" && in.HasICC {
		if arpc, err := domain.IssuerAuth(in.ICC); err == nil {
			resp.ICC = arpc
		}
	}
	s.txns[key] = &txn{resp: resp, amount: in.Amount}
	s.Logf("[%s] purchase %s amount %s -> RC %s", peer, domain.MaskPAN(in.PAN), in.Amount, rc)
	resp.Drop = rc == "00" && in.PAN == domain.PANLostReply
	return resp
}

// Reversal undoes an earlier purchase. A reversal is ALWAYS accepted: the
// terminal has to be able to clear it, whether or not the original reached us.
func (s *Host) Reversal(peer string, in Reversal) Reply {
	s.mu.Lock()
	defer s.mu.Unlock()

	rrn := in.RRN
	if od := in.Original; len(od) == 42 {
		origSTAN, origDE7 := od[4:10], od[10:20]
		switch t, ok := s.txns[txnKey(in.TID, origSTAN, origDE7)]; {
		case !ok:
			s.Logf("[%s] reversal: original STAN %s never reached the host, nothing to undo", peer, origSTAN)
		case t.reversed:
			s.Logf("[%s] reversal: original STAN %s was already reversed", peer, origSTAN)
			rrn = t.resp.RRN
		case t.resp.RC != "00":
			s.Logf("[%s] reversal: original STAN %s was declined (RC %s), nothing to undo", peer, origSTAN, t.resp.RC)
			t.reversed = true
			rrn = t.resp.RRN
		default:
			t.reversed = true
			rrn = t.resp.RRN
			s.Logf("[%s] reversal: original STAN %s approved (auth %s, amount %s) -> reversed, hold released",
				peer, origSTAN, t.resp.AuthID, t.amount)
		}
	}
	if rrn == "" {
		rrn = s.nextRRN()
	}
	return Reply{
		MTI:            "0410",
		ProcessingCode: in.ProcessingCode,
		Amount:         in.Amount,
		Transmission:   in.Transmission,
		STAN:           in.STAN,
		LocalTime:      in.LocalTime,
		LocalDate:      in.LocalDate,
		RRN:            rrn,
		RC:             "00",
		TID:            in.TID,
		MID:            in.MID,
		Currency:       in.Currency,
	}
}

// Lookup lets tests inspect host state for one purchase.
func (s *Host) Lookup(tid, stan, de7 string) (found, approved, reversed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.txns[txnKey(tid, stan, de7)]
	if !ok {
		return false, false, false
	}
	return true, t.resp.RC == "00", t.reversed
}

// nextRRN: 1-digit year + 3-digit day of year + 2-digit hour + 6-digit sequence.
// The caller must hold s.mu.
func (s *Host) nextRRN() string {
	now := time.Now().UTC()
	s.rrnSeq++
	return fmt.Sprintf("%d%03d%02d%06d", now.Year()%10, now.YearDay(), now.Hour(), s.rrnSeq)
}
