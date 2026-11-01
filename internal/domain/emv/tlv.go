// Package emv reads and writes the flat BER-TLV blob carried in DE 55.
package emv

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// TLV is one EMV data object. Tag is upper-case hex, e.g. "9F26".
type TLV struct {
	Tag   string
	Value []byte
}

var errTrunc = errors.New("truncated TLV data")

// Parse decodes a flat sequence of TLVs.
func Parse(b []byte) ([]TLV, error) {
	var out []TLV
	for i := 0; i < len(b); {
		start := i
		first := b[i]
		i++
		if first&0x1F == 0x1F { // multi-byte tag
			for {
				if i >= len(b) {
					return nil, errTrunc
				}
				c := b[i]
				i++
				if c&0x80 == 0 {
					break
				}
			}
		}
		tag := strings.ToUpper(hex.EncodeToString(b[start:i]))
		if i >= len(b) {
			return nil, errTrunc
		}
		l := int(b[i])
		i++
		if l&0x80 != 0 {
			nb := l & 0x7F
			if nb == 0 || nb > 2 || i+nb > len(b) {
				return nil, fmt.Errorf("tag %s: unsupported length encoding", tag)
			}
			l = 0
			for k := 0; k < nb; k++ {
				l = l<<8 | int(b[i])
				i++
			}
		}
		if i+l > len(b) {
			return nil, fmt.Errorf("tag %s: %w", tag, errTrunc)
		}
		out = append(out, TLV{tag, append([]byte(nil), b[i:i+l]...)})
		i += l
	}
	return out, nil
}

// Build encodes TLVs back to bytes.
func Build(tlvs []TLV) ([]byte, error) {
	var out []byte
	for _, t := range tlvs {
		tag, err := hex.DecodeString(t.Tag)
		if err != nil || len(tag) == 0 {
			return nil, fmt.Errorf("bad tag %q", t.Tag)
		}
		out = append(out, tag...)
		switch n := len(t.Value); {
		case n < 0x80:
			out = append(out, byte(n))
		case n <= 0xFF:
			out = append(out, 0x81, byte(n))
		case n <= 0xFFFF:
			out = append(out, 0x82, byte(n>>8), byte(n))
		default:
			return nil, fmt.Errorf("tag %s: value too long", t.Tag)
		}
		out = append(out, t.Value...)
	}
	return out, nil
}

// Find returns the value of the first TLV with the given tag.
func Find(tlvs []TLV, tag string) ([]byte, bool) {
	for _, t := range tlvs {
		if t.Tag == tag {
			return t.Value, true
		}
	}
	return nil, false
}

// Names is the subset of EMV tags used in this PoC.
var Names = map[string]string{
	"9F26": "Application Cryptogram (ARQC)",
	"9F27": "Cryptogram Information Data",
	"9F10": "Issuer Application Data",
	"9F37": "Unpredictable Number",
	"9F36": "Application Transaction Counter",
	"95":   "Terminal Verification Results",
	"9A":   "Transaction Date (YYMMDD)",
	"9C":   "Transaction Type",
	"9F02": "Amount, Authorised",
	"5F2A": "Transaction Currency Code",
	"82":   "Application Interchange Profile",
	"9F1A": "Terminal Country Code",
	"9F03": "Amount, Other",
	"9F33": "Terminal Capabilities",
	"9F34": "CVM Results",
	"9F35": "Terminal Type",
	"84":   "Dedicated File (AID) Name",
	"91":   "Issuer Authentication Data (ARPC+ARC)",
}
