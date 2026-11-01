package infrastructure

import (
	"bytes"
	"testing"
	"testing/iotest"
)

func TestFramingSplitReadsAndEndianness(t *testing.T) {
	payload := []byte("hello iso 8583")
	for _, f := range []Framing{DefaultFraming, {LenBytes: 2, LittleEndian: true}, {LenBytes: 4}, {LenBytes: 2, IncludesHeader: true}} {
		wire := f.Encode(payload)
		// one byte at a time: proves we use ReadFull and do not trust a single Read
		got, err := f.Read(iotest.OneByteReader(bytes.NewReader(wire)), 100)
		if err != nil || !bytes.Equal(got, payload) {
			t.Errorf("%+v: got %q err %v", f, got, err)
		}
	}
	if w := DefaultFraming.Encode(payload); w[0] != 0 || int(w[1]) != len(payload) {
		t.Errorf("default header should be big-endian: % X", w[:2])
	}
}

func TestFramingTwoMessagesInOneWrite(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(DefaultFraming.Encode([]byte("first")))
	buf.Write(DefaultFraming.Encode([]byte("second")))
	a, _ := DefaultFraming.Read(&buf, 100)
	b, _ := DefaultFraming.Read(&buf, 100)
	if string(a) != "first" || string(b) != "second" {
		t.Fatalf("got %q %q", a, b)
	}
}
