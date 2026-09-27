package redischeck_test

import (
	"context"
	"testing"

	"github.com/danielcaze/distributed-rate-limiter/config"
	"github.com/danielcaze/distributed-rate-limiter/redischeck"
	"github.com/danielcaze/distributed-rate-limiter/testsupport"
)

func TestRedis_Connection(t *testing.T) {
	checker, ctx := setupChecker(t, config.Policy{})

	if err := checker.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestChecker_FirstCallAllowed(t *testing.T) {
	checker, ctx := setupChecker(t, config.Policy{Capacity: 2, RefillTokensPerSecond: 1})
	key := "user-123"

	decision, err := checker.Check(ctx, key)

	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if !decision.Allowed {
		t.Fatalf("decision.Allowed = %v, want true", decision.Allowed)
	}
}

func TestChecker_DeniesWhenExhausted(t *testing.T) {
	capacity := 2
	checker, ctx := setupChecker(t, config.Policy{Capacity: uint64(capacity), RefillTokensPerSecond: 1})
	key := "user-123"

	for i := range capacity + 1 {
		decision, err := checker.Check(ctx, key)

		if err != nil {
			t.Fatalf("check: %v", err)
		}

		if i == capacity {
			if decision.RetryAfter == 0 {
				t.Fatalf("decision.RetryAfter = %v, want a positive duration", decision.RetryAfter)
			}

			if decision.Allowed {
				t.Fatalf("decision.Allowed = %v, want false", decision.Allowed)
			}

		} else {
			if !decision.Allowed {
				t.Fatalf("decision.Allowed = %v, want true", decision.Allowed)
			}
		}
	}
}

func setupChecker(t *testing.T, policy config.Policy) (*redischeck.Checker, context.Context) {
	ctx := t.Context()
	addr, cleanup, err := testsupport.RedisContainer(ctx)

	if err != nil {
		t.Fatalf("container: %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(context.Background()); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})

	checker, err := redischeck.New(ctx, addr, policy)
	if err != nil {
		t.Fatalf("redis check: %v", err)
	}

	return checker, ctx
}
