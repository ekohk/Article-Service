package cache

import (
	"context"
	"testing"
	"time"

	"github.com/kumparan/article-service/internal/config"
)

func TestRedisGetSetUnavailableIsReported(t *testing.T) {
	r := NewRedis(config.RedisConfig{Addr: "127.0.0.1:1"})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, _, err := r.Get(ctx, "articles:test"); err == nil {
		t.Fatal("expected Redis connection error")
	}
	_ = r.Close()
}
