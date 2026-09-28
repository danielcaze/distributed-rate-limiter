package redischeck

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielcaze/distributed-rate-limiter/config"
	"github.com/danielcaze/distributed-rate-limiter/testsupport"
)

func TestChecker_ContentionAcrossInstancesAdmitsOnlyCapacity(t *testing.T) {
	instances := 2
	capacity := 100
	calls := 1000
	checkers, ctx := setupMultipleCheckers(t, config.Policy{Capacity: uint64(capacity), RefillTokensPerSecond: 1}, instances)
	var failed, allowed, denied atomic.Int64

	now := time.Unix(1_700_000_000, 0).UTC()
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range calls {
		c := checkers[i%len(checkers)]
		wg.Go(func() {
			<-start
			decision, err := c.checkAt(ctx, "user-123", now)

			if err != nil {
				failed.Add(1)
				return
			}

			if decision.Allowed {
				allowed.Add(1)
			} else {
				denied.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()

	gotFailed, gotAllowed, gotDenied := failed.Load(), allowed.Load(), denied.Load()

	if gotAllowed != int64(capacity) || gotDenied != int64(calls-capacity) || gotFailed != 0 {
		t.Fatalf("allowed=%d denied=%d errors=%d, want %d/%d/%d", gotAllowed, gotDenied, gotFailed, capacity, calls-capacity, 0)
	}
	t.Logf("allowed=%d denied=%d errors=%d, want %d/%d/%d", gotAllowed, gotDenied, gotFailed, capacity, calls-capacity, 0)
}

func setupMultipleCheckers(t *testing.T, policy config.Policy, numberOfCheckers int) ([]*Checker, context.Context) {
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

	checkers := make([]*Checker, 0, numberOfCheckers)
	for i := range numberOfCheckers {
		checker, err := New(ctx, addr, policy)

		if err != nil {
			t.Fatalf("new checker %d: %v", i, err)
		}

		checkers = append(checkers, checker)
		t.Cleanup(func() {
			if err := checker.Close(); err != nil {
				t.Errorf("cleanup Redis client: %v", err)
			}
		})
	}

	return checkers, ctx
}
