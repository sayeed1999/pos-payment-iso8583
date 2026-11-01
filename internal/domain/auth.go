package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"titp-poc/internal/domain/emv"
	"titp-poc/internal/domain/pin"
)

// Test behaviour (see the README).
const (
	DemoPIN      = "1234"
	PANDecline   = "4000000000000002" // issuer says 05 do not honour
	PANLostReply = "4000000000000341" // host approves, but the reply is "lost"
)

// Authorise stands in for "ask the issuer". pinBlock is nil when DE 52 is absent.
func Authorise(pan, expiry, amount string, pinBlock []byte) string {
	if !luhn(pan) {
		return "14" // invalid card number
	}
	if expiry != "" && expiry < time.Now().UTC().Format("0601") {
		return "54" // expired card
	}
	if pan == PANDecline {
		return "05" // do not honour
	}
	if n, _ := strconv.ParseInt(amount, 10, 64); n <= 0 {
		return "13" // invalid amount
	}
	if pinBlock != nil {
		got, err := pin.Decrypt(pinBlock, pan)
		if err != nil || got != DemoPIN {
			return "55" // incorrect PIN
		}
	}
	return "00"
}

// ParseTrack2 pulls PAN and expiry (YYMM) out of track 2 (DE 35).
func ParseTrack2(track string) (pan, expiry string) {
	if i := strings.IndexAny(track, "=D"); i > 0 {
		pan = track[:i]
		if len(track) >= i+5 {
			expiry = track[i+1 : i+5]
		}
	}
	return
}

// IssuerAuth fakes EMV tag 91: 8-byte ARPC + 2-byte ARC ("00"). The ARPC is a
// hash of the request cryptogram, NOT a real issuer cryptogram.
func IssuerAuth(de55 string) (string, error) {
	raw, err := hex.DecodeString(de55)
	if err != nil {
		return "", err
	}
	tlvs, err := emv.Parse(raw)
	if err != nil {
		return "", err
	}
	arqc, ok := emv.Find(tlvs, "9F26")
	if !ok {
		return "", errors.New("no ARQC in DE 55")
	}
	sum := sha256.Sum256(arqc)
	blob, err := emv.Build([]emv.TLV{{Tag: "91", Value: append(sum[:8:8], '0', '0')}})
	if err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(blob)), nil
}

func luhn(pan string) bool {
	if len(pan) < 12 {
		return false
	}
	sum, alt := 0, false
	for i := len(pan) - 1; i >= 0; i-- {
		d := int(pan[i] - '0')
		if d < 0 || d > 9 {
			return false
		}
		if alt {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}
