package main

import (
	"context"
	"flag"
	"net"
	"os"

	"pos-payment-poc/config"
	"pos-payment-poc/internal/application/transaction"
	"pos-payment-poc/internal/infrastructure"
	"pos-payment-poc/internal/transport/iso8583"
	"pos-payment-poc/logger"
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
