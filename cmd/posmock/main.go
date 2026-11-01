// Command posmock behaves like a chip card terminal: it opens a TCP connection
// to the host and walks through the purchase journey and the reversal journey.
// It is a simulation only.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"titp-poc/internal/application"
	"titp-poc/internal/handlers/iso8583/spec"
	"titp-poc/internal/infrastructure"
)

type outcome struct {
	name, expect, got string
	ok                bool
}

type scenario struct {
	name string
	desc string
	run  func(t *application.Terminal) outcome
}

func purchaseOnly(name, desc, cardKey, pin, wantRC string) scenario {
	return scenario{name, desc, func(t *application.Terminal) outcome {
		r := t.Purchase(application.Cards[cardKey], 125000, pin)
		got := r.RC
		if r.Err != nil {
			got = "error: " + r.Err.Error()
		}
		return outcome{name, "0210 RC " + wantRC, "0210 RC " + got, r.Err == nil && r.RC == wantRC}
	}}
}

func reversed(name string, rev *application.Result, prefix string) outcome {
	exp := prefix + "0410 RC 00"
	if rev.Err != nil {
		return outcome{name, exp, prefix + "error: " + rev.Err.Error(), false}
	}
	return outcome{name, exp, prefix + "0410 RC " + rev.RC, rev.RC == "00"}
}

var scenarios = []scenario{
	purchaseOnly("approved", "PURCHASE: chip + PIN, host approves. Journey ends here, no reversal.", "visa", "1234", "00"),
	purchaseOnly("declined", "PURCHASE: issuer declines (05). Nothing was authorised, so no reversal.", "decline", "1234", "05"),
	purchaseOnly("wrong-pin", "PURCHASE: wrong PIN (55). Nothing was authorised, so no reversal.", "visa", "9999", "55"),

	{"reply-lost", "REVERSAL: host approves but the 0210 is lost. Terminal times out, sends 0400.",
		func(t *application.Terminal) outcome {
			card := application.Cards["lostreply"]
			r := t.Purchase(card, 125000, "1234")
			if !r.TimedOut {
				return outcome{"reply-lost", "timeout, 0410 RC 00", fmt.Sprintf("no timeout (RC %q, err %v)", r.RC, r.Err), false}
			}
			fmt.Printf("\n  !! no 0210 within %s. The terminal does NOT know if the card was charged.\n"+
				"  !! It must send a reversal (0400) for STAN %s. No DE 37: it never saw an RRN.\n", t.Config().Timeout, r.Req.Get(11))
			return reversed("reply-lost", t.Reverse(r.Req, nil, card), "timeout, ")
		}},

	{"request-lost", "REVERSAL: the 0200 never reaches the host. Terminal times out, sends 0400 anyway.",
		func(t *application.Terminal) outcome {
			card := application.Cards["visa"]
			t.LoseNextRequest()
			r := t.Purchase(card, 125000, "1234")
			if !r.TimedOut {
				return outcome{"request-lost", "timeout, 0410 RC 00", fmt.Sprintf("no timeout (RC %q, err %v)", r.RC, r.Err), false}
			}
			fmt.Printf("\n  !! no 0210 within %s. The terminal cannot tell this apart from a lost reply,\n"+
				"  !! so it sends the same reversal (0400) for STAN %s. The host must accept it anyway.\n", t.Config().Timeout, r.Req.Get(11))
			return reversed("request-lost", t.Reverse(r.Req, nil, card), "timeout, ")
		}},

	{"card-refuses-after-approval", "REVERSAL: host approves (0210 RC 00) but the chip then declines at 2nd GENERATE AC.",
		func(t *application.Terminal) outcome {
			card := application.Cards["visa"]
			r := t.Purchase(card, 125000, "1234")
			if !r.Approved() {
				return outcome{"card-refuses-after-approval", "0210 RC 00, 0410 RC 00", fmt.Sprintf("purchase not approved (RC %q, err %v)", r.RC, r.Err), false}
			}
			fmt.Printf("\n  !! host approved, but the chip card answered the second GENERATE AC with AAC (declined).\n" +
				"  !! The terminal declines the sale locally and must reverse the approval. It knows the RRN, so DE 37 is sent.\n")
			return reversed("card-refuses-after-approval", t.Reverse(r.Req, r.Resp, card), "0210 RC 00, ")
		}},
}

func main() {
	addr := flag.String("addr", "localhost:8583", "host address")
	which := flag.String("scenario", "all", "scenario name, comma separated list, or 'all'")
	tid := flag.String("tid", "POS00001", "terminal ID (DE 41)")
	mid := flag.String("mid", "MERCHANT0000001", "merchant ID (DE 42)")
	timeout := flag.Duration("timeout", 2*time.Second, "how long to wait for a host response")
	raw := flag.Bool("raw", false, "also print each raw frame (UNMASKED card data)")
	showProfile := flag.Bool("profile", false, "print the field profile (which DEs are sent when) and exit")
	list := flag.Bool("list", false, "list scenarios and exit")
	flag.Parse()

	if *showProfile {
		fmt.Print(spec.Table())
		return
	}
	if *list {
		for _, s := range scenarios {
			fmt.Printf("%-28s %s\n", s.name, s.desc)
		}
		return
	}

	cfg := application.Config{Addr: *addr, TID: *tid, MID: *mid, Timeout: *timeout}
	cfg.OnFrame = func(dir string, m *spec.Msg) {
		fmt.Printf("\n%s  %s %s\n", dir, m.MTI(), spec.MTIName(m.MTI()))
		fmt.Print(spec.Dump(m))
		if *raw {
			if payload, err := m.Pack(); err == nil {
				fmt.Println("    raw frame (unmasked):", infrastructure.RawFrame(infrastructure.DefaultFraming, payload))
			}
		}
	}
	cfg.Note = func(msg string) { fmt.Println("\n  " + msg) }
	t := application.NewTerminal(cfg)
	defer t.Close()

	wanted := map[string]bool{}
	for _, n := range strings.Split(*which, ",") {
		wanted[strings.TrimSpace(n)] = true
	}
	var results []outcome
	for _, s := range scenarios {
		if !wanted["all"] && !wanted[s.name] {
			continue
		}
		fmt.Printf("\n==================== %s ====================\n%s\n", s.name, s.desc)
		results = append(results, s.run(t))
	}
	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "no scenario matched; use -list")
		os.Exit(2)
	}

	fmt.Printf("\n==================== summary ====================\n")
	failed := 0
	for _, r := range results {
		status := "PASS"
		if !r.ok {
			status, failed = "FAIL", failed+1
		}
		fmt.Printf("  %-28s expected %-26s got %-26s %s\n", r.name, r.expect, r.got, status)
	}
	if failed > 0 {
		os.Exit(1)
	}
}
