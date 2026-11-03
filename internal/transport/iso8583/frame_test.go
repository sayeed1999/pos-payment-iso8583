package iso8583

import (
	"bytes"
	"testing"
	"testing/iotest"
)

func TestFrameSplitRead(t *testing.T) {
	payload := []byte("hello iso 8583")
	wire, err := Encode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if wire[0] != 0 || int(wire[1]) != len(payload) {
		t.Fatalf("header: % X", wire[:2])
	}
	got, err := Read(iotest.OneByteReader(bytes.NewReader(wire)), 100)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q", got)
	}
}

func TestFrameTwoMessages(t *testing.T) {
	var buf bytes.Buffer
	first, err := Encode([]byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encode([]byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buf.Write(first); err != nil {
		t.Fatal(err)
	}
	if _, err := buf.Write(second); err != nil {
		t.Fatal(err)
	}
	a, err := Read(&buf, 100)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Read(&buf, 100)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != "first" || string(b) != "second" {
		t.Fatalf("got %q %q", a, b)
	}
}
