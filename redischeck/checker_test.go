package redischeck_test

import (
	"context"
	"testing"

	"github.com/danielcaze/distributed-rate-limiter/config"
	"github.com/danielcaze/distributed-rate-limiter/redischeck"
	"github.com/danielcaze/distributed-rate-limiter/testsupport"
)

func TestRedis_Connection(t *testing.T) {
	checker, ctx, _ := setupChecker(t, config.Policy{})

	if err := checker.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestChecker_FirstCallAllowed(t *testing.T) {
	checker, ctx, _ := setupChecker(t, config.Policy{Capacity: 2, RefillTokensPerSecond: 1})
	key := "user-123"

	decision, err := checker.Check(ctx, key)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !decision.Allowed || decision.Remaining != 1 {
		t.Fatalf("decision = {allowed: %v, remaining: %d}, want {allowed: true, remaining: 1}", decision.Allowed, decision.Remaining)
	}
}

func TestChecker_DeniesWhenExhausted(t *testing.T) {
	capacity := 2
	checker, ctx, _ := setupChecker(t, config.Policy{Capacity: uint64(capacity), RefillTokensPerSecond: 1})
	key := "user-123"

	for i := range capacity + 1 {
		decision, err := checker.Check(ctx, key)
		if err != nil {
			t.Fatalf("check %d: %v", i, err)
		}

		if i == capacity {
			if decision.Allowed || decision.Remaining != 0 {
				t.Fatalf("decision = {allowed: %v, remaining: %d}, want {allowed: false, remaining: 0}", decision.Allowed, decision.Remaining)
			}
			if decision.RetryAfter == 0 {
				t.Fatalf("decision.RetryAfter = %v, want a positive duration", decision.RetryAfter)
			}
			continue
		}

		if !decision.Allowed {
			t.Fatalf("check %d: decision.Allowed = false, want true", i)
		}
	}
}

func setupChecker(t *testing.T, policy config.Policy) (*redischeck.Checker, context.Context, func() error) {
	t.Helper()
	ctx := t.Context()
	addr, cleanup, err := testsupport.RedisContainer(ctx)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(context.Background()); err != nil {
			t.Errorf("cleanup Redis container: %v", err)
		}
	})

	checker, err := redischeck.New(ctx, addr, policy)
	if err != nil {
		t.Fatalf("redis check: %v", err)
	}

	closed := false
	closeChecker := func() error {
		if closed {
			return nil
		}
		closed = true
		return checker.Close()
	}
	t.Cleanup(func() {
		if err := closeChecker(); err != nil {
			t.Errorf("cleanup Redis client: %v", err)
		}
	})

	return checker, ctx, closeChecker
}
