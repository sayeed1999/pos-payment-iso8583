// Package iso8583 is the TCP handler. It reads a framed message, checks the
// field profile, calls the purchase or reversal use case, and writes the answer.
package iso8583

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"titp-poc/internal/application"
	"titp-poc/internal/domain"
	"titp-poc/internal/handlers/iso8583/spec"
	"titp-poc/internal/infrastructure"
)

type Server struct {
	App     *application.Host
	Framing infrastructure.Framing
	ShowRaw bool // also print the raw frame (UNMASKED card data, PoC only)
}

func New(app *application.Host) *Server {
	return &Server{App: app, Framing: infrastructure.DefaultFraming}
}

// Serve accepts connections until the listener is closed.
func (s *Server) Serve(l net.Listener) error {
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go s.handleConn(c)
	}
}

func (s *Server) logf(format string, args ...any) {
	s.App.Logf(format, args...)
}

func (s *Server) handleConn(c net.Conn) {
	defer c.Close()
	peer := c.RemoteAddr().String()
	s.logf("[%s] terminal connected", peer)
	defer s.logf("[%s] terminal disconnected", peer)

	for {
		c.SetReadDeadline(time.Now().Add(2 * time.Minute))
		body, err := s.Framing.Read(c, 4096)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.logf("[%s] read error: %v", peer, err)
			}
			return
		}
		req, err := spec.Unpack(body)
		if err != nil {
			s.logf("[%s] cannot parse frame: %v\n    raw: %s", peer, err, body)
			return
		}
		s.trace(peer, "TERMINAL -> HOST", req)

		resp, drop := s.dispatch(peer, req)
		if resp == nil {
			s.logf("[%s] no handler for MTI %s, closing connection", peer, req.MTI())
			return
		}
		if drop {
			s.logf("[%s] SIMULATION: host processed STAN %s but the response is lost (not sent)", peer, req.Get(11))
			continue
		}
		out, err := resp.Pack()
		if err != nil {
			s.logf("[%s] internal error packing response: %v", peer, err)
			return
		}
		c.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := c.Write(s.Framing.Encode(out)); err != nil {
			s.logf("[%s] write error: %v", peer, err)
			return
		}
		s.trace(peer, "HOST -> TERMINAL", resp)
	}
}

func (s *Server) trace(peer, dir string, m *spec.Msg) {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s  %s %s\n", peer, dir, m.MTI(), spec.MTIName(m.MTI()))
	b.WriteString(spec.Dump(m))
	if s.ShowRaw {
		if payload, err := m.Pack(); err == nil {
			b.WriteString("    raw frame (unmasked): " + infrastructure.RawFrame(s.Framing, payload) + "\n")
		}
	}
	s.logf("%s", strings.TrimRight(b.String(), "\n"))
}

func (s *Server) dispatch(peer string, req *spec.Msg) (*spec.Msg, bool) {
	chk := spec.Check(req)
	if chk.Rule == nil {
		return nil, false
	}
	if len(chk.Missing) > 0 {
		s.logf("[%s] FORMAT ERROR: %s -> answering 30", peer, chk)
		return formatError(req), false
	}
	if len(chk.Unexpected) > 0 {
		s.logf("[%s] WARNING: %s (processing anyway)", peer, chk)
	}
	switch req.MTI() {
	case "0200":
		return s.purchase(peer, req)
	case "0400":
		reply := s.App.Reversal(peer, application.Reversal{
			TID:            req.Get(41),
			RRN:            req.Get(37),
			Original:       req.Get(90),
			ProcessingCode: req.Get(3),
			Amount:         req.Get(4),
			Transmission:   req.Get(7),
			STAN:           req.Get(11),
			LocalTime:      req.Get(12),
			LocalDate:      req.Get(13),
			MID:            req.Get(42),
			Currency:       req.Get(49),
		})
		return replyMsg(reply), false
	}
	return nil, false
}

func (s *Server) purchase(peer string, req *spec.Msg) (*spec.Msg, bool) {
	pan, expiry := domain.ParseTrack2(req.Get(35))
	var pinBlock []byte
	if req.Has(52) {
		pinBlock = req.Bytes(52)
	}
	reply := s.App.Purchase(peer, application.Purchase{
		TID:            req.Get(41),
		STAN:           req.Get(11),
		Transmission:   req.Get(7),
		Amount:         req.Get(4),
		PAN:            pan,
		Expiry:         expiry,
		PINBlock:       pinBlock,
		ICC:            req.Get(55),
		HasICC:         req.Has(55),
		ProcessingCode: req.Get(3),
		LocalTime:      req.Get(12),
		LocalDate:      req.Get(13),
		MID:            req.Get(42),
		Currency:       req.Get(49),
	})
	return replyMsg(reply), reply.Drop
}

func replyMsg(r application.Reply) *spec.Msg {
	m := spec.New(r.MTI).
		Set(3, r.ProcessingCode).Set(4, r.Amount).Set(7, r.Transmission).
		Set(11, r.STAN).Set(12, r.LocalTime).Set(13, r.LocalDate).
		Set(37, r.RRN).Set(39, r.RC).
		Set(41, r.TID).Set(42, r.MID).Set(49, r.Currency)
	if r.AuthID != "" {
		m.Set(38, r.AuthID)
	}
	if r.ICC != "" {
		m.Set(55, r.ICC)
	}
	return m
}

func formatError(req *spec.Msg) *spec.Msg {
	resp := spec.New(spec.ResponseMTI(req.MTI()))
	for _, n := range []int{3, 4, 7, 11, 12, 13, 41, 42, 49} {
		if req.Has(n) {
			resp.Set(n, req.Get(n))
		}
	}
	return resp.Set(39, "30")
}
