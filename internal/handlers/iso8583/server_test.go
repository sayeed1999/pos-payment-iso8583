package iso8583_test

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"titp-poc/internal/application"
	"titp-poc/internal/domain/emv"
	"titp-poc/internal/handlers/iso8583"
	"titp-poc/internal/handlers/iso8583/spec"
	"titp-poc/internal/infrastructure"
)

type env struct {
	srv  *iso8583.Server
	term *application.Terminal
	mu   sync.Mutex
	logs []string
}

func (e *env) hostLog() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.Join(e.logs, "\n")
}

func start(t *testing.T) *env {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	app := application.NewHost()
	e := &env{srv: iso8583.New(app)}
	app.Logf = func(f string, a ...any) {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.logs = append(e.logs, strings.TrimSpace(fmt.Sprintf(f, a...)))
	}
	go e.srv.Serve(l)
	t.Cleanup(func() { l.Close() })

	e.term = application.NewTerminal(application.Config{
		Addr:    l.Addr().String(),
		Timeout: 300 * time.Millisecond,
		Note: func(m string) {
			if strings.HasPrefix(m, "WARNING") {
				t.Error(m)
			}
		},
	})
	t.Cleanup(e.term.Close)
	return e
}

func TestPurchaseOutcomes(t *testing.T) {
	e := start(t)
	cases := []struct {
		name string
		card string
		pin  string
		rc   string
	}{
		{"approved visa", "visa", "1234", "00"},
		{"approved mastercard, no PIN", "mastercard", "", "00"},
		{"wrong pin", "visa", "9999", "55"},
		{"issuer decline", "decline", "1234", "05"},
	}
	for _, c := range cases {
		r := e.term.Purchase(application.Cards[c.card], 125000, c.pin)
		if r.Err != nil || r.RC != c.rc {
			t.Errorf("%s: RC %q err %v, want %s", c.name, r.RC, r.Err, c.rc)
			continue
		}
		if res := spec.Check(r.Resp); !res.OK() {
			t.Errorf("%s: response breaks the profile: %s", c.name, res)
		}
	}
}

func TestApprovalReturnsIssuerAuthData(t *testing.T) {
	e := start(t)
	r := e.term.Purchase(application.Cards["visa"], 125000, "1234")
	if !r.Approved() {
		t.Fatalf("%+v", r)
	}
	tlvs, err := emv.Parse(mustDecodeHex(t, r.Resp.Get(55)))
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := emv.Find(tlvs, "91"); !ok || len(v) != 10 {
		t.Fatalf("tag 91 missing or wrong size: % X", v)
	}
}

func TestDuplicateRequestReplaysTheOriginalResponse(t *testing.T) {
	e := start(t)
	r := e.term.Purchase(application.Cards["visa"], 125000, "1234")
	again, err := e.term.Exchange(r.Req)
	if err != nil || again.Get(37) != r.Resp.Get(37) || again.Get(38) != r.Resp.Get(38) {
		t.Fatalf("replay differs: %v err %v", again, err)
	}
}

// Journey: host approves, reply is lost, terminal times out, terminal reverses.
func TestReplyLostThenReversal(t *testing.T) {
	e := start(t)
	card := application.Cards["lostreply"]

	r := e.term.Purchase(card, 125000, "1234")
	if !r.TimedOut {
		t.Fatalf("expected a timeout, got RC %q err %v", r.RC, r.Err)
	}
	tid, stan, de7 := "POS00001", r.Req.Get(11), r.Req.Get(7)
	if found, approved, reversed := e.srv.App.Lookup(tid, stan, de7); !found || !approved || reversed {
		t.Fatalf("host state before reversal: found=%v approved=%v reversed=%v", found, approved, reversed)
	}

	rev := e.term.Reverse(r.Req, nil, card)
	if rev.Err != nil || rev.RC != "00" {
		t.Fatalf("reversal: RC %q err %v", rev.RC, rev.Err)
	}
	if _, _, reversed := e.srv.App.Lookup(tid, stan, de7); !reversed {
		t.Fatal("host did not mark the original as reversed")
	}
	if rev.Req.Get(11) == r.Req.Get(11) {
		t.Fatal("a reversal must use a new STAN")
	}
	if rev.Req.Has(37) {
		t.Fatal("terminal never saw an RRN, so the reversal must not carry DE 37")
	}
	if od := rev.Req.Get(90); od[:4] != "0200" || od[4:10] != stan || od[10:20] != de7 {
		t.Fatalf("DE 90 does not point at the original: %s", od)
	}
}

// Journey: the request never reaches the host, terminal times out, reverses anyway.
func TestRequestLostThenReversal(t *testing.T) {
	e := start(t)
	card := application.Cards["visa"]

	e.term.LoseNextRequest()
	r := e.term.Purchase(card, 125000, "1234")
	if !r.TimedOut {
		t.Fatalf("expected a timeout, got RC %q err %v", r.RC, r.Err)
	}
	if found, _, _ := e.srv.App.Lookup("POS00001", r.Req.Get(11), r.Req.Get(7)); found {
		t.Fatal("host should never have seen this purchase")
	}
	rev := e.term.Reverse(r.Req, nil, card)
	if rev.Err != nil || rev.RC != "00" {
		t.Fatalf("host must still accept the reversal: RC %q err %v", rev.RC, rev.Err)
	}
	if !strings.Contains(e.hostLog(), "never reached the host") {
		t.Fatalf("host log should say there was nothing to undo:\n%s", e.hostLog())
	}
}

// Journey: host approved, terminal received 0210, then the card refused. Reverse with DE 37.
func TestCardRefusesAfterApprovalThenReversal(t *testing.T) {
	e := start(t)
	card := application.Cards["visa"]

	r := e.term.Purchase(card, 125000, "1234")
	if !r.Approved() {
		t.Fatalf("%+v", r)
	}
	rev := e.term.Reverse(r.Req, r.Resp, card)
	if rev.Err != nil || rev.RC != "00" {
		t.Fatalf("reversal: RC %q err %v", rev.RC, rev.Err)
	}
	if rev.Req.Get(37) != r.Resp.Get(37) || rev.Resp.Get(37) != r.Resp.Get(37) {
		t.Fatalf("RRN must carry through: purchase %s, reversal req %s, reversal resp %s",
			r.Resp.Get(37), rev.Req.Get(37), rev.Resp.Get(37))
	}
	if _, _, reversed := e.srv.App.Lookup("POS00001", r.Req.Get(11), r.Req.Get(7)); !reversed {
		t.Fatal("host did not mark the original as reversed")
	}
	// A repeated reversal (terminal did not get the 0410) must still be answered 00.
	again := e.term.Reverse(r.Req, r.Resp, card)
	if again.Err != nil || again.RC != "00" || !strings.Contains(e.hostLog(), "already reversed") {
		t.Fatalf("second reversal: RC %q err %v", again.RC, again.Err)
	}
}

func TestMissingMandatoryFieldGetsFormatError(t *testing.T) {
	e := start(t)
	c, err := net.Dial("tcp", e.term.Config().Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	bad := spec.New("0200").Set(3, "000000").Set(4, "000000001000").Set(7, "1030092215").
		Set(11, "000009").Set(22, "051").Set(41, "POS00001") // missing 12, 13, 23, 25, 35, 42, 49, 55
	payload, err := bad.Pack()
	if err != nil {
		t.Fatal(err)
	}
	c.Write(infrastructure.DefaultFraming.Encode(payload))
	c.SetReadDeadline(time.Now().Add(time.Second))
	body, err := infrastructure.DefaultFraming.Read(c, 4096)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := spec.Unpack(body)
	if err != nil || resp.MTI() != "0210" || resp.Get(39) != "30" {
		t.Fatalf("%v %v", resp, err)
	}
}

func mustDecodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
