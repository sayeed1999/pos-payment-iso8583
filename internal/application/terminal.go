package application

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"titp-poc/internal/domain/emv"
	"titp-poc/internal/domain/pin"
	"titp-poc/internal/handlers/iso8583/spec"
	"titp-poc/internal/infrastructure"
)

type Config struct {
	Addr     string
	TID      string // DE 41, 8 chars
	MID      string // DE 42, 15 chars
	Currency string // DE 49 / EMV 5F2A, ISO 4217 numeric ("050" = BDT)
	Country  string // EMV 9F1A, ISO 3166 numeric ("050" = Bangladesh)
	Timeout  time.Duration
	Zone     *time.Location // terminal local time zone (DE 12 / 13)
	Framing  infrastructure.Framing

	OnFrame func(dir string, m *spec.Msg) // every message sent / received
	Note    func(msg string)              // simulation notes and warnings
}

func (c *Config) defaults() {
	if c.TID == "" {
		c.TID = "POS00001"
	}
	if c.MID == "" {
		c.MID = "MERCHANT0000001"
	}
	if c.Currency == "" {
		c.Currency = "050"
	}
	if c.Country == "" {
		c.Country = "050"
	}
	if c.Timeout == 0 {
		c.Timeout = 5 * time.Second
	}
	if c.Zone == nil {
		c.Zone = time.FixedZone("BDT", 6*3600)
	}
	if c.Framing.LenBytes == 0 {
		c.Framing = infrastructure.DefaultFraming
	}
}

// Card is a test card as the terminal reads it from the chip.
type Card struct {
	Name        string
	PAN         string
	Expiry      string // YYMM
	ServiceCode string
	PANSeq      string // EMV 5F34 -> DE 23
	AID         string // EMV 84
	AIP         string // EMV 82
	IAD         string // EMV 9F10
}

const (
	visaAID = "A0000000031010"
	mcAID   = "A0000000041010"
	visaIAD = "06010A03A0B800"
	mcIAD   = "0110A04000220000000000000000000000FF"
)

// Cards are public test PANs (Luhn-valid). The host decides what each one does.
var Cards = map[string]Card{
	"visa":       {"Visa credit", "4111111111111111", "2812", "201", "001", visaAID, "1980", visaIAD},
	"mastercard": {"Mastercard credit", "5555555555554444", "2812", "201", "001", mcAID, "5C00", mcIAD},
	"decline":    {"Visa, issuer declines", "4000000000000002", "2812", "201", "001", visaAID, "1980", visaIAD},
	"lostreply":  {"Visa, host reply gets lost", "4000000000000341", "2812", "201", "001", visaAID, "1980", visaIAD},
}

var ErrTimeout = errors.New("no response from host within timeout")

type Terminal struct {
	cfg      Config
	conn     net.Conn
	stan     int
	atc      int
	loseNext bool
}

func NewTerminal(cfg Config) *Terminal {
	cfg.defaults()
	return &Terminal{cfg: cfg}
}

func (t *Terminal) Config() Config { return t.cfg }

func (t *Terminal) Connect() error {
	c, err := net.DialTimeout("tcp", t.cfg.Addr, t.cfg.Timeout)
	if err != nil {
		return err
	}
	t.conn = c
	return nil
}

func (t *Terminal) Close() {
	if t.conn != nil {
		t.conn.Close()
		t.conn = nil
	}
}

// LoseNextRequest simulates the network eating the next request: the terminal
// believes it sent it, the host never sees it, and no response ever comes.
func (t *Terminal) LoseNextRequest() { t.loseNext = true }

func (t *Terminal) note(s string) {
	if t.cfg.Note != nil {
		t.cfg.Note(s)
	}
}

func (t *Terminal) nextSTAN() string {
	t.stan++
	if t.stan > 999999 {
		t.stan = 1
	}
	return fmt.Sprintf("%06d", t.stan)
}

// stamp returns DE 7 (UTC, MMDDhhmmss), the local DE 12 / DE 13 and local time.
func (t *Terminal) stamp() (de7, de12, de13 string, local time.Time) {
	now := time.Now()
	local = now.In(t.cfg.Zone)
	return now.UTC().Format("0102150405"), local.Format("150405"), local.Format("0102"), local
}

// Exchange sends one request and waits for the matching response. It refuses
// to send anything that breaks the profile.
func (t *Terminal) Exchange(req *spec.Msg) (*spec.Msg, error) {
	if res := spec.Check(req); !res.OK() {
		return nil, fmt.Errorf("refusing to send, message breaks the profile: %s", res)
	}
	payload, err := req.Pack()
	if err != nil {
		return nil, err
	}
	t.trace("TERMINAL -> HOST", req)

	if t.loseNext {
		t.loseNext = false
		t.note("SIMULATION: the request is lost in the network, it never reaches the host")
		time.Sleep(t.cfg.Timeout)
		return nil, ErrTimeout
	}

	if t.conn == nil {
		if err := t.Connect(); err != nil {
			return nil, err
		}
	}
	t.conn.SetDeadline(time.Now().Add(t.cfg.Timeout))
	if _, err := t.conn.Write(t.cfg.Framing.Encode(payload)); err != nil {
		t.Close()
		return nil, err
	}
	body, err := t.cfg.Framing.Read(t.conn, 4096)
	if err != nil {
		t.Close() // after a timeout the connection state is unknown: start clean
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return nil, ErrTimeout
		}
		return nil, err
	}
	resp, err := spec.Unpack(body)
	if err != nil {
		return nil, fmt.Errorf("cannot parse host response: %w", err)
	}
	t.trace("HOST -> TERMINAL", resp)
	if resp.MTI() != spec.ResponseMTI(req.MTI()) || resp.Get(11) != req.Get(11) || resp.Get(41) != req.Get(41) {
		return resp, fmt.Errorf("response does not match request (MTI %s, STAN %s)", resp.MTI(), resp.Get(11))
	}
	if res := spec.Check(resp); !res.OK() {
		t.note("WARNING: host response deviates from profile: " + res.String())
	}
	return resp, nil
}

func (t *Terminal) trace(dir string, m *spec.Msg) {
	if t.cfg.OnFrame != nil {
		t.cfg.OnFrame(dir, m)
	}
}

type Result struct {
	Req      *spec.Msg
	Resp     *spec.Msg
	RC       string
	Err      error
	TimedOut bool
}

func (r *Result) Approved() bool { return r.Err == nil && r.RC == "00" }

func result(req, resp *spec.Msg, err error) *Result {
	r := &Result{Req: req, Resp: resp, Err: err, TimedOut: errors.Is(err, ErrTimeout)}
	if resp != nil {
		r.RC = resp.Get(39)
	}
	return r
}

// Purchase builds and sends a chip 0200. amount is in minor units
// (125000 = 1250.00). pinCode is what the cardholder typed, or "" for none.
func (t *Terminal) Purchase(card Card, amount int64, pinCode string) *Result {
	de7, de12, de13, local := t.stamp()
	req := spec.New("0200").
		Set(3, "000000"). // purchase, account type unspecified
		Set(4, fmt.Sprintf("%012d", amount)).
		Set(7, de7).
		Set(11, t.nextSTAN()).
		Set(12, de12).
		Set(13, de13).
		Set(22, "051"). // 05 = chip, 1 = terminal has a PIN pad
		Set(23, card.PANSeq).
		Set(25, "00"). // normal presentment
		Set(35, card.PAN+"="+card.Expiry+card.ServiceCode+"0000000000").
		Set(41, t.cfg.TID).
		Set(42, t.cfg.MID).
		Set(49, t.cfg.Currency)

	blob, err := t.emvData(card, amount, local, pinCode != "")
	if err != nil {
		return result(req, nil, err)
	}
	req.Set(55, strings.ToUpper(hex.EncodeToString(blob)))

	if pinCode != "" {
		blk, err := pin.Encrypt(pinCode, card.PAN)
		if err != nil {
			return result(req, nil, err)
		}
		req.SetBytes(52, blk)
	}
	resp, err := t.Exchange(req)
	return result(req, resp, err)
}

// Reverse sends a 0400 that undoes a purchase. It uses a NEW STAN and points at
// the original through DE 90 = original MTI + STAN + DE 7 + acquirer ID +
// forwarding ID (the last two are zeros: the terminal never learnt them).
//
// origResp is the 0210 if the terminal received one (card refused the approval
// afterwards), or nil if it never did (timeout). Only when the terminal knows
// the RRN does it send DE 37.
func (t *Terminal) Reverse(orig, origResp *spec.Msg, card Card) *Result {
	de7, de12, de13, _ := t.stamp()
	req := spec.New("0400").
		Set(2, card.PAN).
		Set(3, orig.Get(3)).
		Set(4, orig.Get(4)).
		Set(7, de7).
		Set(11, t.nextSTAN()).
		Set(12, de12).
		Set(13, de13).
		Set(14, card.Expiry).
		Set(22, orig.Get(22)).
		Set(25, orig.Get(25)).
		Set(41, t.cfg.TID).
		Set(42, t.cfg.MID).
		Set(49, orig.Get(49)).
		Set(90, orig.MTI()+orig.Get(11)+orig.Get(7)+strings.Repeat("0", 22))
	if origResp != nil {
		req.Set(37, origResp.Get(37))
	}
	resp, err := t.Exchange(req)
	return result(req, resp, err)
}

func (t *Terminal) emvData(card Card, amount int64, local time.Time, pinUsed bool) ([]byte, error) {
	t.atc++
	cvm := "1F0302" // no CVM required
	if pinUsed {
		cvm = "420300" // enciphered PIN verified online
	}
	tlvs := []emv.TLV{
		{Tag: "9F26", Value: randBytes(8)}, // fake ARQC: random, not a real cryptogram
		{Tag: "9F27", Value: mustHex("80")},
		{Tag: "9F10", Value: mustHex(card.IAD)},
		{Tag: "9F37", Value: randBytes(4)},
		{Tag: "9F36", Value: []byte{byte(t.atc >> 8), byte(t.atc)}},
		{Tag: "95", Value: mustHex("0000000000")},
		{Tag: "9A", Value: mustHex(local.Format("060102"))},
		{Tag: "9C", Value: mustHex("00")},
		{Tag: "9F02", Value: mustHex(fmt.Sprintf("%012d", amount))},
		{Tag: "5F2A", Value: mustHex("0" + t.cfg.Currency)},
		{Tag: "82", Value: mustHex(card.AIP)},
		{Tag: "9F1A", Value: mustHex("0" + t.cfg.Country)},
		{Tag: "9F03", Value: mustHex("000000000000")},
		{Tag: "9F33", Value: mustHex("E0F8C8")},
		{Tag: "9F34", Value: mustHex(cvm)},
		{Tag: "9F35", Value: mustHex("22")},
		{Tag: "84", Value: mustHex(card.AID)},
	}
	return emv.Build(tlvs)
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(fmt.Sprintf("bad hex %q: %v", s, err))
	}
	return b
}
