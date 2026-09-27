package redischeck

import (
	"context"
	"testing"
	"time"

	"github.com/danielcaze/distributed-rate-limiter/config"
	"github.com/danielcaze/distributed-rate-limiter/limiter"
	"github.com/danielcaze/distributed-rate-limiter/testsupport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestChecker_BucketsStartFullPerKey(t *testing.T) {
	checker, ctx, _ := setupChecker(t, config.Policy{Capacity: 2, RefillTokensPerSecond: 1})
	now := time.Unix(1_700_000_000, 0).UTC()

	firstA, err := checker.checkAt(ctx, "key-a", now)
	if err != nil {
		t.Fatalf("first check for key-a: %v", err)
	}
	assertDecision(t, firstA, true, 1)

	firstB, err := checker.checkAt(ctx, "key-b", now)
	if err != nil {
		t.Fatalf("first check for key-b: %v", err)
	}
	assertDecision(t, firstB, true, 1)

	secondA, err := checker.checkAt(ctx, "key-a", now)
	if err != nil {
		t.Fatalf("second check for key-a: %v", err)
	}
	assertDecision(t, secondA, true, 0)

	secondB, err := checker.checkAt(ctx, "key-b", now)
	if err != nil {
		t.Fatalf("second check for key-b: %v", err)
	}
	assertDecision(t, secondB, true, 0)
}

func TestChecker_LastTokenAndNextCallBeforeRefill(t *testing.T) {
	checker, ctx, _ := setupChecker(t, config.Policy{Capacity: 2, RefillTokensPerSecond: 1})
	key := "user-123"
	base := time.Unix(1_700_000_000, 0).UTC()

	first, err := checker.checkAt(ctx, key, base)
	if err != nil {
		t.Fatalf("first check: %v", err)
	}
	assertDecision(t, first, true, 1)
	assertResetAt(t, first, base.Add(time.Second))

	last, err := checker.checkAt(ctx, key, base)
	if err != nil {
		t.Fatalf("last admission: %v", err)
	}
	assertDecision(t, last, true, 0)
	assertResetAt(t, last, base.Add(2*time.Second))

	denied, err := checker.checkAt(ctx, key, base)
	if err != nil {
		t.Fatalf("check after last admission: %v", err)
	}
	assertDecision(t, denied, false, 0)
	assertResetAt(t, denied, base.Add(2*time.Second))
	assertRetryAfter(t, denied, time.Second)

	beforeRefill, err := checker.checkAt(ctx, key, base.Add(999*time.Millisecond))
	if err != nil {
		t.Fatalf("check before refill: %v", err)
	}
	assertDecision(t, beforeRefill, false, 0)
	assertResetAt(t, beforeRefill, base.Add(2*time.Second))
	assertRetryAfter(t, beforeRefill, time.Millisecond)

	nextAdmission, err := checker.checkAt(ctx, key, base.Add(time.Second))
	if err != nil {
		t.Fatalf("check at next admission: %v", err)
	}
	assertDecision(t, nextAdmission, true, 0)
	assertResetAt(t, nextAdmission, base.Add(3*time.Second))
	assertRetryAfter(t, nextAdmission, 0)
}

func TestChecker_DenialDoesNotConsumeAndRefillsContinuously(t *testing.T) {
	checker, ctx, _ := setupChecker(t, config.Policy{Capacity: 1, RefillTokensPerSecond: 2})
	key := "user-123"
	base := time.Unix(1_700_000_000, 0).UTC()

	first, err := checker.checkAt(ctx, key, base)
	if err != nil {
		t.Fatalf("first admission: %v", err)
	}
	assertDecision(t, first, true, 0)

	halfRefilled, err := checker.checkAt(ctx, key, base.Add(250*time.Millisecond))
	if err != nil {
		t.Fatalf("check at half token: %v", err)
	}
	assertDecision(t, halfRefilled, false, 0)
	assertRetryAfter(t, halfRefilled, 250*time.Millisecond)
	assertResetAt(t, halfRefilled, base.Add(500*time.Millisecond))

	moreRefilled, err := checker.checkAt(ctx, key, base.Add(400*time.Millisecond))
	if err != nil {
		t.Fatalf("check during continuous refill: %v", err)
	}
	assertDecision(t, moreRefilled, false, 0)
	assertDurationWithin(t, moreRefilled.RetryAfter, 100*time.Millisecond, time.Microsecond)
	assertResetAt(t, moreRefilled, base.Add(500*time.Millisecond))

	fullyRefilled, err := checker.checkAt(ctx, key, base.Add(500*time.Millisecond))
	if err != nil {
		t.Fatalf("check at full refill: %v", err)
	}
	assertDecision(t, fullyRefilled, true, 0)
	assertResetAt(t, fullyRefilled, base.Add(time.Second))
}

func TestChecker_TestTimeIsInternalAndProductionUsesRedisTime(t *testing.T) {
	checker, ctx, _ := setupChecker(t, config.Policy{Capacity: 1, RefillTokensPerSecond: 1})
	key := "user-123"
	fakeTime := time.Unix(1, 0).UTC()

	testDecision, err := checker.checkAt(ctx, key, fakeTime)
	if err != nil {
		t.Fatalf("check at test time: %v", err)
	}
	assertDecision(t, testDecision, true, 0)

	productionDecision, err := checker.Check(ctx, key)
	if err != nil {
		t.Fatalf("production check: %v", err)
	}
	assertDecision(t, productionDecision, true, 0)
}

func TestChecker_RedisFailureIsNotQuotaDenial(t *testing.T) {
	checker, ctx, closeChecker := setupChecker(t, config.Policy{Capacity: 1, RefillTokensPerSecond: 1})
	key := "user-123"
	now := time.Unix(1_700_000_000, 0).UTC()

	_, err := checker.checkAt(ctx, key, now)
	if err != nil {
		t.Fatalf("first admission: %v", err)
	}
	denied, err := checker.checkAt(ctx, key, now)
	if err != nil {
		t.Fatalf("quota denial: %v", err)
	}
	assertDecision(t, denied, false, 0)

	if err := closeChecker(); err != nil {
		t.Fatalf("close Redis client: %v", err)
	}
	_, err = checker.Check(ctx, key)
	if got := status.Code(err); got != codes.Unavailable {
		t.Fatalf("status.Code(error) = %s, want %s (error: %v)", got, codes.Unavailable, err)
	}
}

func setupChecker(t *testing.T, policy config.Policy) (*Checker, context.Context, func() error) {
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

	checker, err := New(ctx, addr, policy)
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

func assertDecision(t *testing.T, decision limiter.Decision, wantAllowed bool, wantRemaining uint64) {
	t.Helper()
	if decision.Allowed != wantAllowed {
		t.Errorf("decision.Allowed = %v, want %v", decision.Allowed, wantAllowed)
	}
	if decision.Remaining != wantRemaining {
		t.Errorf("decision.Remaining = %d, want %d", decision.Remaining, wantRemaining)
	}
}

func assertResetAt(t *testing.T, decision limiter.Decision, want time.Time) {
	t.Helper()
	if !decision.ResetAt.Equal(want) {
		t.Errorf("decision.ResetAt = %s, want %s", decision.ResetAt, want)
	}
}

func assertRetryAfter(t *testing.T, decision limiter.Decision, want time.Duration) {
	t.Helper()
	if decision.RetryAfter != want {
		t.Errorf("decision.RetryAfter = %s, want %s", decision.RetryAfter, want)
	}
}

func assertDurationWithin(t *testing.T, got, want, tolerance time.Duration) {
	t.Helper()
	difference := got - want
	if difference < -tolerance || difference > tolerance {
		t.Errorf("duration = %s, want %s within %s", got, want, tolerance)
	}
}
