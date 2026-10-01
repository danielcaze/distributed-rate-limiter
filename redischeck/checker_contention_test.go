package redischeck

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielcaze/distributed-rate-limiter/config"
	"github.com/danielcaze/distributed-rate-limiter/testsupport"
)

func TestChecker_ContentionAcrossInstancesAdmitsOnlyCapacity(t *testing.T) {
	// Calls exceed capacity so a non-atomic check would show up as extra admissions.
	// Two checkers with separate clients stand in for two service instances sharing one Redis.
	const instances = 2
	const capacity = 100
	const calls = 1000

	if instances < 2 || calls <= capacity {
		t.Fatalf("invalid setup: need instances >= 2 and calls > capacity, got instances=%d calls=%d capacity=%d", instances, calls, capacity)
	}

	checkers, ctx := setupMultipleCheckers(t, config.Policy{Limit: uint64(capacity), Period: time.Duration(capacity) * time.Second}, instances)
	var failed, allowed, denied, outOfRange atomic.Int64

	now := time.Unix(1_700_000_000, 0).UTC()
	start := make(chan struct{})
	var remainings [capacity]atomic.Int64
	var firstError atomic.Pointer[error]
	var wg sync.WaitGroup
	for i := range calls {
		c := checkers[i%len(checkers)]
		wg.Go(func() {
			<-start
			decision, err := c.checkAt(ctx, "user-123", now)

			if err != nil {
				failed.Add(1)
				firstError.CompareAndSwap(nil, &err)
				return
			}

			if decision.Allowed {
				allowed.Add(1)
				if decision.Remaining < capacity {
					remainings[decision.Remaining].Add(1)
				} else {
					outOfRange.Add(1)
				}
			} else {
				denied.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()

	if firstErr := firstError.Load(); firstErr != nil {
		t.Errorf("first error captured: %v", *firstErr)
	}

	remainingDivergences := []string{}
	for i := range capacity {
		gotRemaining := remainings[i].Load()

		if gotRemaining != 1 {
			remainingDivergences = append(remainingDivergences, fmt.Sprintf("remaining=%d seen %d times", i, gotRemaining))
		}
	}

	if len(remainingDivergences) != 0 {
		t.Errorf("remaining divergences: %v", remainingDivergences)
	}

	gotFailed, gotAllowed, gotDenied, gotOutOfRange := failed.Load(), allowed.Load(), denied.Load(), outOfRange.Load()

	if gotAllowed != int64(capacity) || gotDenied != int64(calls-capacity) || gotFailed != 0 || gotOutOfRange != 0 {
		t.Fatalf("allowed=%d denied=%d errors=%d outOfRange=%d, want %d/%d/%d/%d", gotAllowed, gotDenied, gotFailed, gotOutOfRange, capacity, calls-capacity, 0, 0)
	}
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
