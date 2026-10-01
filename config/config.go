// Package config reads the shared service settings from the environment.
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

// Accepted ALGORITHM values.
const (
	TokenBucket          = "token_bucket"
	FixedWindow          = "fixed_window"
	SlidingWindowLog     = "sliding_window_log"
	SlidingWindowCounter = "sliding_window_counter"
)

// Policy is the one admission policy shared by every algorithm: Limit
// admissions per Period.
type Policy struct {
	Limit  uint64
	Period time.Duration
}

// RefillPerSecond is the token bucket's continuous refill rate. The bucket
// capacity is Limit, so an empty bucket fills in exactly one Period.
func (p Policy) RefillPerSecond() float64 {
	return float64(p.Limit) / p.Period.Seconds()
}

type Config struct {
	HTTPAddr        string
	GRPCAddr        string
	RedisAddr       string
	Algorithm       string
	Policy          Policy
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
}

// Load validates all settings before either listener starts. Durations use Go
// duration syntax, such as 2s or 500ms.
func Load() (Config, error) {
	c := Config{
		HTTPAddr:  env("HTTP_ADDR", "127.0.0.1:8080"),
		GRPCAddr:  env("GRPC_ADDR", "127.0.0.1:9090"),
		RedisAddr: env("REDIS_ADDR", "127.0.0.1:6379"),
		Algorithm: env("ALGORITHM", TokenBucket),
	}
	switch c.Algorithm {
	case TokenBucket, FixedWindow, SlidingWindowLog, SlidingWindowCounter:
	default:
		return Config{}, fmt.Errorf("ALGORITHM must be one of %s, %s, %s, %s", TokenBucket, FixedWindow, SlidingWindowLog, SlidingWindowCounter)
	}
	var err error
	if c.Policy.Limit, err = positiveUint("LIMIT", "2"); err != nil {
		return Config{}, err
	}
	if c.Policy.Period, err = positiveDuration("PERIOD", "2s"); err != nil {
		return Config{}, err
	}
	// The scripts work in whole Unix microseconds.
	if c.Policy.Period%time.Microsecond != 0 {
		return Config{}, fmt.Errorf("PERIOD must be a whole number of microseconds")
	}
	if c.RequestTimeout, err = positiveDuration("REQUEST_TIMEOUT", "3s"); err != nil {
		return Config{}, err
	}
	if c.ShutdownTimeout, err = positiveDuration("SHUTDOWN_TIMEOUT", "5s"); err != nil {
		return Config{}, err
	}
	for name, addr := range map[string]string{"HTTP_ADDR": c.HTTPAddr, "GRPC_ADDR": c.GRPCAddr, "REDIS_ADDR": c.RedisAddr} {
		host, port, splitErr := net.SplitHostPort(addr)
		if splitErr != nil || host == "" || port == "" {
			return Config{}, fmt.Errorf("%s must be host:port", name)
		}
		n, parseErr := strconv.Atoi(port)
		if parseErr != nil || n < 1 || n > 65535 {
			return Config{}, fmt.Errorf("%s must have a port from 1 to 65535", name)
		}
	}
	host, _, _ := net.SplitHostPort(c.GRPCAddr)
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return Config{}, fmt.Errorf("GRPC_ADDR must use a loopback host")
	}
	return c, nil
}

func env(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}

func positiveUint(name, fallback string) (uint64, error) {
	v, err := strconv.ParseUint(env(name, fallback), 10, 64)
	if err != nil || v == 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return v, nil
}

func positiveDuration(name, fallback string) (time.Duration, error) {
	v, err := time.ParseDuration(env(name, fallback))
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", name)
	}
	return v, nil
}
