// Package limiter defines the admission seam used by the future gRPC adapter.
package limiter

import (
	"context"
	"time"
)

// Decision describes the bucket immediately after one admission check.
// Times are based on Redis server time in production.
type Decision struct {
	Allowed    bool
	Remaining  uint64
	ResetAt    time.Time
	RetryAfter time.Duration
}

// Checker checks a trusted backend's opaque bucket key against the shared policy.
// A denied admission returns a Decision with Allowed=false and a nil error.
// An error means the admission result is unknown; callers must not treat it as denial.
type Checker interface {
	Check(ctx context.Context, key string) (Decision, error)
}
