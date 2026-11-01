package emv

import (
	"bytes"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	in := []TLV{
		{"9F26", []byte{1, 2, 3, 4, 5, 6, 7, 8}},
		{"95", []byte{0, 0, 0, 0, 0}},
		{"5F2A", []byte{0x00, 0x50}},
		{"9F10", bytes.Repeat([]byte{0xCC}, 200)}, // forces the 0x81 long length form
	}
	b, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Parse(b)
	if err != nil || len(out) != len(in) {
		t.Fatalf("err %v, %d tlvs", err, len(out))
	}
	for i := range in {
		if out[i].Tag != in[i].Tag || !bytes.Equal(out[i].Value, in[i].Value) {
			t.Errorf("tlv %d differs", i)
		}
	}
}

func TestKnownEncoding(t *testing.T) {
	b, _ := Build([]TLV{{"9F27", []byte{0x80}}})
	if !bytes.Equal(b, []byte{0x9F, 0x27, 0x01, 0x80}) {
		t.Fatalf("% X", b)
	}
}

func TestTruncated(t *testing.T) {
	if _, err := Parse([]byte{0x9F, 0x26, 0x08, 1, 2}); err == nil {
		t.Fatal("truncated value accepted")
	}
}
