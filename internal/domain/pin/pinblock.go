// Package pin builds and opens ISO format-0 PIN blocks encrypted with 3DES,
// which is how a real terminal protects the PIN in DE 52.
//
// DemoTPK is a public TEST key. In production the terminal PIN key is injected
// into the PIN pad's secure hardware and the host decrypts inside an HSM.
package pin

import (
	"crypto/cipher"
	"crypto/des"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
)

var DemoTPK = []byte{
	0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF,
	0xFE, 0xDC, 0xBA, 0x98, 0x76, 0x54, 0x32, 0x10,
}

func block() (cipher.Block, error) {
	key := append(append([]byte{}, DemoTPK...), DemoTPK[:8]...) // 2-key 3DES: K1 K2 K1
	return des.NewTripleDESCipher(key)
}

// panField: 0000 + the 12 right-most PAN digits excluding the check digit.
func panField(pan string) ([]byte, error) {
	if len(pan) < 13 {
		return nil, errors.New("PAN too short for PIN block")
	}
	return hex.DecodeString("0000" + pan[len(pan)-13:len(pan)-1])
}

func xor8(a, b []byte) []byte {
	out := make([]byte, 8)
	for i := range out {
		out[i] = a[i] ^ b[i]
	}
	return out
}

// Encrypt returns the 8-byte encrypted ISO-0 PIN block for DE 52.
func Encrypt(pin, pan string) ([]byte, error) {
	if len(pin) < 4 || len(pin) > 12 {
		return nil, errors.New("PIN must be 4..12 digits")
	}
	if _, err := strconv.Atoi(pin); err != nil {
		return nil, errors.New("PIN must be numeric")
	}
	p := fmt.Sprintf("0%X%s", len(pin), pin)
	for len(p) < 16 {
		p += "F"
	}
	pf, err := hex.DecodeString(p)
	if err != nil {
		return nil, err
	}
	af, err := panField(pan)
	if err != nil {
		return nil, err
	}
	c, err := block()
	if err != nil {
		return nil, err
	}
	out := make([]byte, 8)
	c.Encrypt(out, xor8(pf, af))
	return out, nil
}

// Decrypt opens a PIN block and returns the clear PIN (host side).
func Decrypt(blk []byte, pan string) (string, error) {
	if len(blk) != 8 {
		return "", errors.New("PIN block must be 8 bytes")
	}
	af, err := panField(pan)
	if err != nil {
		return "", err
	}
	c, err := block()
	if err != nil {
		return "", err
	}
	clear := make([]byte, 8)
	c.Decrypt(clear, blk)
	s := fmt.Sprintf("%X", xor8(clear, af))
	if s[0] != '0' {
		return "", errors.New("not an ISO format-0 PIN block")
	}
	l, err := strconv.ParseInt(s[1:2], 16, 32)
	if err != nil || l < 4 || l > 12 {
		return "", errors.New("bad PIN length nibble")
	}
	pin := s[2 : 2+int(l)]
	if _, err := strconv.Atoi(pin); err != nil {
		return "", errors.New("PIN block does not decode to digits (wrong key or PAN)")
	}
	return pin, nil
}
