// Package spec is the ONLY package that imports moov-io/iso8583.
//
// It holds the byte dialect and WHICH data elements each message carries.
//
// Dialect: moov's stock "ISO 8583 v1987 ASCII" spec (specs.Spec87ASCII).
//   - MTI: 4 ASCII digits
//   - bitmap: hex-encoded, primary (+ secondary when any DE > 64 is present)
//   - numeric / alphanumeric fields: ASCII; LL / LLL = 2 / 3 ASCII-digit length prefix
//   - DE 52 (PIN block): 8 raw bytes, sent as 16 hex characters
//   - DE 55 (EMV data): the TLV bytes as a hex string, LLL length prefix
//
// That is a common real-world dialect, not necessarily the one TITP uses. When
// you have a capture from the device, copy specs.Spec87ASCII into this package,
// edit the fields that differ, and set `Spec` below to it. Nothing else changes.
package spec

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/moov-io/iso8583"
	"github.com/moov-io/iso8583/encoding"
	"github.com/moov-io/iso8583/field"
	"github.com/moov-io/iso8583/prefix"
	"github.com/moov-io/iso8583/specs"
)

// Spec is Spec87ASCII with field 52 replaced. The stock field is a string
// whose length check wants 16 characters, so an 8-byte PIN block will not
// pack. This field keeps the 8 raw bytes and writes them as 16 hex characters.
var Spec = pinSpec()

func pinSpec() *iso8583.MessageSpec {
	fields := make(map[int]field.Field, len(specs.Spec87ASCII.Fields))
	for id, f := range specs.Spec87ASCII.Fields {
		fields[id] = f
	}
	fields[52] = field.NewBinary(&field.Spec{
		Length:      8,
		Description: "PIN Data",
		Enc:         encoding.BytesToASCIIHex,
		Pref:        prefix.ASCII.Fixed,
	})
	return &iso8583.MessageSpec{Name: specs.Spec87ASCII.Name, Fields: fields}
}

// Msg is a thin wrapper over *iso8583.Message. Set / SetBytes can be chained
// and remember the first error, which Pack then reports.
type Msg struct {
	m   *iso8583.Message
	err error
}

func New(mti string) *Msg {
	m := iso8583.NewMessage(Spec)
	m.MTI(mti)
	return &Msg{m: m}
}

// Unpack parses MTI + bitmap + fields (no TCP length header).
// moov rejects a bitmap that announces a field the spec does not define.
func Unpack(b []byte) (*Msg, error) {
	m := iso8583.NewMessage(Spec)
	if err := m.Unpack(b); err != nil {
		return nil, err
	}
	return &Msg{m: m}, nil
}

func (x *Msg) Pack() ([]byte, error) {
	if x.err != nil {
		return nil, x.err
	}
	return x.m.Pack()
}

func (x *Msg) MTI() string {
	s, _ := x.m.GetMTI()
	return s
}

// Set stores a character field.
func (x *Msg) Set(n int, v string) *Msg {
	if err := x.m.Field(n, v); err != nil && x.err == nil {
		x.err = fmt.Errorf("DE %d: %w", n, err)
	}
	return x
}

// SetBytes stores a binary field (DE 52).
func (x *Msg) SetBytes(n int, v []byte) *Msg {
	if err := x.m.BinaryField(n, v); err != nil && x.err == nil {
		x.err = fmt.Errorf("DE %d: %w", n, err)
	}
	return x
}

func (x *Msg) Has(n int) bool {
	if n < 2 { // 0 = MTI, 1 = bitmap are not data elements
		return false
	}
	_, ok := x.m.GetFields()[n]
	return ok
}

func (x *Msg) Get(n int) string {
	if !x.Has(n) {
		return ""
	}
	s, _ := x.m.GetString(n)
	return s
}

func (x *Msg) Bytes(n int) []byte {
	if !x.Has(n) {
		return nil
	}
	b, _ := x.m.GetBytes(n)
	return b
}

// Numbers returns the data element numbers present, ascending.
func (x *Msg) Numbers() []int {
	var out []int
	for n := range x.m.GetFields() {
		if n >= 2 {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

// ResponseMTI maps a request MTI to its response MTI (0200 -> 0210).
func ResponseMTI(mti string) string {
	n, err := strconv.Atoi(mti)
	if err != nil {
		return mti
	}
	return fmt.Sprintf("%04d", n+10)
}
