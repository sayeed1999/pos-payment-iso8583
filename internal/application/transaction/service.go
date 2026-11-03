package transaction

import (
	"context"
	"fmt"
	"sync"
	"time"

	"titp-poc/internal/domain"
	"titp-poc/logger"
)

// Directory is the terminal and merchant lookup. The infrastructure store
// implements it.
type Directory interface {
	Terminal(id string) (name string, active, found bool)
	Merchant(id string) (name string, active, found bool)
}

type sale struct {
	reply    Reply
	reversed bool
}

// Service decides purchase, void, and reversal.
type Service struct {
	dir Directory

	mu sync.Mutex
	// TODO(poc): these maps are the demo's memory of a sale. A restart forgets
	// them, so a retried 0200 or 0400 looks like a new transaction and can
	// approve twice. Replace with a store that survives restart.
	sales     map[string]sale
	byRRN     map[string]string
	voids     map[string]Reply
	reversals map[string]Reply
	rrnSeq    int
	authSeq   int
}

func NewService(dir Directory) *Service {
	return &Service{
		dir:       dir,
		sales:     map[string]sale{},
		byRRN:     map[string]string{},
		voids:     map[string]Reply{},
		reversals: map[string]Reply{},
	}
}

func saleKey(tid, stan, transmission string) string {
	return tid + "|" + stan + "|" + transmission
}

// Authorize handles a 0200. Caller does not hold s.mu.
func (s *Service) Authorize(ctx context.Context, req Request) Reply {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rc, ok := s.parties(ctx, req); !ok {
		return s.echo(req, "0210", rc, false)
	}
	switch req.ProcessingCode {
	case domain.ProcVoid:
		return s.void(ctx, req)
	case domain.ProcPurchase:
		return s.purchase(ctx, req)
	default:
		logger.Error(ctx, "unknown processing code", "processing_code", req.ProcessingCode)
		return s.echo(req, "0210", domain.RCFormat, false)
	}
}

// Reverse handles a 0400. A reversal is accepted even when the original sale
// never arrived. Amount 400 withholds the first 0410 so the terminal can resend that 0400.
func (s *Service) Reverse(ctx context.Context, req Request) Reply {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rc, ok := s.parties(ctx, req); !ok {
		return s.echo(req, "0410", rc, false)
	}
	key := saleKey(req.TerminalID, req.STAN, req.Transmission)
	if prev, ok := s.reversals[key]; ok {
		prev.Silent = false
		s.reversals[key] = prev
		logger.Info(ctx, "duplicate reversal, replaying", "stan", req.STAN, "rc", prev.RC, "rrn", prev.RRN)
		return prev
	}

	origKey := saleKey(req.TerminalID, req.OriginalSTAN, req.OriginalTransmission)
	orig, found := s.sales[origKey]
	var originalRRN string
	if !found {
		logger.Info(ctx, "original sale not found", "original_stan", req.OriginalSTAN)
	} else {
		originalRRN = orig.reply.RRN
		if orig.reply.Amount != req.Amount {
			logger.Error(ctx, "reversal amount differs from original", "original_amount", orig.reply.Amount, "reversal_amount", req.Amount)
		}
		switch {
		case orig.reversed:
			logger.Info(ctx, "original sale already reversed", "original_stan", req.OriginalSTAN)
		case orig.reply.RC != domain.RCApproved:
			logger.Info(ctx, "original sale was not approved", "original_stan", req.OriginalSTAN, "rc", orig.reply.RC)
		default:
			orig.reversed = true
			s.sales[origKey] = orig
			logger.Info(ctx, "reversal released hold", "original_stan", req.OriginalSTAN, "amount", orig.reply.Amount, "auth_id", orig.reply.AuthID)
		}
	}

	// TODO(poc): amount 400 is not a reversal rule. It drops the first 0410 so
	// `pos-terminal -reverse -amount 400` can resend the same 0400. Delete this
	// and always answer.
	silent := false
	if req.Amount == 400 {
		silent = true
	}
	reply := s.echo(req, "0410", domain.RCApproved, silent)
	if originalRRN != "" {
		reply.RRN = originalRRN
	}
	s.reversals[key] = reply
	logger.Info(ctx, "reversal decided", "stan", req.STAN, "rc", reply.RC, "rrn", reply.RRN, "withhold", silent)
	return reply
}

// caller holds s.mu.
func (s *Service) parties(ctx context.Context, req Request) (string, bool) {
	name, active, found := s.dir.Terminal(req.TerminalID)
	if !found || !active {
		logger.Error(ctx, "terminal rejected", "terminal_id", req.TerminalID, "found", found, "active", active)
		return domain.RCTerminal, false
	}
	logger.Info(ctx, "terminal accepted", "terminal_id", req.TerminalID, "terminal_name", name)

	name, active, found = s.dir.Merchant(req.MerchantID)
	if !found || !active {
		logger.Error(ctx, "merchant rejected", "merchant_id", req.MerchantID, "found", found, "active", active)
		return domain.RCMerchant, false
	}
	logger.Info(ctx, "merchant accepted", "merchant_id", req.MerchantID, "merchant_name", name)
	return "", true
}

// caller holds s.mu.
func (s *Service) purchase(ctx context.Context, req Request) Reply {
	key := saleKey(req.TerminalID, req.STAN, req.Transmission)
	if rec, ok := s.sales[key]; ok {
		logger.Info(ctx, "duplicate purchase, replaying", "stan", req.STAN, "rc", rec.reply.RC)
		return rec.reply
	}
	// TODO(poc): stand-in for the issuer. The amount picks the outcome so the
	// terminal demo can force each journey. Replace with the issuer's decision.
	//   5600 -> RC 56 insufficient funds
	//   9600 -> RC 96 system failure
	//   500  -> approve, but do not write the 0210
	//   anything else -> RC 00
	rc := domain.RCApproved
	withhold := false
	if req.Amount == 5600 {
		rc = domain.RCInsufficientFunds
	} else if req.Amount == 9600 {
		rc = domain.RCSystemFailure
	} else if req.Amount == 500 {
		withhold = true
	}
	reply := s.echo(req, "0210", rc, withhold)
	if rc == domain.RCApproved {
		reply.AuthID = s.nextAuth()
	}
	s.sales[key] = sale{reply: reply}
	s.byRRN[reply.RRN] = key
	logger.Info(ctx, "purchase decided", "stan", req.STAN, "amount", req.Amount, "rc", rc, "rrn", reply.RRN, "withhold", withhold)
	return reply
}

// caller holds s.mu.
func (s *Service) void(ctx context.Context, req Request) Reply {
	key := saleKey(req.TerminalID, req.STAN, req.Transmission)
	if prev, ok := s.voids[key]; ok {
		logger.Info(ctx, "duplicate void, replaying", "stan", req.STAN, "rc", prev.RC)
		return prev
	}
	if origKey, ok := s.byRRN[req.RRN]; ok {
		rec := s.sales[origKey]
		logger.Info(ctx, "void matched sale", "rrn", req.RRN, "original_stan", rec.reply.STAN, "original_rc", rec.reply.RC)
	} else {
		// TODO(poc): an unmatched void is approved so `-void` always prints RC 00.
		// Decline it once the host requires a live approved sale with the same
		// terminal, amount, and RRN.
		logger.Info(ctx, "void has no matching sale, approving anyway", "rrn", req.RRN)
	}
	reply := s.echo(req, "0210", domain.RCApproved, false)
	reply.AuthID = s.nextAuth()
	s.voids[key] = reply
	logger.Info(ctx, "void approved", "stan", req.STAN, "rc", reply.RC, "rrn", reply.RRN)
	return reply
}

// caller holds s.mu.
func (s *Service) echo(req Request, mti, rc string, silent bool) Reply {
	return Reply{
		MTI:            mti,
		ProcessingCode: req.ProcessingCode,
		Amount:         req.Amount,
		Transmission:   req.Transmission,
		STAN:           req.STAN,
		LocalTime:      req.LocalTime,
		LocalDate:      req.LocalDate,
		RRN:            s.rrn(req),
		RC:             rc,
		TerminalID:     req.TerminalID,
		MerchantID:     req.MerchantID,
		Currency:       req.Currency,
		Silent:         silent,
	}
}

// rrn keeps DE 37 when the request already has one (void). A purchase has none, so the host assigns it.
// caller holds s.mu.
func (s *Service) rrn(req Request) string {
	if req.RRN != "" {
		return req.RRN
	}
	return s.nextRRN()
}

// TODO(poc): the counter is in memory. After a restart the next RRN can repeat
// one already given to a sale that can still be voided. Persist the counter
// with the sale store.
// caller holds s.mu.
func (s *Service) nextRRN() string {
	now := time.Now().UTC()
	s.rrnSeq++
	if s.rrnSeq > 999999 {
		s.rrnSeq = 1
	}
	return fmt.Sprintf("%d%03d%02d%06d", now.Year()%10, now.YearDay(), now.Hour(), s.rrnSeq)
}

// caller holds s.mu.
func (s *Service) nextAuth() string {
	s.authSeq++
	if s.authSeq > 999999 {
		s.authSeq = 1
	}
	return fmt.Sprintf("%06d", s.authSeq)
}
