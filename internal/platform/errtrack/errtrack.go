// Package errtrack captures the errors users actually hit and hands them to a
// sink for storage, so the admin error screen shows what is happening in
// production rather than staying empty.
//
// Why a package rather than a call at each error site: there were already three
// places that turn a failure into a response — httpx.Error for API replies,
// httpx.Recover for panics, and the UI's own error page — and each logged to
// slog in its own shape. Logs are not a record anyone can search from the admin
// panel, and adding a database write to each site would have coupled the
// platform layer to a business module, which the module boundaries forbid.
//
// The shape is the same one used elsewhere in this codebase for a platform
// package that needs a business capability: an interface here, an
// implementation in the module, wired together at start-up in cmd/server.
//
// Three properties matter more than completeness, and they are why this is not
// simply a synchronous insert:
//
//   - A request must never wait on error logging. Capture hands off to a
//     buffered channel and returns.
//   - A failure to record an error must never become an error itself. The
//     writer swallows sink failures into slog.
//   - A hot failure loop must not fill the table. Identical errors are
//     throttled by fingerprint.
package errtrack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Level classifies how bad an event is. It matches the values the admin screen
// filters on.
const (
	LevelCritical = "CRITICAL"
	LevelError    = "ERROR"
	LevelWarning  = "WARNING"
)

// Event is one captured failure, in terms the platform layer can express
// without knowing how it will be stored.
type Event struct {
	Level          string
	Message        string
	ExceptionClass string
	StackTrace     string
	FilePath       string
	LineNumber     int

	HTTPMethod string
	URLPath    string
	IPAddress  string
	UserAgent  string
	RequestID  string
	StatusCode int

	UserID           *int64
	UserName         string
	UserEmail        string
	OrganizationName string

	OccurredAt time.Time
}

// Sink stores a captured event. It is implemented outside this package, by
// whichever module owns the error table.
//
// Implementations are called from a background goroutine with a context that is
// not tied to the request, because the request is usually finished by then.
type Sink interface {
	Capture(ctx context.Context, e Event) error
}

// Tracker owns the queue and the throttle.
type Tracker struct {
	sink Sink
	log  *slog.Logger

	queue chan Event
	done  chan struct{}
	wg    sync.WaitGroup

	mu       sync.Mutex
	lastSeen map[string]time.Time

	// dropped counts events discarded because the queue was full. It is
	// reported when the tracker stops, so a flood is visible rather than
	// silent.
	dropped int64
}

// Config tunes the tracker. The zero value is usable.
type Config struct {
	// QueueSize bounds how many events may wait to be written. Beyond this,
	// events are dropped rather than allowed to slow requests down.
	QueueSize int
	// ThrottleWindow is how long an identical error is suppressed after being
	// recorded once. A panic in a hot path would otherwise write thousands of
	// identical rows and bury everything else.
	ThrottleWindow time.Duration
	// WriteTimeout bounds a single sink write.
	WriteTimeout time.Duration
}

func (c Config) withDefaults() Config {
	if c.QueueSize <= 0 {
		c.QueueSize = 256
	}
	if c.ThrottleWindow <= 0 {
		c.ThrottleWindow = 30 * time.Second
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = 5 * time.Second
	}
	return c
}

// New starts a tracker writing to sink. Stop must be called to drain it.
func New(sink Sink, log *slog.Logger, cfg Config) *Tracker {
	cfg = cfg.withDefaults()
	if log == nil {
		log = slog.Default()
	}
	t := &Tracker{
		sink:     sink,
		log:      log,
		queue:    make(chan Event, cfg.QueueSize),
		done:     make(chan struct{}),
		lastSeen: make(map[string]time.Time),
	}
	t.wg.Add(1)
	go t.run(cfg)
	return t
}

func (t *Tracker) run(cfg Config) {
	defer t.wg.Done()
	// The writer must outlive individual requests, so it uses its own context.
	for e := range t.queue {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.WriteTimeout)
		if err := t.sink.Capture(ctx, e); err != nil {
			// Recording a failure must not produce a second failure. This is
			// the one place the error stops.
			t.log.ErrorContext(ctx, "errtrack: sink write failed",
				"error", err, "event_message", e.Message, "path", e.URLPath)
		}
		cancel()
	}
	close(t.done)
}

// Stop drains the queue and waits for the writer.
func (t *Tracker) Stop() {
	if t == nil {
		return
	}
	close(t.queue)
	<-t.done
	t.wg.Wait()
	if t.dropped > 0 {
		t.log.Warn("errtrack: events dropped because the queue was full", "count", t.dropped)
	}
}

// Capture queues an event. It never blocks and never returns an error: nothing
// a caller could do about a failure here is worth the code at the call site.
func (t *Tracker) Capture(e Event) {
	if t == nil || t.sink == nil {
		return
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now()
	}
	if e.Level == "" {
		e.Level = LevelError
	}
	if !t.admit(e) {
		return
	}

	select {
	case t.queue <- e:
	default:
		// Full queue means the database is slower than the failure rate. The
		// request is more important than the record.
		t.mu.Lock()
		t.dropped++
		t.mu.Unlock()
	}
}

// admit reports whether this event should be recorded, or suppressed as a
// repeat of one recorded moments ago.
func (t *Tracker) admit(e Event) bool {
	key := fingerprint(e)
	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()

	if last, seen := t.lastSeen[key]; seen && now.Sub(last) < throttleWindow {
		return false
	}
	t.lastSeen[key] = now

	// The map is bounded by eviction rather than by a cache library: distinct
	// errors are few, and a flood of unique ones must not become a leak.
	if len(t.lastSeen) > maxFingerprints {
		for k, at := range t.lastSeen {
			if now.Sub(at) > throttleWindow {
				delete(t.lastSeen, k)
			}
		}
	}
	return true
}

const (
	throttleWindow  = 30 * time.Second
	maxFingerprints = 2048
)

// fingerprint identifies "the same error again".
//
// The message is included but the request path is not: the same nil-pointer
// dereference reached from three URLs is one bug, and throttling it as one is
// the point. The location is what distinguishes two failures that happen to
// share a message.
func fingerprint(e Event) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		e.Level, e.ExceptionClass, e.Message, e.FilePath,
	}, "|")))
	return hex.EncodeToString(sum[:8])
}

// Caller returns the file and line of the code skip frames above, for events
// that have no stack of their own.
func Caller(skip int) (string, int) {
	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return "", 0
	}
	return trimPath(file), line
}

// trimPath shortens an absolute build path to something readable in a table:
// the module-relative path, or the last two segments when that fails.
func trimPath(file string) string {
	if i := strings.Index(file, "dawa24-store/"); i >= 0 {
		return file[i+len("dawa24-store/"):]
	}
	file = strings.ReplaceAll(file, "\\", "/")
	parts := strings.Split(file, "/")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return file
}

// FromRequest fills in the request-shaped fields of an event.
//
// It deliberately records no request body. A payload can carry a password, a
// session token or a national ID, and an error table is exactly the wrong place
// for those: it is read by staff, exported, and kept.
func FromRequest(r *http.Request, e Event) Event {
	if r == nil {
		return e
	}
	e.HTTPMethod = r.Method
	e.URLPath = r.URL.Path
	e.UserAgent = r.UserAgent()
	e.IPAddress = clientIP(r)
	return e
}

var (
	clientIPMu       sync.RWMutex
	clientIPResolver func(r *http.Request) string
)

// SetClientIPResolver installs the trusted client IP resolver (e.g. backed by httpx.ClientIP).
func SetClientIPResolver(fn func(r *http.Request) string) {
	clientIPMu.Lock()
	defer clientIPMu.Unlock()
	clientIPResolver = fn
}

func clientIP(r *http.Request) string {
	clientIPMu.RLock()
	resolver := clientIPResolver
	clientIPMu.RUnlock()
	if resolver != nil {
		return resolver(r)
	}

	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return strings.TrimSpace(v)
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		return host[:i]
	}
	return host
}

// typeName is the concrete type of a value, used as the closest equivalent Go
// has to an exception class. A *apperr.Error reports its kind as well, because
// "conflict" and "internal" are different problems wearing the same type.
func typeName(v any) string {
	if v == nil {
		return ""
	}
	if k, ok := v.(interface{ KindName() string }); ok {
		if name := k.KindName(); name != "" {
			return "apperr." + name
		}
	}
	return reflect.TypeOf(v).String()
}
