package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kumparan/article-service/internal/config"
)

func TestRedisIntegration_SetGetAndTTL(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set; skipping Redis integration test")
	}
	r := NewRedis(config.RedisConfig{Addr: addr, DB: 15})
	defer r.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	key := "article-service:test:cache"
	if err := r.Set(ctx, key, []byte(`{"ok":true}`), time.Second); err != nil {
		t.Fatal(err)
	}
	value, found, err := r.Get(ctx, key)
	if err != nil || !found || string(value) != `{"ok":true}` {
		t.Fatalf("value=%q found=%v err=%v", value, found, err)
	}
	time.Sleep(1100 * time.Millisecond)
	_, found, err = r.Get(ctx, key)
	if err != nil || found {
		t.Fatalf("expired value found=%v err=%v", found, err)
	}
	if err := r.client.Del(ctx, key).Err(); err != nil {
		t.Fatal(err)
	}
	_, found, err = r.Get(ctx, key)
	if err != nil || found {
		t.Fatalf("missing value found=%v err=%v", found, err)
	}
}
