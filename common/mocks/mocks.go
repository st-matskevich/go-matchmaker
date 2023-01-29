package mocks

import (
	"context"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/mock"
)

type HTTPClientMock struct {
	mock.Mock
}

func (mocked *HTTPClientMock) Do(req *http.Request) (*http.Response, error) {
	args := mocked.Called(req)
	return args.Get(0).(*http.Response), args.Error(1)
}

type RedisClientMock struct {
	mock.Mock
}

func (mocked *RedisClientMock) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	args := mocked.Called(ctx, key, value, expiration)
	return args.Get(0).(*redis.StatusCmd)
}

func (mocked *RedisClientMock) SetArgs(ctx context.Context, key string, value interface{}, a redis.SetArgs) *redis.StatusCmd {
	args := mocked.Called(ctx, key, value, a)
	return args.Get(0).(*redis.StatusCmd)
}

func (mocked *RedisClientMock) LPush(ctx context.Context, key string, values ...interface{}) *redis.IntCmd {
	args := mocked.Called(ctx, key, values)
	return args.Get(0).(*redis.IntCmd)
}
