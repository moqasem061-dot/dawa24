package progress_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/platform/progress"
)

// The stream has to survive the real middleware chain.
//
// A progress bar that does not move is the whole complaint this work exists to
// answer, and the ways it can fail to move are almost all outside the handler:
// a ResponseWriter wrapper that does not implement http.Flusher makes
// progress.Stream fall back to a single JSON snapshot; a compression middleware
// buffers the events until the connection closes; a request deadline cuts the
// connection at twenty-five seconds. None of those are visible in a unit test
// of the handler on its own, and every one of them presents to the user
// identically — as a page that needs a refresh.
//
// So this mounts the chain from cmd/server/main.go, in its order, and asserts
// that an event published after the client connected actually arrives.
func TestStreamSurvivesTheServerMiddlewareChain(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := progress.NewHub()

	const runID = "run-under-test"
	base := time.Now()

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Logger(log))
	r.Use(httpx.SecurityHeaders)
	r.Use(httpx.Locale)
	r.Use(httpx.RequestTimeout(25*time.Second, httpx.IsLongRunning))
	r.Use(chimw.Compress(5))

	r.Get("/vendor/ingest/{id}/stream", func(w http.ResponseWriter, req *http.Request) {
		fetch := func(context.Context) (progress.Snapshot, bool) {
			return progress.Snapshot{ID: runID, Percent: 10, Message: "بدأ", At: base}, true
		}
		progress.Stream(w, req, hub, runID, fetch)
	})

	srv := httptest.NewServer(r)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/vendor/ingest/abc/stream", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	// Exactly what EventSource sends, including the encoding a browser offers.
	// Compression is the specific thing that turns a live stream into one burst
	// at the end, so the test must give the middleware the chance to do it.
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream.\n"+
			"A non-streaming content type means Stream took its no-Flusher fallback: "+
			"some wrapper in the chain does not implement http.Flusher.", ct)
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("Content-Encoding = %q; a compressed event stream is buffered and "+
			"arrives as one burst when the connection closes", enc)
	}

	events := make(chan progress.Snapshot, 8)
	go func() {
		defer close(events)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var s progress.Snapshot
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &s); err == nil {
				events <- s
			}
		}
	}()

	// The opening snapshot must arrive before anything is published, or a bar
	// that opens on a run already in flight shows nothing until the next tick.
	first := recv(t, events, "the opening snapshot")
	if first.Percent != 10 {
		t.Errorf("opening snapshot = %d%%, want 10", first.Percent)
	}

	// And the point of the whole exercise: a snapshot published while the
	// client is connected reaches it, promptly, without the connection closing.
	hub.Publish(progress.Snapshot{ID: runID, Percent: 55, Message: "جارٍ المطابقة", At: base.Add(time.Second)})

	second := recv(t, events, "a published update")
	if second.Percent != 55 {
		t.Errorf("published update = %d%%, want 55", second.Percent)
	}

	hub.Publish(progress.Snapshot{ID: runID, Percent: 100, Done: true, At: base.Add(2 * time.Second)})
	final := recv(t, events, "the terminal snapshot")
	if !final.Done {
		t.Error("terminal snapshot did not report done; the bar would never finish")
	}
}

func recv(t *testing.T, ch <-chan progress.Snapshot, what string) progress.Snapshot {
	t.Helper()
	return recvWithin(t, ch, 5*time.Second, what)
}

func recvWithin(t *testing.T, ch <-chan progress.Snapshot, d time.Duration, what string) progress.Snapshot {
	t.Helper()
	select {
	case s, open := <-ch:
		if !open {
			t.Fatalf("stream closed before %s arrived", what)
		}
		return s
	case <-time.After(d):
		t.Fatalf("timed out waiting for %s — this is the bar that never moves", what)
		return progress.Snapshot{}
	}
}

// A stream whose publisher goes quiet must still finish.
//
// This is the bug behind "the bar stops and I have to refresh". Stream drops
// any snapshot older than the one it is showing — correct for events reordered
// across a process boundary. But most Fetch implementations have no timestamp
// to give and returned a ZERO one, and a zero time is before everything: once a
// single snapshot had been PUBLISHED, every subsequent safety read looked stale
// and was discarded. The one mechanism that recovers a stream whose publisher
// has stopped talking was switched off by the first thing the publisher said.
//
// Here the publisher reports 40% and then goes silent for good, while the
// source of truth moves on to a finished run. The stream must notice.
func TestStreamRecoversWhenThePublisherGoesQuiet(t *testing.T) {
	hub := progress.NewHub()
	const runID = "quiet-run"

	var mu sync.Mutex
	// No At, exactly as ingestSnapshot and the other tool snapshots return it.
	truth := progress.Snapshot{ID: runID, Percent: 40, State: "processing"}
	fetch := func(context.Context) (progress.Snapshot, bool) {
		mu.Lock()
		defer mu.Unlock()
		return truth, true
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		progress.Stream(w, r, hub, runID, fetch)
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()

	events := make(chan progress.Snapshot, 8)
	go func() {
		defer close(events)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var s progress.Snapshot
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &s); err == nil {
				events <- s
			}
		}
	}()

	recv(t, events, "the opening snapshot")

	// One published event with a real timestamp — after which the publisher is
	// never heard from again.
	hub.Publish(progress.Snapshot{ID: runID, Percent: 40, State: "processing", At: time.Now()})
	recv(t, events, "the published update")

	// The run finishes, and only the source of truth knows.
	mu.Lock()
	truth = progress.Snapshot{ID: runID, Percent: 100, State: "review", Done: true}
	mu.Unlock()

	// Longer than safetyPoll: the whole point is that the recovery comes from
	// the periodic read, not from an event.
	final := recvWithin(t, events, 20*time.Second, "the finish the safety poll must notice")
	if !final.Done {
		t.Fatalf("safety read reported done=%v percent=%d; the bar would sit where it "+
			"stopped until the page was reloaded", final.Done, final.Percent)
	}
}
