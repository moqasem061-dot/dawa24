package cache

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestL1_GetSet(t *testing.T) {
	c := NewL1[string]()
	c.Set("k1", "v1", 100*time.Millisecond)

	val, ok := c.Get("k1")
	if !ok || val != "v1" {
		t.Fatalf("expected v1, got %v (ok=%v)", val, ok)
	}

	time.Sleep(120 * time.Millisecond)
	_, ok = c.Get("k1")
	if ok {
		t.Fatalf("expected expired key to return false")
	}
}

func TestL1_Remember_Singleflight(t *testing.T) {
	c := NewL1[int]()
	var calls atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			val, err := c.Remember("shared", time.Second, func() (int, error) {
				time.Sleep(20 * time.Millisecond)
				calls.Add(1)
				return 42, nil
			})
			if err != nil || val != 42 {
				t.Errorf("unexpected result: val=%d, err=%v", val, err)
			}
		}()
	}
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected compute to be called exactly once by singleflight, called %d times", got)
	}
}

func TestL1_Remember_Error(t *testing.T) {
	c := NewL1[string]()
	errExpected := errors.New("fail")

	_, err := c.Remember("err_key", time.Second, func() (string, error) {
		return "", errExpected
	})
	if !errors.Is(err, errExpected) {
		t.Fatalf("expected errExpected, got %v", err)
	}

	// Make sure errored result wasn't cached
	_, ok := c.Get("err_key")
	if ok {
		t.Fatalf("errored computation should not be cached")
	}
}

func TestL1_Invalidate(t *testing.T) {
	c := NewL1[string]()
	c.Set("cat:1", "c1", time.Minute)
	c.Set("cat:2", "c2", time.Minute)
	c.Set("brand:1", "b1", time.Minute)

	c.Invalidate("cat:1")
	if _, ok := c.Get("cat:1"); ok {
		t.Fatalf("expected cat:1 to be invalidated")
	}

	c.InvalidatePrefix("cat:")
	if _, ok := c.Get("cat:2"); ok {
		t.Fatalf("expected cat:2 to be invalidated by prefix")
	}
	if _, ok := c.Get("brand:1"); !ok {
		t.Fatalf("expected brand:1 to still exist")
	}

	c.InvalidateAll()
	if c.Len() != 0 {
		t.Fatalf("expected empty cache after InvalidateAll")
	}
}

func TestL1_CapacityCap(t *testing.T) {
	c := NewL1WithCapacity[int](10)
	for i := 0; i < 20; i++ {
		c.Set(string(rune('a'+i)), i, time.Minute)
	}
	if c.Len() > 10 {
		t.Fatalf("cache size %d exceeded capacity 10", c.Len())
	}
}
