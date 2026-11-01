package spec

import (
	"sort"
	"testing"
)

// fake lets the profile be tested without building a packed message.
type fake struct {
	mti string
	f   map[int]string
}

func (m fake) MTI() string      { return m.mti }
func (m fake) Has(n int) bool   { _, ok := m.f[n]; return ok }
func (m fake) Get(n int) string { return m.f[n] }
func (m fake) Numbers() []int {
	var out []int
	for n := range m.f {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

func purchase() fake {
	return fake{"0200", map[int]string{
		3: "000000", 4: "000000125000", 7: "1030092215", 11: "000001", 12: "152215", 13: "1030",
		22: "051", 23: "001", 25: "00", 35: "4111111111111111=28122010000000000",
		41: "POS00001", 42: "MERCHANT0000001", 49: "050", 55: "9F270180",
	}}
}

func TestPurchaseOK(t *testing.T) {
	if res := Check(purchase()); !res.OK() {
		t.Fatal(res)
	}
}

func TestMissingAlwaysSentField(t *testing.T) {
	m := purchase()
	delete(m.f, 49) // currency is always sent
	res := Check(m)
	if res.OK() || len(res.Missing) != 1 || res.Missing[0] != 49 {
		t.Fatal(res)
	}
}

func TestFieldsThatAreNotSent(t *testing.T) {
	m := purchase()
	m.f[2] = "4111111111111111" // PAN is already inside DE 35
	m.f[37] = "123456789012"    // RRN is created by the host
	res := Check(m)
	if res.OK() || len(res.Unexpected) != 2 {
		t.Fatal(res)
	}
}

func TestPINBlockIsOptional(t *testing.T) {
	m := purchase()
	m.f[52] = "x"
	if res := Check(m); !res.OK() {
		t.Fatal(res)
	}
}

func TestApprovedResponseNeedsAuthID(t *testing.T) {
	resp := fake{"0210", map[int]string{
		3: "000000", 4: "000000125000", 7: "1030092215", 11: "000001", 12: "152215", 13: "1030",
		37: "630301000001", 39: "00", 41: "POS00001", 42: "MERCHANT0000001", 49: "050",
	}}
	if res := Check(resp); res.OK() || res.Missing[0] != 38 {
		t.Fatalf("approved response without DE 38 passed: %s", res)
	}
	resp.f[39] = "05" // a decline carries no auth ID
	if res := Check(resp); !res.OK() {
		t.Fatal(res)
	}
}

func TestReversalRRNIsOptional(t *testing.T) {
	m := fake{"0400", map[int]string{
		2: "4111111111111111", 3: "000000", 4: "000000125000", 7: "1030092300", 11: "000002",
		12: "152300", 13: "1030", 22: "051", 25: "00", 41: "POS00001", 42: "MERCHANT0000001",
		49: "050", 90: "0200000001103009221500000000000000000000000000",
	}}
	if res := Check(m); !res.OK() {
		t.Fatal(res)
	}
	m.f[37] = "630301000001"
	if res := Check(m); !res.OK() {
		t.Fatal(res)
	}
	m.f[35] = "4111111111111111=2812" // no track data in a reversal
	if res := Check(m); res.OK() {
		t.Fatal("track 2 in a reversal passed")
	}
}
