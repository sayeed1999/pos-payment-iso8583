package iso8583

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime/debug"
	"time"

	"titp-poc/internal/application/transaction"
	"titp-poc/internal/domain"
	"titp-poc/internal/transport/iso8583/spec"
	"titp-poc/logger"
)

// Server reads one ISO 8583 frame per request and answers on the same connection.
type Server struct {
	App *transaction.Service
}

func NewServer(app *transaction.Service) *Server {
	return &Server{App: app}
}

// Serve accepts until the listener is closed. Each connection has its own
// goroutine. A panic in one connection is recovered so the listener and the
// other connections keep running. That connection then goes back to reading.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				logger.Info(ctx, "listener closed")
				return err
			}
			logger.Error(ctx, "accept failed", "error", err)
			continue
		}
		go s.serveConn(ctx, conn)
	}
}

func (s *Server) serveConn(ctx context.Context, conn net.Conn) {
	peer := conn.RemoteAddr().String()
	defer func() {
		if rec := recover(); rec != nil {
			logger.Error(ctx, "connection goroutine recovered", "peer", peer, "panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
		}
	}()
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Error(ctx, "close connection failed", "peer", peer, "error", err)
		}
	}()

	logger.Info(ctx, "terminal connected", "peer", peer)
	defer logger.Info(ctx, "terminal disconnected", "peer", peer)

	for {
		if err := s.one(ctx, conn, peer); err != nil {
			if !errors.Is(err, io.EOF) {
				logger.Error(ctx, "connection ended", "peer", peer, "error", err)
			}
			return
		}
	}
}

func (s *Server) one(ctx context.Context, conn net.Conn, peer string) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			logger.Error(ctx, "recovered panic, continuing connection", "peer", peer, "panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
			err = nil
		}
	}()

	if err = conn.SetReadDeadline(time.Now().Add(2 * time.Minute)); err != nil {
		return err
	}
	body, err := Read(conn, maxFrame)
	if err != nil {
		return err
	}
	ctx = logger.WithCorrelationID(ctx, logger.NewCorrelationID())
	logger.Info(ctx, "frame received", "peer", peer, "bytes", len(body))

	msg, err := spec.Unpack(body)
	if err != nil {
		logger.Error(ctx, "cannot parse iso8583", "error", err, "bytes", len(body))
		return err
	}
	req, err := parseRequest(msg)
	if err != nil {
		logger.Error(ctx, "rejected request", "mti", req.MTI, "error", err)
		if responseMTI(req.MTI) == "" {
			return err
		}
		return s.reject(ctx, conn, req)
	}
	logger.Info(ctx, "request accepted", "mti", req.MTI, "processing_code", req.ProcessingCode, "stan", req.STAN, "amount", req.Amount, "terminal_id", req.TerminalID, "merchant_id", req.MerchantID)

	var reply transaction.Reply
	switch req.MTI {
	case "0200":
		reply = s.App.Authorize(ctx, req)
	case "0400":
		reply = s.App.Reverse(ctx, req)
	default:
		return fmt.Errorf("unsupported MTI %s", req.MTI)
	}
	// TODO(poc): do not skip the write. Silent is how amounts 500 and 400 pretend
	// the host timed out. The decision above is still stored; only the bytes are dropped.
	if reply.Silent {
		logger.Info(ctx, "response withheld to simulate host timeout", "mti", reply.MTI, "stan", reply.STAN, "rc", reply.RC)
		return nil
	}
	return s.write(ctx, conn, reply)
}

func (s *Server) reject(ctx context.Context, conn net.Conn, req transaction.Request) error {
	reply := transaction.Reply{
		MTI:            responseMTI(req.MTI),
		ProcessingCode: fallback(req.ProcessingCode, "000000"),
		Amount:         req.Amount,
		Transmission:   fallback(req.Transmission, "0000000000"),
		STAN:           fallback(req.STAN, "000000"),
		LocalTime:      fallback(req.LocalTime, "000000"),
		LocalDate:      fallback(req.LocalDate, "0000"),
		RRN:            "000000000000",
		RC:             domain.RCFormat,
		TerminalID:     fallback(req.TerminalID, "00000000"),
		MerchantID:     fallback(req.MerchantID, "000000000000000"),
		Currency:       fallback(req.Currency, "000"),
	}
	logger.Error(ctx, "sending format error", "mti", reply.MTI, "stan", reply.STAN, "rc", reply.RC)
	return s.write(ctx, conn, reply)
}

func (s *Server) write(ctx context.Context, conn net.Conn, reply transaction.Reply) error {
	payload, err := packReply(reply)
	if err != nil {
		return err
	}
	frame, err := Encode(payload)
	if err != nil {
		return err
	}
	if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	if _, err := conn.Write(frame); err != nil {
		return err
	}
	logger.Info(ctx, "response sent", "mti", reply.MTI, "stan", reply.STAN, "rc", reply.RC, "rrn", reply.RRN)
	return nil
}
