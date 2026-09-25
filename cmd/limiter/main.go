package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/danielcaze/distributed-rate-limiter/config"
	limiterv1 "github.com/danielcaze/distributed-rate-limiter/gen/limiter/v1"
	"github.com/danielcaze/distributed-rate-limiter/redischeck"
	"github.com/danielcaze/distributed-rate-limiter/transport/grpcserver"
	"github.com/danielcaze/distributed-rate-limiter/transport/httpgateway"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	checker := redischeck.New(cfg.RedisAddr, cfg.Policy)
	defer checker.Close()
	startupCtx, cancel := context.WithTimeout(context.Background(), cfg.RequestTimeout)
	err = checker.Ping(startupCtx)
	cancel()
	if err != nil {
		return err
	}
	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return err
	}
	defer grpcListener.Close()
	httpListener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return err
	}
	defer httpListener.Close()

	grpcServer := grpc.NewServer()
	limiterv1.RegisterLimiterServer(grpcServer, grpcserver.Server{Checker: checker})
	// The only client target is the listener in this process. Configured gRPC
	// retries are disabled for this admission call.
	conn, err := grpc.NewClient(grpcListener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig())
	if err != nil {
		return err
	}
	defer conn.Close()
	httpServer := &http.Server{
		Handler:           httpgateway.New(limiterv1.NewLimiterClient(conn), cfg.RequestTimeout, checker.Ping),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errorsCh := make(chan error, 2)
	go func() {
		if err := grpcServer.Serve(grpcListener); err != nil {
			errorsCh <- err
		}
	}()
	go func() {
		if err := httpServer.Serve(httpListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- err
		}
	}()
	log.Printf("HTTP listening on %s; gRPC listening on %s", httpListener.Addr(), grpcListener.Addr())
	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var serveErr error
	select {
	case <-signalCtx.Done():
	case serveErr = <-errorsCh:
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	httpErr := httpServer.Shutdown(shutdownCtx)
	grpcDone := make(chan struct{})
	go func() { grpcServer.GracefulStop(); close(grpcDone) }()
	select {
	case <-grpcDone:
	case <-shutdownCtx.Done():
		grpcServer.Stop()
		<-grpcDone
	}
	if serveErr != nil {
		return serveErr
	}
	return httpErr
}
