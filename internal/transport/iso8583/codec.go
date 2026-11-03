package iso8583

import (
	"fmt"
	"strconv"
	"strings"

	"titp-poc/internal/application/transaction"
	"titp-poc/internal/domain"
	"titp-poc/internal/transport/iso8583/spec"
)

func packRequest(req transaction.Request) ([]byte, error) {
	m, err := spec.New(req.MTI)
	if err != nil {
		return nil, err
	}
	fields := map[int]string{
		2:  req.PAN,
		3:  req.ProcessingCode,
		4:  fmt.Sprintf("%012d", req.Amount),
		7:  req.Transmission,
		11: req.STAN,
		12: req.LocalTime,
		13: req.LocalDate,
		41: req.TerminalID,
		42: req.MerchantID,
		49: req.Currency,
	}
	if req.MTI == "0200" {
		fields[22] = req.EntryMode
	}
	if req.ProcessingCode == domain.ProcVoid {
		fields[37] = req.RRN
	}
	if req.MTI == "0400" {
		de90 := req.OriginalMTI + req.OriginalSTAN + req.OriginalTransmission + strings.Repeat("0", 22)
		if len(de90) != 42 {
			return nil, fmt.Errorf("DE 90 length %d", len(de90))
		}
		fields[90] = de90
	}
	for n, v := range fields {
		if v == "" {
			return nil, fmt.Errorf("request DE %d is empty", n)
		}
		if err := m.Set(n, v); err != nil {
			return nil, err
		}
	}
	return m.Pack()
}

func packReply(r transaction.Reply) ([]byte, error) {
	m, err := spec.New(r.MTI)
	if err != nil {
		return nil, err
	}
	fields := []struct {
		n int
		v string
	}{
		{3, r.ProcessingCode},
		{4, fmt.Sprintf("%012d", r.Amount)},
		{7, r.Transmission},
		{11, r.STAN},
		{12, r.LocalTime},
		{13, r.LocalDate},
		{37, r.RRN},
		{39, r.RC},
		{41, r.TerminalID},
		{42, r.MerchantID},
		{49, r.Currency},
	}
	if r.AuthID != "" {
		fields = append(fields, struct {
			n int
			v string
		}{38, r.AuthID})
	}
	for _, f := range fields {
		if f.v == "" {
			return nil, fmt.Errorf("response DE %d is empty", f.n)
		}
		if err := m.Set(f.n, f.v); err != nil {
			return nil, err
		}
	}
	return m.Pack()
}

func parseRequest(m *spec.Msg) (transaction.Request, error) {
	var req transaction.Request
	var err error
	req.MTI, err = m.MTI()
	if err != nil {
		return req, err
	}
	var amount, de90 string
	if err := copyPresent(m, map[int]*string{
		2:  &req.PAN,
		3:  &req.ProcessingCode,
		4:  &amount,
		7:  &req.Transmission,
		11: &req.STAN,
		12: &req.LocalTime,
		13: &req.LocalDate,
		22: &req.EntryMode,
		37: &req.RRN,
		41: &req.TerminalID,
		42: &req.MerchantID,
		49: &req.Currency,
		90: &de90,
	}); err != nil {
		return req, err
	}
	req.ProcessingCode = padLeft(req.ProcessingCode, 6)
	if amount != "" {
		req.Amount, err = strconv.ParseInt(amount, 10, 64)
		if err != nil {
			return req, fmt.Errorf("DE 4: %w", err)
		}
	}
	if de90 != "" {
		if len(de90) != 42 {
			return req, fmt.Errorf("DE 90 length %d", len(de90))
		}
		req.OriginalMTI = de90[0:4]
		req.OriginalSTAN = de90[4:10]
		req.OriginalTransmission = de90[10:20]
	}
	if err := requireFields(m, req); err != nil {
		return req, err
	}
	return req, nil
}

func copyPresent(m *spec.Msg, dst map[int]*string) error {
	for n, target := range dst {
		if !m.Has(n) {
			continue
		}
		v, err := m.Get(n)
		if err != nil {
			return err
		}
		*target = v
	}
	return nil
}

func requireFields(m *spec.Msg, req transaction.Request) error {
	switch req.MTI {
	case "0200":
		need := []int{2, 3, 4, 7, 11, 12, 13, 22, 41, 42, 49}
		if req.ProcessingCode == domain.ProcVoid {
			need = append(need, 37)
		}
		return missing(m, need)
	case "0400":
		return missing(m, []int{2, 3, 4, 7, 11, 12, 13, 41, 42, 49, 90})
	default:
		return fmt.Errorf("unsupported MTI %s", req.MTI)
	}
}

func missing(m *spec.Msg, ns []int) error {
	var bad []int
	for _, n := range ns {
		if !m.Has(n) {
			bad = append(bad, n)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("missing DE %v", bad)
}

func unpackReply(m *spec.Msg) (transaction.Reply, error) {
	var r transaction.Reply
	var err error
	r.MTI, err = m.MTI()
	if err != nil {
		return r, err
	}
	r.ProcessingCode, err = m.Get(3)
	if err != nil {
		return r, err
	}
	r.ProcessingCode = padLeft(r.ProcessingCode, 6)
	amount, err := m.Get(4)
	if err != nil {
		return r, err
	}
	r.Amount, err = strconv.ParseInt(amount, 10, 64)
	if err != nil {
		return r, fmt.Errorf("DE 4: %w", err)
	}
	r.Transmission, err = m.Get(7)
	if err != nil {
		return r, err
	}
	r.STAN, err = m.Get(11)
	if err != nil {
		return r, err
	}
	r.LocalTime, err = m.Get(12)
	if err != nil {
		return r, err
	}
	r.LocalDate, err = m.Get(13)
	if err != nil {
		return r, err
	}
	r.RRN, err = m.Get(37)
	if err != nil {
		return r, err
	}
	r.RC, err = m.Get(39)
	if err != nil {
		return r, err
	}
	r.TerminalID, err = m.Get(41)
	if err != nil {
		return r, err
	}
	r.MerchantID, err = m.Get(42)
	if err != nil {
		return r, err
	}
	r.Currency, err = m.Get(49)
	if err != nil {
		return r, err
	}
	if m.Has(38) {
		r.AuthID, err = m.Get(38)
		if err != nil {
			return r, err
		}
	}
	return r, nil
}

func responseMTI(mti string) string {
	switch mti {
	case "0200":
		return "0210"
	case "0400":
		return "0410"
	default:
		return ""
	}
}

func padLeft(v string, n int) string {
	if v == "" || len(v) >= n {
		return v
	}
	return strings.Repeat("0", n-len(v)) + v
}

func fallback(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
