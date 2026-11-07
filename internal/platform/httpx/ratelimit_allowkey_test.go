package httpx

import (
	"context"
	"errors"
	"strings"
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

// keyCaptureHook records every Redis key a command touches.
type keyCaptureHook struct {
	keys []string
}

func (h *keyCaptureHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *keyCaptureHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *keyCaptureHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		if len(args) > 1 {
			if key, ok := args[1].(string); ok {
				h.keys = append(h.keys, key)
			}
		}
		switch cmd.Name() {
		case "get":
			cmd.(*redis.StringCmd).SetErr(redis.Nil)
			return redis.Nil
		case "incr":
			cmd.(*redis.IntCmd).SetVal(1)
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

// TestKeyMethodsUseOwnNamespace proves the ERP counters are namespaced by the
// limiter's prefix and can never touch a website limiter's keys, even when a
// caller hands both limiters the same key string.
func TestKeyMethodsUseOwnNamespace(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = client.Close() })
	hook := &keyCaptureHook{}
	client.AddHook(hook)
	limiter := NewLimiter(client, "dawa24:erp:ratelimit:")
	if _, _, _, err := limiter.AllowKey(ctx, "dawa24:erp:key:7", 10, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, _, err := limiter.FailedAuthCount(ctx, "dawa24:erp:auth-fail:203.0.113.9"); err != nil {
		t.Fatal(err)
	}
	if err := limiter.IncrementAuthFailure(ctx, "dawa24:erp:auth-fail:203.0.113.9", time.Minute); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"dawa24:erp:ratelimit:dawa24:erp:key:7":                 false,
		"dawa24:erp:ratelimit:dawa24:erp:auth-fail:203.0.113.9": false,
	}
	for _, key := range hook.keys {
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, seen := range want {
		if !seen {
			t.Errorf("expected namespaced key %q was never used (touched: %v)", key, hook.keys)
		}
	}
	for _, key := range hook.keys {
		if !strings.HasPrefix(key, "dawa24:erp:ratelimit:") {
			t.Errorf("key %q escaped the limiter namespace", key)
		}
	}
}
