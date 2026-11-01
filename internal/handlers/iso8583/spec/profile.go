package spec

// Field rules for a card-present chip purchase and its reversal.
//
//   - Required: always sent. A missing one is a format error (host answers 30).
//   - Optional: sent only when applicable (for example the PIN block only when
//     the cardholder typed a PIN).
//   - Anything else must NOT be sent and is reported as "unexpected".
//
// Choices that mirror how Visa / Mastercard authorisations work:
//   - With a chip card the PAN and expiry travel INSIDE DE 35 (track 2
//     equivalent data), so DE 2 and DE 14 are not repeated in the 0200.
//   - DE 37 (RRN), DE 38 (auth ID) and DE 39 (response code) are created by the
//     host, so a terminal never sends them in a 0200.
//   - DE 18 (MCC), DE 32 (acquirer ID) and DE 43 (merchant name/location) are
//     added by the acquirer host from the merchant profile; a terminal does
//     not send them.
//   - A reversal has no track data, so it carries the PAN in DE 2 (+ DE 14), and
//     DE 90 pointing at the original. DE 37 is sent only when the terminal has
//     already received the 0210 (so it knows the RRN).

import (
	"fmt"
	"sort"
	"strings"
)

// Fields is what a field check needs to know about a message. *Msg satisfies it.
type Fields interface {
	MTI() string
	Has(n int) bool
	Get(n int) string
	Numbers() []int
}

// Names of the data elements this PoC uses.
var Names = map[int]string{
	2:  "Primary account number",
	3:  "Processing code",
	4:  "Amount, transaction",
	7:  "Transmission date & time (UTC)",
	11: "STAN",
	12: "Local transaction time",
	13: "Local transaction date",
	14: "Expiration date",
	22: "POS entry mode",
	23: "Card sequence number",
	25: "POS condition code",
	35: "Track 2 data",
	37: "Retrieval reference number",
	38: "Authorization ID response",
	39: "Response code",
	41: "Terminal ID",
	42: "Merchant ID",
	49: "Currency code, transaction",
	52: "PIN data (encrypted)",
	55: "ICC / EMV data",
	90: "Original data elements",
}

type Rule struct {
	Name  string
	MTI   string
	Req   []int
	Opt   []int
	match func(m Fields) bool
}

var rsp = []int{3, 4, 7, 11, 12, 13, 37, 39, 41, 42, 49}

var Rules = []Rule{
	{"0200 Purchase request (chip)", "0200",
		[]int{3, 4, 7, 11, 12, 13, 22, 23, 25, 35, 41, 42, 49, 55}, []int{52},
		func(Fields) bool { return true }},
	{"0210 Purchase response, approved", "0210",
		append([]int{38}, rsp...), []int{55},
		func(m Fields) bool { return m.Get(39) == "00" }},
	{"0210 Purchase response, declined", "0210",
		rsp, []int{55},
		func(m Fields) bool { return m.Get(39) != "00" }},
	{"0400 Reversal request", "0400",
		[]int{2, 3, 4, 7, 11, 12, 13, 22, 25, 41, 42, 49, 90}, []int{14, 37},
		func(Fields) bool { return true }},
	{"0410 Reversal response", "0410",
		rsp, nil,
		func(Fields) bool { return true }},
}

// Find returns the rule that applies to m, or nil.
func Find(m Fields) *Rule {
	for i := range Rules {
		if Rules[i].MTI == m.MTI() && Rules[i].match(m) {
			return &Rules[i]
		}
	}
	return nil
}

type Result struct {
	Rule       *Rule
	MTI        string
	Missing    []int // required but absent
	Unexpected []int // present but not in the profile
}

func (r Result) OK() bool { return r.Rule != nil && len(r.Missing) == 0 && len(r.Unexpected) == 0 }

func (r Result) String() string {
	if r.Rule == nil {
		return fmt.Sprintf("no profile for MTI %s", r.MTI)
	}
	var parts []string
	if len(r.Missing) > 0 {
		parts = append(parts, fmt.Sprintf("missing required DE %v", r.Missing))
	}
	if len(r.Unexpected) > 0 {
		parts = append(parts, fmt.Sprintf("unexpected DE %v", r.Unexpected))
	}
	if len(parts) == 0 {
		return "ok (" + r.Rule.Name + ")"
	}
	return strings.Join(parts, "; ") + " [" + r.Rule.Name + "]"
}

// Check compares a message against the profile.
func Check(m Fields) Result {
	res := Result{MTI: m.MTI(), Rule: Find(m)}
	if res.Rule == nil {
		return res
	}
	allowed := map[int]bool{}
	for _, n := range res.Rule.Req {
		allowed[n] = true
		if !m.Has(n) {
			res.Missing = append(res.Missing, n)
		}
	}
	for _, n := range res.Rule.Opt {
		allowed[n] = true
	}
	for _, n := range m.Numbers() {
		if !allowed[n] {
			res.Unexpected = append(res.Unexpected, n)
		}
	}
	return res
}

func list(v []int) string {
	if len(v) == 0 {
		return "none"
	}
	c := append([]int{}, v...)
	sort.Ints(c)
	s := make([]string, len(c))
	for i, n := range c {
		s[i] = fmt.Sprint(n)
	}
	return strings.Join(s, ", ")
}

// Table renders the profile as Markdown.
func Table() string {
	var b strings.Builder
	b.WriteString("| Message | Required DEs (always sent) | Optional DEs (only when applicable) |\n")
	b.WriteString("|---|---|---|\n")
	used := map[int]bool{}
	for _, r := range Rules {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", r.Name, list(r.Req), list(r.Opt))
		for _, n := range append(append([]int{}, r.Req...), r.Opt...) {
			used[n] = true
		}
	}
	b.WriteString("\n| DE | Name |\n|---|---|\n")
	var all []int
	for n := range used {
		all = append(all, n)
	}
	sort.Ints(all)
	for _, n := range all {
		fmt.Fprintf(&b, "| %d | %s |\n", n, Names[n])
	}
	return b.String()
}
