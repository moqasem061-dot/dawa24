package safe

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestGoRecoversPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	// This should panic inside the goroutine, but safe.Go must recover it
	// and allow the test process to continue normally.
	Go(nil, "test_panic_routine", func() {
		defer wg.Done()
		panic("simulated critical crash")
	})

	wg.Wait()
}

func TestGoWithRecoverCallsHandler(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	var recovered atomic.Bool
	GoWithRecover(nil, "test_custom_recover", func() {
		panic("custom panic test")
	}, func(p any) {
		defer wg.Done()
		if p == "custom panic test" {
			recovered.Store(true)
		}
	})

	wg.Wait()
	if !recovered.Load() {
		t.Fatal("expected onPanic handler to be called")
	}
}
