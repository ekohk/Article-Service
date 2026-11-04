package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kumparan/article-service/internal/config"
)

type Redis struct {
	client *redis.Client
}

func NewRedis(cfg config.RedisConfig) *Redis {
	return &Redis{client: redis.NewClient(&redis.Options{
		Addr: cfg.Addr, Password: cfg.Password, DB: cfg.DB,
	})}
}

func (r *Redis) Get(ctx context.Context, key string) ([]byte, bool, error) {
	value, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (r *Redis) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *Redis) Close() error {
	return r.client.Close()
}
