package interfaces

import (
	"context"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type RedisClient interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	SetArgs(ctx context.Context, key string, value interface{}, a redis.SetArgs) *redis.StatusCmd
	LPush(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
}
