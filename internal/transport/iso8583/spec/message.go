// Package spec is the only package that imports moov-io/iso8583.
// The dialect is moov's ISO 8583:1987 ASCII spec (specs.Spec87ASCII):
// 4-digit ASCII MTI, hex bitmap, ASCII fields, LL/LLL length prefixes.
package spec

import (
	"fmt"

	"github.com/moov-io/iso8583"
	"github.com/moov-io/iso8583/specs"
)

// Msg is one ISO 8583 message without the TCP length header.
type Msg struct {
	m *iso8583.Message
}

func New(mti string) (*Msg, error) {
	if len(mti) != 4 {
		return nil, fmt.Errorf("MTI %q must be 4 digits", mti)
	}
	for _, c := range mti {
		if c < '0' || c > '9' {
			return nil, fmt.Errorf("MTI %q must be 4 digits", mti)
		}
	}
	m := iso8583.NewMessage(specs.Spec87ASCII)
	m.MTI(mti)
	return &Msg{m: m}, nil
}

func Unpack(b []byte) (*Msg, error) {
	m := iso8583.NewMessage(specs.Spec87ASCII)
	if err := m.Unpack(b); err != nil {
		return nil, err
	}
	return &Msg{m: m}, nil
}

func (x *Msg) Pack() ([]byte, error) {
	return x.m.Pack()
}

func (x *Msg) MTI() (string, error) {
	return x.m.GetMTI()
}

func (x *Msg) Set(n int, v string) error {
	if err := x.m.Field(n, v); err != nil {
		return fmt.Errorf("DE %d: %w", n, err)
	}
	return nil
}

func (x *Msg) Has(n int) bool {
	if n < 2 {
		return false
	}
	_, ok := x.m.GetFields()[n]
	return ok
}

func (x *Msg) Get(n int) (string, error) {
	if !x.Has(n) {
		return "", fmt.Errorf("DE %d absent", n)
	}
	v, err := x.m.GetString(n)
	if err != nil {
		return "", fmt.Errorf("DE %d: %w", n, err)
	}
	return v, nil
}
