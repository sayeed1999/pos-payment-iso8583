package iso8583

import (
	"fmt"
	"io"
)

const maxFrame = 8192

// Encode prefixes payload with a 2-byte big-endian length. The length counts
// the payload only. TCP has no message boundaries, so the peer reads this
// header before the ISO 8583 bytes.
func Encode(payload []byte) ([]byte, error) {
	if len(payload) > 65535 {
		return nil, fmt.Errorf("frame length %d exceeds 65535", len(payload))
	}
	out := make([]byte, 2+len(payload))
	out[0] = byte(len(payload) >> 8)
	out[1] = byte(len(payload))
	copy(out[2:], payload)
	return out, nil
}

// Read blocks until one full frame is in. io.ReadFull is required: one
// conn.Read can return half a message, or two messages stuck together.
func Read(r io.Reader, max int) ([]byte, error) {
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err
	}
	n := int(hdr[0])<<8 | int(hdr[1])
	if n > max {
		return nil, fmt.Errorf("frame length %d exceeds %d", n, max)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("short frame: %w", err)
	}
	return body, nil
}
