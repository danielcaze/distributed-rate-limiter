package redischeck_test

import (
	"context"
	"testing"

	"github.com/danielcaze/distributed-rate-limiter/config"
	"github.com/danielcaze/distributed-rate-limiter/redischeck"
	"github.com/danielcaze/distributed-rate-limiter/testsupport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRedis_Connection(t *testing.T) {
	ctx := t.Context()
	addr, cleanup, err := testsupport.RedisContainer(ctx)

	if err != nil {
		t.Fatalf("container: %v", err)
	}

	checker := redischeck.New(addr, config.Policy{})

	_, err = checker.Check(ctx, "")

	// Update this check once issue #3 implements real admission; Check will
	// stop always returning Unimplemented.
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("check: %v", err)
	}

	t.Cleanup(func() {
		err := cleanup(context.Background())
		if err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
}
