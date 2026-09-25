package httpgateway_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	limiterv1 "github.com/danielcaze/distributed-rate-limiter/gen/limiter/v1"
	"github.com/danielcaze/distributed-rate-limiter/limiter"
	"github.com/danielcaze/distributed-rate-limiter/transport/grpcserver"
	"github.com/danielcaze/distributed-rate-limiter/transport/httpgateway"
)

type fixedChecker struct {
	Allowed    bool
	Remaining  uint64
	ResetAt    time.Time
	RetryAfter time.Duration
	Key        chan string
	Err        error
}

func (f *fixedChecker) Check(ctx context.Context, key string) (limiter.Decision, error) {
	f.Key <- key

	if f.Err != nil {
		return limiter.Decision{}, f.Err
	}

	decision := limiter.Decision{
		Allowed:    f.Allowed,
		Remaining:  f.Remaining,
		ResetAt:    f.ResetAt,
		RetryAfter: f.RetryAfter,
	}

	return decision, nil
}

func TestCheck_Allowed(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	fc := &fixedChecker{
		Allowed:    true,
		Remaining:  0,
		ResetAt:    time.Now().Add(1),
		RetryAfter: 0,
		Key:        make(chan string, 1),
	}

	grpcServer := grpc.NewServer()
	grpcService := grpcserver.Server{Checker: fc}
	limiterv1.RegisterLimiterServer(grpcServer, grpcService)

	go grpcServer.Serve(listener)

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))

	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	client := limiterv1.NewLimiterClient(conn)
	handler := httpgateway.New(client, 10*time.Second, func(_ context.Context) error { return nil })

	httpServer := httptest.NewServer(handler)

	key := "user-123"

	reqBody := strings.NewReader(fmt.Sprintf(`{"key": %q}`, key))

	res, err := http.Post(httpServer.URL+"/v1/check", "application/json", reqBody)

	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", res.StatusCode, http.StatusOK)
	}

	if gotKey := <-fc.Key; gotKey != key {
		t.Fatalf("key received by checker = %q, want %q", gotKey, key)
	}

	response := &limiterv1.CheckResponse{}

	raw, err := io.ReadAll(res.Body)

	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	if err := protojson.Unmarshal(raw, response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if !response.Allowed {
		t.Fatalf("response.Allowed = false, want true")
	}
	if response.Remaining != 0 {
		t.Fatalf("response.Remaining = %d, want 0", response.Remaining)
	}

	t.Cleanup(func() {
		listener.Close()
		grpcServer.Stop()
		httpServer.Close()
		conn.Close()
		res.Body.Close()
	})
}

func TestCheck_Denied(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	fc := &fixedChecker{
		Allowed:    false,
		Remaining:  0,
		ResetAt:    time.Now().Add(1),
		RetryAfter: 0,
		Key:        make(chan string, 1),
	}

	grpcServer := grpc.NewServer()
	grpcService := grpcserver.Server{Checker: fc}
	limiterv1.RegisterLimiterServer(grpcServer, grpcService)

	go grpcServer.Serve(listener)

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))

	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	client := limiterv1.NewLimiterClient(conn)
	handler := httpgateway.New(client, 10*time.Second, func(_ context.Context) error { return nil })

	httpServer := httptest.NewServer(handler)

	key := "user-123"

	reqBody := strings.NewReader(fmt.Sprintf(`{"key": %q}`, key))

	res, err := http.Post(httpServer.URL+"/v1/check", "application/json", reqBody)

	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status code = %d, want %d", res.StatusCode, http.StatusTooManyRequests)
	}

	if gotKey := <-fc.Key; gotKey != key {
		t.Fatalf("key received by checker = %q, want %q", gotKey, key)
	}

	response := &limiterv1.CheckResponse{}

	raw, err := io.ReadAll(res.Body)

	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	if err := protojson.Unmarshal(raw, response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if response.Allowed {
		t.Fatalf("response.Allowed = %v, want %v", response.Allowed, !response.Allowed)
	}
	if response.Remaining != 0 {
		t.Fatalf("response.Remaining = %d, want 0", response.Remaining)
	}

	t.Cleanup(func() {
		listener.Close()
		grpcServer.Stop()
		httpServer.Close()
		conn.Close()
		res.Body.Close()
	})
}

func TestCheck_NoKey(t *testing.T) {
	var client limiterv1.LimiterClient
	handler := httpgateway.New(client, 10*time.Second, func(_ context.Context) error { return nil })

	httpServer := httptest.NewServer(handler)

	key := ""

	reqBody := strings.NewReader(fmt.Sprintf(`{"key": %q}`, key))

	res, err := http.Post(httpServer.URL+"/v1/check", "application/json", reqBody)

	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", res.StatusCode, http.StatusBadRequest)
	}

	t.Cleanup(func() {
		httpServer.Close()
		res.Body.Close()
	})
}

func TestCheck_DeadlineExceeded(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	fc := &fixedChecker{
		Allowed:    false,
		Remaining:  0,
		ResetAt:    time.Now().Add(1),
		RetryAfter: 0,
		Key:        make(chan string, 1),
		Err:        context.DeadlineExceeded,
	}

	grpcServer := grpc.NewServer()
	grpcService := grpcserver.Server{Checker: fc}
	limiterv1.RegisterLimiterServer(grpcServer, grpcService)

	go grpcServer.Serve(listener)

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))

	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	client := limiterv1.NewLimiterClient(conn)
	handler := httpgateway.New(client, 10*time.Second, func(_ context.Context) error { return nil })

	httpServer := httptest.NewServer(handler)

	key := "user-123"

	reqBody := strings.NewReader(fmt.Sprintf(`{"key": %q}`, key))

	res, err := http.Post(httpServer.URL+"/v1/check", "application/json", reqBody)

	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if res.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("status code = %d, want %d", res.StatusCode, http.StatusGatewayTimeout)
	}

	if gotKey := <-fc.Key; gotKey != key {
		t.Fatalf("key received by checker = %q, want %q", gotKey, key)
	}

	t.Cleanup(func() {
		listener.Close()
		grpcServer.Stop()
		httpServer.Close()
		conn.Close()
		res.Body.Close()
	})
}

func TestCheck_Unimplemented(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	grpcService := grpcserver.Server{Checker: nil}
	limiterv1.RegisterLimiterServer(grpcServer, grpcService)

	go grpcServer.Serve(listener)

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))

	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	client := limiterv1.NewLimiterClient(conn)
	handler := httpgateway.New(client, 10*time.Second, func(_ context.Context) error { return nil })

	httpServer := httptest.NewServer(handler)

	key := "user-123"

	reqBody := strings.NewReader(fmt.Sprintf(`{"key": %q}`, key))

	res, err := http.Post(httpServer.URL+"/v1/check", "application/json", reqBody)

	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if res.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status code = %d, want %d", res.StatusCode, http.StatusNotImplemented)
	}

	t.Cleanup(func() {
		listener.Close()
		grpcServer.Stop()
		httpServer.Close()
		conn.Close()
		res.Body.Close()
	})
}

func TestCheck_ClockInjection(t *testing.T) {
	var client limiterv1.LimiterClient
	handler := httpgateway.New(client, 10*time.Second, func(_ context.Context) error { return nil })

	httpServer := httptest.NewServer(handler)

	key := "user-123"

	reqBody := strings.NewReader(fmt.Sprintf(`{"key": %q, "clock": "2026-09-25"}`, key))

	res, err := http.Post(httpServer.URL+"/v1/check", "application/json", reqBody)

	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", res.StatusCode, http.StatusBadRequest)
	}

	t.Cleanup(func() {
		httpServer.Close()
		res.Body.Close()
	})
}

func TestHealth_Success(t *testing.T) {
	var client limiterv1.LimiterClient
	handler := httpgateway.New(client, 10*time.Second, func(_ context.Context) error { return nil })

	httpServer := httptest.NewServer(handler)

	res, err := http.Get(httpServer.URL + "/healthz")

	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", res.StatusCode, http.StatusOK)
	}

	t.Cleanup(func() {
		httpServer.Close()
		res.Body.Close()
	})
}

func TestReady_Success(t *testing.T) {
	var client limiterv1.LimiterClient
	handler := httpgateway.New(client, 10*time.Second, func(_ context.Context) error { return nil })

	httpServer := httptest.NewServer(handler)

	res, err := http.Get(httpServer.URL + "/readyz")

	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", res.StatusCode, http.StatusOK)
	}

	t.Cleanup(func() {
		httpServer.Close()
		res.Body.Close()
	})
}

func TestReady_Error(t *testing.T) {
	var client limiterv1.LimiterClient
	handler := httpgateway.New(client, 10*time.Second, func(_ context.Context) error { return context.Canceled })

	httpServer := httptest.NewServer(handler)

	res, err := http.Get(httpServer.URL + "/readyz")

	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d, want %d", res.StatusCode, http.StatusServiceUnavailable)
	}

	t.Cleanup(func() {
		httpServer.Close()
		res.Body.Close()
	})
}
