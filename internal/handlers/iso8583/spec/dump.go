package spec

import (
	"encoding/hex"
	"fmt"
	"strings"

	"titp-poc/internal/domain"
	"titp-poc/internal/domain/emv"
)

var mtiNames = map[string]string{
	"0200": "Purchase request",
	"0210": "Purchase response",
	"0400": "Reversal request",
	"0410": "Reversal response",
}

func MTIName(mti string) string {
	if n, ok := mtiNames[mti]; ok {
		return n
	}
	return "unknown"
}

func maskTrack2(t string) string {
	i := strings.IndexAny(t, "=D")
	if i <= 0 {
		return strings.Repeat("*", len(t))
	}
	return domain.MaskPAN(t[:i]) + "=" + strings.Repeat("*", len(t)-i-1)
}

// Dump prints a field table. Card data (DE 2, 14, 35, 52) is masked.
func Dump(m *Msg) string {
	var b strings.Builder
	for _, n := range m.Numbers() {
		name := Names[n]
		if name == "" {
			name = "?"
		}
		fmt.Fprintf(&b, "    DE %03d  %-30s %s\n", n, name, show(n, m))
	}
	return b.String()
}

func show(n int, m *Msg) string {
	switch n {
	case 2:
		return `"` + domain.MaskPAN(m.Get(2)) + `"`
	case 14:
		return `"****"`
	case 35:
		return `"` + maskTrack2(m.Get(35)) + `"`
	case 52:
		return "<encrypted PIN block, 8 bytes>"
	case 55:
		return emvLines(m.Get(55))
	}
	return `"` + m.Get(n) + `"`
}

func emvLines(hexStr string) string {
	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		return "<DE 55 is not a hex string>"
	}
	tlvs, err := emv.Parse(raw)
	if err != nil {
		return fmt.Sprintf("%d bytes (unparseable TLV: %v)", len(raw), err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d bytes of EMV TLV", len(raw))
	for _, t := range tlvs {
		fmt.Fprintf(&b, "\n              %-5s %-38s %X", t.Tag, emv.Names[t.Tag], t.Value)
	}
	return b.String()
}
