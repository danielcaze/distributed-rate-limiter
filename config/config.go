// Package config reads the shared M1 service settings from the environment.
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

type Policy struct {
	Capacity              uint64
	RefillTokensPerSecond uint64
}

type Config struct {
	HTTPAddr        string
	GRPCAddr        string
	RedisAddr       string
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
	}
	var err error
	if c.Policy.Capacity, err = positiveUint("CAPACITY", "2"); err != nil {
		return Config{}, err
	}
	if c.Policy.RefillTokensPerSecond, err = positiveUint("REFILL_TOKENS_PER_SECOND", "1"); err != nil {
		return Config{}, err
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
