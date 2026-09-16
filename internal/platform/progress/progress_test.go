package progress

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func snap(id string, pct int, done bool, at time.Time) Snapshot {
	return Snapshot{ID: id, Percent: pct, Done: done, At: at}
}

// A subscriber that arrives after the work started must be told where it is,
// not made to wait for the next change.
//
// An import whose last event was thirty seconds ago is not an import with no
// progress, and a bar that renders empty until something moves is the reason
// people reload the page and start the upload again.
func TestSubscribeSeesTheLastSnapshot(t *testing.T) {
	h := NewHub()
	now := time.Now()
	h.Publish(snap("run-1", 40, false, now))

	_, last, known, cancel := h.Subscribe("run-1")
	defer cancel()

	if !known {
		t.Fatal("a run with a published snapshot must report one to a new subscriber")
	}
	if last.Percent != 40 {
		t.Errorf("last.Percent = %d, want 40", last.Percent)
	}
}

// Every subscriber gets every publish.
func TestPublishFansOut(t *testing.T) {
	h := NewHub()
	const watchers = 5

	chans := make([]<-chan Snapshot, watchers)
	for i := range chans {
		ch, _, _, cancel := h.Subscribe("run-1")
		defer cancel()
		chans[i] = ch
	}

	h.Publish(snap("run-1", 55, false, time.Now()))

	for i, ch := range chans {
		select {
		case got := <-ch:
			if got.Percent != 55 {
				t.Errorf("watcher %d got %d%%, want 55", i, got.Percent)
			}
		case <-time.After(time.Second):
			t.Fatalf("watcher %d received nothing", i)
		}
	}
}

// A publisher must never be held up by a browser that is not reading.
//
// This is the import's own goroutine. If a stalled viewer could block it, one
// person with a frozen tab would slow the import down for the vendor who
// started it.
func TestPublishNeverBlocksOnASlowSubscriber(t *testing.T) {
	h := NewHub()
	ch, _, _, cancel := h.Subscribe("run-1")
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < subscriberBuffer*20; i++ {
			h.Publish(snap("run-1", i, false, time.Now()))
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Publish blocked on a subscriber that was not reading")
	}

	// And what it did keep is the newest, because that is the only value a
	// progress bar has any use for.
	var newest int
	for len(ch) > 0 {
		newest = (<-ch).Percent
	}
	if newest != subscriberBuffer*20-1 {
		t.Errorf("newest retained snapshot was %d%%, want the last published (%d%%)",
			newest, subscriberBuffer*20-1)
	}
}

// A snapshot that arrives behind a newer one is discarded. Once messages cross
// a process boundary they can be reordered, and a bar that goes backwards reads
// as a failure even when the run is healthy.
func TestOutOfOrderSnapshotsAreIgnored(t *testing.T) {
	h := NewHub()
	now := time.Now()
	h.Publish(snap("run-1", 80, false, now))
	h.Publish(snap("run-1", 20, false, now.Add(-time.Minute)))

	_, last, _, cancel := h.Subscribe("run-1")
	defer cancel()
	if last.Percent != 80 {
		t.Errorf("last.Percent = %d, want the newer 80", last.Percent)
	}
}

// Cancelling is idempotent and actually removes the watcher, or a long-lived
// process leaks a channel per page view.
func TestCancelIsIdempotentAndRemovesTheWatcher(t *testing.T) {
	h := NewHub()
	_, _, _, cancel := h.Subscribe("run-1")
	if got := h.Watchers("run-1"); got != 1 {
		t.Fatalf("Watchers = %d, want 1", got)
	}
	cancel()
	cancel()
	if got := h.Watchers("run-1"); got != 0 {
		t.Errorf("Watchers after cancel = %d, want 0", got)
	}
}

// Concurrent publishing and subscribing must not race. Run with -race.
func TestHubIsSafeUnderConcurrency(t *testing.T) {
	h := NewHub()
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; ; n++ {
				select {
				case <-stop:
					return
				default:
				}
				h.Publish(snap("run-1", n%100, false, time.Now()))
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				ch, _, _, cancel := h.Subscribe("run-1")
				<-time.After(time.Millisecond)
				select {
				case <-ch:
				default:
				}
				cancel()
			}
		}()
	}
	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// The stream delivers the current state at once and closes when the run is
// done, rather than holding a connection open for an event that will not come.
func TestStreamSendsCurrentStateThenClosesOnTerminal(t *testing.T) {
	h := NewHub()
	base := time.Now()

	var mu sync.Mutex
	current := snap("run-1", 30, false, base)
	fetch := func(context.Context) (Snapshot, bool) {
		mu.Lock()
		defer mu.Unlock()
		return current, true
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Stream(w, r, h, "run-1", fetch)
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	if resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Error("X-Accel-Buffering: no is missing; every deployed proxy would buffer the stream")
	}

	events := make(chan Snapshot, 8)
	go func() {
		defer close(events)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var s Snapshot
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &s); err == nil {
				events <- s
			}
		}
	}()

	first := recvSnapshot(t, events, "the opening snapshot")
	if first.Percent != 30 {
		t.Errorf("first snapshot = %d%%, want 30", first.Percent)
	}

	mu.Lock()
	current = snap("run-1", 100, true, base.Add(time.Second))
	mu.Unlock()
	h.Publish(current)

	final := recvSnapshot(t, events, "the terminal snapshot")
	if !final.Done || final.Percent != 100 {
		t.Errorf("final snapshot = %d%% done=%v, want 100%% done", final.Percent, final.Done)
	}

	// The terminal snapshot ends the stream; the body must reach EOF on its own.
	select {
	case _, open := <-events:
		if open {
			t.Error("the stream kept sending after the run finished")
		}
	case <-time.After(3 * time.Second):
		t.Error("the stream stayed open after the run finished")
	}
}

func recvSnapshot(t *testing.T, ch <-chan Snapshot, what string) Snapshot {
	t.Helper()
	select {
	case s, open := <-ch:
		if !open {
			t.Fatalf("stream closed before %s", what)
		}
		return s
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		return Snapshot{}
	}
}
