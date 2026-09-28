// Package redischeck is the Redis admission adapter.
package redischeck

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/danielcaze/distributed-rate-limiter/config"
	"github.com/danielcaze/distributed-rate-limiter/limiter"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//go:embed admission.lua
var admissionScript string

const (
	allowedIdx = iota
	remainingIdx
	resetAtIdx
	retryAfterIdx
)

type Checker struct {
	client *redis.Client
	policy config.Policy

	shaMu sync.RWMutex
	sha   string
}

func New(ctx context.Context, addr string, policy config.Policy) (*Checker, error) {
	client := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
	sha, err := client.ScriptLoad(ctx, admissionScript).Result()
	if err != nil {
		return nil, err
	}
	return &Checker{client: client, policy: policy, sha: sha}, nil
}

// scriptSHA returns the cached script SHA under a read lock; concurrent
// callers may share one Checker, and the SHA is rewritten after NOSCRIPT.
func (c *Checker) scriptSHA() string {
	c.shaMu.RLock()
	defer c.shaMu.RUnlock()
	return c.sha
}

func (c *Checker) setScriptSHA(sha string) {
	c.shaMu.Lock()
	defer c.shaMu.Unlock()
	c.sha = sha
}

func (c *Checker) Close() error { return c.client.Close() }

func (c *Checker) Ping(ctx context.Context) error { return c.client.Ping(ctx).Err() }

// Check runs the admission script atomically against the bucket for key.
func (c *Checker) Check(ctx context.Context, key string) (limiter.Decision, error) {
	return c.check(ctx, key, nil)
}

// checkAt runs the same admission path at an explicit time for deterministic
// same-package tests. Production calls use Check, which leaves time selection
// to Redis TIME.
func (c *Checker) checkAt(ctx context.Context, key string, now time.Time) (limiter.Decision, error) {
	nowMicros := now.UnixMicro()
	return c.check(ctx, key, &nowMicros)
}

func (c *Checker) check(ctx context.Context, key string, testNowMicros *int64) (limiter.Decision, error) {
	if err := c.Ping(ctx); err != nil {
		return limiter.Decision{}, redisError(err)
	}

	res, err := evalWithRetry(ctx, c, key, testNowMicros)

	if err != nil {
		return limiter.Decision{}, redisError(err)
	}

	arr, ok := res.([]interface{})
	if !ok {
		return limiter.Decision{}, status.Error(codes.Internal, fmt.Sprintf("admission script returned unexpected reply type %T", res))
	}

	allowed := arr[allowedIdx].(int64) == 1
	remaining := uint64(arr[remainingIdx].(int64))
	resetAt := time.UnixMicro(arr[resetAtIdx].(int64))
	retryAfter := time.Duration(arr[retryAfterIdx].(int64)) * time.Microsecond

	reply := limiter.Decision{
		Allowed:    allowed,
		Remaining:  remaining,
		ResetAt:    resetAt,
		RetryAfter: retryAfter,
	}

	return reply, nil
}

func evalWithRetry(ctx context.Context, c *Checker, key string, testNowMicros *int64) (interface{}, error) {
	args := []interface{}{c.policy.Capacity, c.policy.RefillTokensPerSecond}
	if testNowMicros != nil {
		args = append(args, *testNowMicros)
	}

	res, err := c.client.EvalSha(ctx, c.scriptSHA(), []string{key}, args...).Result()

	if err != nil {
		if redis.HasErrorPrefix(err, "NOSCRIPT") {
			sha, err := c.client.ScriptLoad(ctx, admissionScript).Result()
			if err != nil {
				return nil, redisError(err)
			}
			c.setScriptSHA(sha)

			res, err := c.client.EvalSha(ctx, sha, []string{key}, args...).Result()
			if err != nil {
				return nil, redisError(err)
			}

			return res, nil
		}
		return nil, redisError(err)
	}

	return res, nil
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
