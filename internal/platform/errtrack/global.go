package errtrack

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/muhiya/dawa24-store/internal/platform/observability"
)

// The default tracker.
//
// The three places that turn a failure into a response — httpx.Error,
// httpx.Recover and the UI's error page — are middleware and helpers with no
// dependency injection of their own, and threading a tracker into each would
// mean changing every call site of every one of them. A process-wide tracker,
// installed once at start-up, is the smaller change and matches how the asset
// resolver and the gateway settings are already wired here.
//
// It is an atomic pointer rather than a plain variable because Install runs
// during start-up while the readiness probe may already be serving.
var global atomic.Pointer[Tracker]

// Install makes t the process-wide tracker. Passing nil disables capture, which
// is what tests and the CLI want.
func Install(t *Tracker) {
	global.Store(t)
}

// Installed reports whether capture is active. Callers use it to skip building
// an event when nothing would receive it.
func Installed() bool { return global.Load() != nil }

// Report queues an event on the process-wide tracker. It is a no-op when none
// is installed, so nothing has to guard its call sites.
func Report(e Event) {
	if strings.Contains(strings.ToLower(e.Message), "context canceled") {
		return
	}
	if t := global.Load(); t != nil {
		t.Capture(e)
	}
}

// ReportRequest captures a failure that happened while serving a request,
// filling in everything derivable from the request and its context.
//
// level is one of the Level constants. status is the HTTP status the client
// received, which is what separates "the user saw a 500" from "the user was
// told their input was invalid".
func ReportRequest(ctx context.Context, r *http.Request, err error, level string, status int) {
	if !Installed() || err == nil {
		return
	}

	// Normal client cancellations (user navigated away, closed tab, or connection aborted)
	// are not application failures and must not pollute the error tracker.
	if errors.Is(err, context.Canceled) ||
		(ctx != nil && errors.Is(ctx.Err(), context.Canceled)) ||
		(r != nil && errors.Is(r.Context().Err(), context.Canceled)) ||
		strings.Contains(strings.ToLower(err.Error()), "context canceled") {
		return
	}

	e := Event{
		Level:          level,
		Message:        err.Error(),
		ExceptionClass: exceptionClass(err),
		StatusCode:     status,
		RequestID:      observability.RequestIDFrom(ctx),
	}
	e.FilePath, e.LineNumber = Caller(1)
	e = FromRequest(r, e)
	e = withActor(ctx, e)
	Report(e)
}

// ActorResolver reports who is behind a request, for attributing an error.
//
// It is a variable rather than a direct call into authctx because authctx
// already imports httpx, and httpx captures errors: reaching back the other way
// would close an import cycle. cmd/server installs the real resolver, which
// keeps this package free of dependencies on the rest of the platform.
type ActorResolver func(ctx context.Context) (userID int64, name, email, organization string, ok bool)

var resolveActor atomic.Pointer[ActorResolver]

// SetActorResolver installs the function that names the actor behind a request.
func SetActorResolver(fn ActorResolver) {
	if fn == nil {
		resolveActor.Store(nil)
		return
	}
	resolveActor.Store(&fn)
}

// withActor attaches who hit the error, when the request got far enough for
// authentication to have run. An error nobody can attribute is much harder to
// act on than one with a name against it.
func withActor(ctx context.Context, e Event) Event {
	fn := resolveActor.Load()
	if fn == nil {
		return e
	}
	userID, name, email, organization, ok := (*fn)(ctx)
	if !ok {
		return e
	}
	if userID > 0 {
		id := userID
		e.UserID = &id
	}
	e.UserName = name
	e.UserEmail = email
	if e.OrganizationName == "" {
		e.OrganizationName = organization
	}
	return e
}

// exceptionClass names the kind of failure, which is what the admin screen
// groups by. Go has no exception classes, so the closest honest equivalent is
// the concrete type of the error.
func exceptionClass(err error) string {
	if err == nil {
		return ""
	}
	return typeName(err)
}

// ReportWithActor captures a pre-built event, attaching the actor from ctx.
// The panic path builds its own event because it has a stack to carry.
func ReportWithActor(ctx context.Context, e Event) {
	if !Installed() {
		return
	}
	e.RequestID = observability.RequestIDFrom(ctx)
	Report(withActor(ctx, e))
}

// PanicClass names the type of a recovered panic value, which is the closest
// Go has to the exception class the admin screen groups by.
func PanicClass(rec any) string {
	if err, ok := rec.(error); ok {
		return typeName(err)
	}
	return typeName(rec)
}

// PanicOrigin picks the first frame in our own code out of a panic stack.
//
// The top frames are always the runtime and this middleware; what an operator
// needs is the first line inside the application, which is where the bug is.
func PanicOrigin(stack string) (string, int) {
	for _, line := range strings.Split(stack, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "/") && !strings.Contains(line, ":\\") {
			continue
		}
		if !strings.Contains(line, "dawa24-store") ||
			strings.Contains(line, "/platform/errtrack/") ||
			strings.Contains(line, "/platform/httpx/") {
			continue
		}
		// "<file>:<line> +0x.."
		frame := strings.Fields(line)[0]
		i := strings.LastIndexByte(frame, ':')
		if i <= 0 {
			continue
		}
		n, err := strconv.Atoi(frame[i+1:])
		if err != nil {
			continue
		}
		return trimPath(frame[:i]), n
	}
	return "", 0
}
