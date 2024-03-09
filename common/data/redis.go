package data

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
	"github.com/st-matskevich/go-matchmaker/common"
)

const REDIS_DB_ID = 0
const REDIS_QUEUE_LIST_KEY = "queue"

type RedisDataProvider struct {
	client *redis.Client
}

func (provider *RedisDataProvider) Set(req common.RequestBody) (*common.RequestBody, error) {
	ctx := context.Background()
	setArgs := redis.SetArgs{Get: true}
	bytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	result, err := provider.client.SetArgs(ctx, req.ID, bytes, setArgs).Result()
	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	prev := common.RequestBody{}
	err = json.Unmarshal([]byte(result), &prev)
	if err != nil {
		return nil, err
	}

	return &prev, nil
}

func (provider *RedisDataProvider) ListPush(ID string) error {
	ctx := context.Background()
	err := provider.client.LPush(ctx, REDIS_QUEUE_LIST_KEY, ID).Err()
	if err != nil {
		return err
	}

	return nil
}

func (provider *RedisDataProvider) ListPop() (string, error) {
	ctx := context.Background()
	val, err := provider.client.BRPop(ctx, 0, REDIS_QUEUE_LIST_KEY).Result()
	if err != nil {
		return "", err
	}

	return val[1], nil
}

func CreateRedisDataProvider(url string) (*RedisDataProvider, error) {
	ctx := context.Background()
	clientRedis := redis.NewClient(&redis.Options{
		Addr: url,
		DB:   REDIS_DB_ID,
	})

	_, err := clientRedis.Ping(ctx).Result()
	if err != nil {
		return nil, err
	}

	return &RedisDataProvider{client: clientRedis}, nil
}
