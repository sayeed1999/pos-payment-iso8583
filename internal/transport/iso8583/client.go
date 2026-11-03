package iso8583

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"pos-payment-poc/internal/application/transaction"
	"pos-payment-poc/internal/transport/iso8583/spec"
	"pos-payment-poc/logger"
)

// Client is the terminal side of the socket.
// DialTimeout is net.Dial with a deadline so a dead host cannot block the process.
type Client struct {
	Addr    string
	Timeout time.Duration
	conn    net.Conn
}

func NewClient(addr string, timeout time.Duration) *Client {
	return &Client{Addr: addr, Timeout: timeout}
}

func (c *Client) Dial() error {
	conn, err := net.DialTimeout("tcp", c.Addr, c.Timeout)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) Exchange(ctx context.Context, req transaction.Request) (transaction.Reply, error) {
	if c.conn == nil {
		if err := c.Dial(); err != nil {
			return transaction.Reply{}, err
		}
	}
	payload, err := packRequest(req)
	if err != nil {
		return transaction.Reply{}, err
	}
	frame, err := Encode(payload)
	if err != nil {
		return transaction.Reply{}, err
	}
	if err := c.conn.SetDeadline(time.Now().Add(c.Timeout)); err != nil {
		return transaction.Reply{}, err
	}
	defer func() {
		if derr := c.conn.SetDeadline(time.Time{}); derr != nil {
			logger.Error(ctx, "clear deadline failed", "error", derr)
		}
	}()
	if _, err := c.conn.Write(frame); err != nil {
		return transaction.Reply{}, err
	}
	logger.Info(ctx, "request sent", "payload", req.Masked())

	body, err := Read(c.conn, maxFrame)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			logger.Error(ctx, "host timeout", "mti", req.MTI, "stan", req.STAN, "error", err)
			return transaction.Reply{}, transaction.ErrTimeout
		}
		return transaction.Reply{}, err
	}
	msg, err := spec.Unpack(body)
	if err != nil {
		return transaction.Reply{}, err
	}
	reply, err := unpackReply(msg)
	if err != nil {
		return transaction.Reply{}, err
	}
	want := responseMTI(req.MTI)
	if reply.MTI != want || reply.STAN != req.STAN {
		return transaction.Reply{}, fmt.Errorf("response MTI %s STAN %s does not match request MTI %s STAN %s", reply.MTI, reply.STAN, req.MTI, req.STAN)
	}
	logger.Info(ctx, "response received", "payload", reply)
	return reply, nil
}
