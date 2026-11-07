package httpx

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type counterHook struct{ count int64 }

func (h *counterHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *counterHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *counterHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		switch cmd.Name() {
		case "incr":
			h.count++
			cmd.(*redis.IntCmd).SetVal(h.count)
		case "expire":
			cmd.(*redis.BoolCmd).SetVal(true)
		case "ttl":
			cmd.(*redis.DurationCmd).SetVal(time.Minute)
		default:
			return errors.New("unexpected Redis command")
		}
		return nil
	}
}

func TestAllowKey(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = client.Close() })
	client.AddHook(&counterHook{})
	limiter := NewLimiter(client, "test:")
	for i := int64(1); i <= 4; i++ {
		allowed, count, retry, err := limiter.AllowKey(ctx, "key", 3, time.Minute)
		if err != nil || allowed != (i <= 3) || count != i || retry != time.Minute {
			t.Errorf("request %d: allowed=%v count=%d retry=%v err=%v", i, allowed, count, retry, err)
		}
	}
	allowed, _, _, err := NewLimiter(nil, "test:").AllowKey(ctx, "key", 3, time.Minute)
	if !allowed || err != nil {
		t.Errorf("nil Redis: allowed=%v err=%v", allowed, err)
	}
	broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 200 * time.Millisecond})
	t.Cleanup(func() { _ = broken.Close() })
	allowed, _, _, err = NewLimiter(broken, "test:").AllowKey(ctx, "key", 3, time.Minute)
	if !allowed || err == nil {
		t.Errorf("Redis error: allowed=%v err=%v", allowed, err)
	}
}
