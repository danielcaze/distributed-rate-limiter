// Package redischeck is the Redis admission adapter. The admission script is pending.
package redischeck

import (
	"context"
	"errors"
	"net"

	"github.com/danielcaze/distributed-rate-limiter/config"
	"github.com/danielcaze/distributed-rate-limiter/limiter"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Checker struct {
	client *redis.Client
	policy config.Policy
}

func New(addr string, policy config.Policy) *Checker {
	client := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
	return &Checker{client: client, policy: policy}
}

func (c *Checker) Close() error { return c.client.Close() }

func (c *Checker) Ping(ctx context.Context) error { return c.client.Ping(ctx).Err() }

// Check keeps Redis failure mapping reachable while the Lua script is absent.
// A successful PING says nothing about admission; no bucket command is issued.
func (c *Checker) Check(ctx context.Context, _ string) (limiter.Decision, error) {
	if err := c.Ping(ctx); err != nil {
		return limiter.Decision{}, redisError(err)
	}
	return limiter.Decision{}, status.Error(codes.Unimplemented, "admission script is not implemented")
}

func redisError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "Redis deadline exceeded")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return status.Error(codes.DeadlineExceeded, "Redis deadline exceeded")
	}
	return status.Error(codes.Unavailable, "Redis unavailable")
}
