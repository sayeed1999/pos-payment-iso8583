package config

import "time"

// HostConfig is the host process: the address it listens on.
type HostConfig struct {
	ServiceName string
	ListenAddr  string
}

func DefaultHost() HostConfig {
	return HostConfig{ServiceName: "host", ListenAddr: ":8583"}
}

// TerminalConfig is the POS process: where it dials, how long it waits,
// and the terminal, merchant, and currency it puts on each request.
type TerminalConfig struct {
	ServiceName string
	HostAddr    string
	Timeout     time.Duration
	TerminalID  string
	MerchantID  string
	Currency    string
}

func DefaultTerminal() TerminalConfig {
	return TerminalConfig{
		ServiceName: "terminal",
		HostAddr:    "127.0.0.1:8583",
		Timeout:     2 * time.Second,
		TerminalID:  "POS00001",
		MerchantID:  "MERCHANT0000001",
		Currency:    "050",
	}
}
