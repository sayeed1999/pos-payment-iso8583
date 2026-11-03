package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"os"

	"titp-poc/config"
	"titp-poc/internal/application/transaction"
	"titp-poc/internal/infrastructure"
	"titp-poc/internal/transport/iso8583"
)

func main() {
	cfg := config.DefaultHost()
	flag.StringVar(&cfg.ListenAddr, "addr", cfg.ListenAddr, "TCP listen address")
	flag.Parse()

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		slog.Error("listen failed", "error", err)
		os.Exit(1)
	}
	slog.Info("host listening", "addr", ln.Addr().String())

	svc := transaction.NewService(infrastructure.NewDirectory())
	if err := iso8583.NewServer(svc).Serve(context.Background(), ln); err != nil {
		slog.Error("host stopped", "error", err)
		os.Exit(1)
	}
}
