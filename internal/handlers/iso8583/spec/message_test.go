package spec

import (
	"bytes"
	"testing"
)

func TestPINBlockPacksAs16HexChars(t *testing.T) {
	blk := []byte{0x04, 0x12, 0x34, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	m := New("0200").SetBytes(52, blk)
	packed, err := m.Pack()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(packed, []byte("041234FFFFFFFFFF")) {
		t.Fatalf("packed message has no 16 hex chars for the PIN block: %s", packed)
	}
	got, err := Unpack(packed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(52), blk) {
		t.Fatalf("unpacked %X", got.Bytes(52))
	}
}
