package errtrack

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// recordingSink collects what the tracker writes.
type recordingSink struct {
	mu     sync.Mutex
	events []Event
	fail   error
}

func (s *recordingSink) Capture(_ context.Context, e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return s.fail
}

func (s *recordingSink) all() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.events...)
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCaptureReachesTheSink(t *testing.T) {
	sink := &recordingSink{}
	tr := New(sink, quietLogger(), Config{})

	tr.Capture(Event{Message: "database is down", Level: LevelCritical})
	tr.Stop()

	got := sink.all()
	if len(got) != 1 {
		t.Fatalf("captured %d events; want 1", len(got))
	}
	if got[0].Message != "database is down" {
		t.Errorf("Message = %q", got[0].Message)
	}
	if got[0].OccurredAt.IsZero() {
		t.Error("OccurredAt was not filled in")
	}
}

// A failure loop must not fill the table with the same row. This is the
// property that keeps the error screen readable when something breaks badly.
func TestIdenticalErrorsAreThrottled(t *testing.T) {
	sink := &recordingSink{}
	tr := New(sink, quietLogger(), Config{})

	for i := 0; i < 50; i++ {
		tr.Capture(Event{Message: "same failure", FilePath: "internal/ui/x.go"})
	}
	tr.Stop()

	if n := len(sink.all()); n != 1 {
		t.Fatalf("recorded %d copies of one error; want 1", n)
	}
}

func TestDistinctErrorsAreAllRecorded(t *testing.T) {
	sink := &recordingSink{}
	tr := New(sink, quietLogger(), Config{})

	tr.Capture(Event{Message: "first", FilePath: "a.go"})
	tr.Capture(Event{Message: "second", FilePath: "b.go"})
	tr.Capture(Event{Message: "third", FilePath: "c.go"})
	tr.Stop()

	if n := len(sink.all()); n != 3 {
		t.Fatalf("recorded %d distinct errors; want 3", n)
	}
}

// Recording an error must never become an error. A sink that fails should be
// logged and forgotten, not propagated back into the request.
func TestSinkFailureIsSwallowed(t *testing.T) {
	sink := &recordingSink{fail: errors.New("insert failed")}
	tr := New(sink, quietLogger(), Config{})

	tr.Capture(Event{Message: "boom"})
	tr.Stop() // would deadlock or panic if the writer gave up on error
}

// Capture is called from request paths and must never block, even when the
// queue is full and nothing is draining it.
func TestCaptureDoesNotBlockWhenQueueIsFull(t *testing.T) {
	blocked := make(chan struct{})
	sink := blockingSink{release: blocked}
	tr := New(sink, quietLogger(), Config{QueueSize: 1})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			// Distinct messages, so the throttle is not what saves us here.
			tr.Capture(Event{Message: "e", FilePath: string(rune('a' + i%26)), LineNumber: i})
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Capture blocked when the queue was full")
	}

	close(blocked)
	tr.Stop()
}

type blockingSink struct{ release chan struct{} }

func (b blockingSink) Capture(context.Context, Event) error {
	<-b.release
	return nil
}

func TestReportIsSafeWithNoTrackerInstalled(t *testing.T) {
	Install(nil)
	Report(Event{Message: "nobody is listening"})
	ReportRequest(context.Background(), httptest.NewRequest("GET", "/x", nil),
		errors.New("x"), LevelError, 500)
}

func TestFromRequestRecordsNoBody(t *testing.T) {
	r := httptest.NewRequest("POST", "/auth/login", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	r.Header.Set("User-Agent", "probe/1.0")

	e := FromRequest(r, Event{})

	if e.IPAddress != "203.0.113.9" {
		t.Errorf("IPAddress = %q; want the client, not the proxy", e.IPAddress)
	}
	if e.HTTPMethod != "POST" || e.URLPath != "/auth/login" {
		t.Errorf("method/path = %q %q", e.HTTPMethod, e.URLPath)
	}
	if e.UserAgent != "probe/1.0" {
		t.Errorf("UserAgent = %q", e.UserAgent)
	}
}

// The stack of a panic starts in the runtime and passes through this package
// and the recovery middleware. What an operator needs is the first frame in
// application code.
func TestPanicOriginSkipsRuntimeAndMiddleware(t *testing.T) {
	stack := `goroutine 1 [running]:
runtime/debug.Stack()
	/usr/local/go/src/runtime/debug/stack.go:24 +0x5e
github.com/muhiya/dawa24-store/internal/platform/httpx.Recover.func1.1()
	/build/dawa24-store/internal/platform/httpx/middleware.go:57 +0x11d
github.com/muhiya/dawa24-store/internal/ui.(*UIHandler).CartPage()
	/build/dawa24-store/internal/ui/customer_cart_handlers.go:142 +0x2a
`
	file, line := PanicOrigin(stack)
	if file != "internal/ui/customer_cart_handlers.go" {
		t.Errorf("file = %q; want the application frame", file)
	}
	if line != 142 {
		t.Errorf("line = %d; want 142", line)
	}
}

func TestActorResolverAttributesTheError(t *testing.T) {
	sink := &recordingSink{}
	tr := New(sink, quietLogger(), Config{})
	Install(tr)
	defer Install(nil)

	SetActorResolver(func(context.Context) (int64, string, string, string, bool) {
		return 42, "صيدلية النور", "pharmacy@example.com", "customer", true
	})
	defer SetActorResolver(nil)

	ReportRequest(context.Background(), httptest.NewRequest("GET", "/cart", nil),
		errors.New("connection refused"), LevelError, 500)
	tr.Stop()

	got := sink.all()
	if len(got) != 1 {
		t.Fatalf("captured %d events; want 1", len(got))
	}
	if got[0].UserID == nil || *got[0].UserID != 42 {
		t.Error("the error was not attributed to the user who hit it")
	}
	if got[0].UserEmail != "pharmacy@example.com" {
		t.Errorf("UserEmail = %q", got[0].UserEmail)
	}
}

func TestContextCanceledIsNotReported(t *testing.T) {
	sink := &recordingSink{}
	tr := New(sink, quietLogger(), Config{})
	Install(tr)
	defer Install(nil)

	// 1. Direct context.Canceled
	ReportRequest(context.Background(), httptest.NewRequest("GET", "/customer/catalog", nil),
		context.Canceled, LevelError, 500)

	// 2. Wrapped context canceled error
	ReportRequest(context.Background(), httptest.NewRequest("GET", "/customer/catalog", nil),
		errors.New("context canceled"), LevelError, 500)

	// 3. Canceled request context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("GET", "/customer/catalog", nil).WithContext(canceledCtx)
	ReportRequest(canceledCtx, req, errors.New("read error"), LevelError, 500)

	// 4. Report with message containing context canceled
	Report(Event{Message: "context canceled", Level: LevelError})

	tr.Stop()

	if len(sink.all()) != 0 {
		t.Fatalf("expected 0 events for context cancellations, got %d", len(sink.all()))
	}
}

func TestSetClientIPResolver(t *testing.T) {
	defer SetClientIPResolver(nil)

	SetClientIPResolver(func(r *http.Request) string {
		return "198.51.100.42"
	})

	r := httptest.NewRequest("GET", "/test", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")

	e := FromRequest(r, Event{})
	if e.IPAddress != "198.51.100.42" {
		t.Errorf("expected IP 198.51.100.42 from resolver, got %q", e.IPAddress)
	}
}
