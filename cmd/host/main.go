package main

import (
	"context"
	"flag"
	"net"
	"os"

	"titp-poc/config"
	"titp-poc/internal/application/transaction"
	"titp-poc/internal/infrastructure"
	"titp-poc/internal/transport/iso8583"
	"titp-poc/logger"
)

func main() {
	cfg := config.DefaultHost()
	flag.StringVar(&cfg.ListenAddr, "addr", cfg.ListenAddr, "TCP listen address")
	flag.Parse()

	ctx := logger.WithServiceName(context.Background(), cfg.ServiceName)
	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		logger.Error(ctx, "listen failed", "error", err)
		os.Exit(1)
	}
	logger.Info(ctx, "host listening", "addr", ln.Addr().String())

	svc := transaction.NewService(infrastructure.NewDirectory())
	if err := iso8583.NewServer(svc).Serve(ctx, ln); err != nil {
		logger.Error(ctx, "host stopped", "error", err)
		os.Exit(1)
	}
}
