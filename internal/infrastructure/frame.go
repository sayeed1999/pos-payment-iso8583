// Package infrastructure is the TCP detail: the length header in front of every
// ISO 8583 message. TCP has no message boundaries, so this header is how the
// receiver knows where one message ends and the next begins.
package infrastructure

import (
	"fmt"
	"io"
)

// Framing describes the length header in front of every ISO 8583 message.
//
// Default: 2 bytes, big-endian, counting only the bytes AFTER the header.
// The real TITP framing may differ (little-endian, ASCII digits, a header that
// counts itself...). Adjust here once you see a capture. moov also ships a
// `network` package with ready-made headers (binary, BCD, ASCII) you can use
// instead of this file.
type Framing struct {
	LenBytes       int
	LittleEndian   bool
	IncludesHeader bool // the length value also counts the header bytes
}

var DefaultFraming = Framing{LenBytes: 2}

// Encode returns header + payload, ready to write to the socket.
func (f Framing) Encode(payload []byte) []byte {
	n := len(payload)
	if f.IncludesHeader {
		n += f.LenBytes
	}
	out := make([]byte, f.LenBytes, f.LenBytes+len(payload))
	for i := 0; i < f.LenBytes; i++ {
		shift := uint(8 * (f.LenBytes - 1 - i))
		if f.LittleEndian {
			shift = uint(8 * i)
		}
		out[i] = byte(n >> shift)
	}
	return append(out, payload...)
}

// Read blocks until one complete frame has arrived and returns its payload.
// io.ReadFull matters: a single conn.Read may return half a message or two
// messages glued together.
func (f Framing) Read(r io.Reader, max int) ([]byte, error) {
	hdr := make([]byte, f.LenBytes)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err // io.EOF here = peer closed cleanly between messages
	}
	n := 0
	for i := 0; i < f.LenBytes; i++ {
		if f.LittleEndian {
			n |= int(hdr[i]) << uint(8*i)
		} else {
			n = n<<8 | int(hdr[i])
		}
	}
	if f.IncludesHeader {
		n -= f.LenBytes
	}
	if n < 0 || n > max {
		return nil, fmt.Errorf("frame length %d outside 0..%d", n, max)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("short frame body: %w", err)
	}
	return body, nil
}

// RawFrame is the length header in hex, then the ISO payload as text.
// UNMASKED: card data is visible. PoC only.
func RawFrame(f Framing, payload []byte) string {
	framed := f.Encode(payload)
	return fmt.Sprintf("%X | %s", framed[:f.LenBytes], payload)
}
