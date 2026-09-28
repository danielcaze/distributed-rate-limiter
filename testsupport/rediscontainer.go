// Package testsupport provides a real Redis fixture for later integration tests.
package testsupport

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

const redisImage = "redis:7.4.2-alpine"

// RedisContainer starts an isolated Redis and returns its host address and a
// cleanup function. The caller must defer cleanup immediately after success.
func RedisContainer(ctx context.Context) (string, func(context.Context) error, error) {
	container, err := tcredis.Run(ctx, redisImage)
	if err != nil {
		return "", nil, err
	}
	cleanup := func(ctx context.Context) error { return container.Terminate(ctx) }
	url, err := container.ConnectionString(ctx)
	if err != nil {
		_ = cleanup(context.Background())
		return "", nil, fmt.Errorf("Redis endpoint: %w", err)
	}
	options, err := redis.ParseURL(url)
	if err != nil {
		_ = cleanup(context.Background())
		return "", nil, fmt.Errorf("Redis URL: %w", err)
	}
	return options.Addr, cleanup, nil
}
