package gateway

import (
	"context"
	"errors"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Writing down what every AI call cost, as it happens.
//
// The Gateway keeps its own request log, and for a while that was the only
// record: the usage cards and the AI logs screen called api.muhiya.com live on
// every page render and displayed whatever came back — capped at a hundred
// rows, with no history beyond the Gateway's retention, and blank when the
// Gateway was unreachable. Everything the screens could not get, the handlers
// invented.
//
// Nothing here changes what the Gateway bills. It records what it billed,
// locally, attributed to the منشأة that spent it and to the feature that asked.
// Every field was already on Response at the moment a call returned and was
// simply dropped on the floor.
//
// The recording is deliberately a decorator rather than a branch inside
// HTTPClient. A ledger that can fail must not be able to fail a completion, and
// keeping it outside the transport makes that structural instead of a rule
// somebody has to remember.

// UsageEvent is one AI call as the Store saw it.
type UsageEvent struct {
	OrganizationID int64
	UserID         int64
	Capability     string
	Feature        string

	Model string
	// RequestID is the Gateway's own request_logs id, so a disputed charge can
	// be traced across both systems.
	RequestID string

	InputTokens  int
	OutputTokens int

	// CostNanoUSD is an integer on purpose. A fraction of a cent per request,
	// over a million requests, is a real number of pounds; a float loses it.
	CostNanoUSD int64
	// CostKnown separates a free request from one the Gateway priced at
	// nothing it published. Summing an unknown as zero understates the bill.
	CostKnown bool

	Duration time.Duration

	// Status is the Store's verdict: success, failed, disabled, timeout.
	Status string
	// FinishReason is the upstream's word for why generation stopped. "length"
	// means the answer was truncated — a silent correctness failure that a
	// caller parsing structured output has to be able to find afterwards.
	FinishReason string
	ErrorMessage string

	FromCache bool
	// Fallback marks a call that never reached a model. Counting those as usage
	// would overstate what a feature costs and hide how often AI was down.
	Fallback bool

	At time.Time
}

// UsageRecorder persists usage events.
//
// It returns nothing. A caller has no useful response to a ledger write
// failing — the AI work is already done and paid for — and giving it one would
// invite somebody to fail the request over bookkeeping.
type UsageRecorder interface {
	RecordAIUsage(ctx context.Context, event UsageEvent)
}

// recordingClient is a Client that writes a ledger entry for every call.
type recordingClient struct {
	inner    Client
	recorder UsageRecorder
}

// WithUsageRecorder wraps a Client so every capability invocation and every
// chat stream is written to the usage ledger.
//
// Returns the client unchanged when there is no recorder, so a deployment
// without a database for this — or the black-hole test suite — behaves exactly
// as before.
func WithUsageRecorder(inner Client, recorder UsageRecorder) Client {
	if inner == nil || recorder == nil {
		return inner
	}
	return &recordingClient{inner: inner, recorder: recorder}
}

func resolveActor(ctx context.Context, orgID, userID int64) (int64, int64) {
	if orgID > 0 && userID > 0 {
		return orgID, userID
	}
	if actor, ok := authctx.From(ctx); ok {
		if orgID <= 0 {
			orgID = actor.OrgID
			if orgID <= 0 {
				orgID = actor.OrganizationID
			}
		}
		if userID <= 0 {
			userID = actor.UserID
		}
	}
	if orgID <= 0 {
		if tid, ok := database.TenantFrom(ctx); ok && tid > 0 {
			orgID = tid
		}
	}
	return orgID, userID
}

func (c *recordingClient) Invoke(ctx context.Context, req Request) (*Response, error) {
	started := time.Now()
	resp, err := c.inner.Invoke(ctx, req)

	orgID, userID := resolveActor(ctx, req.OrganizationID, req.UserID)
	event := UsageEvent{
		OrganizationID: orgID,
		UserID:         userID,
		Capability:     string(req.Capability),
		Feature:        req.Feature,
		Duration:       time.Since(started),
		At:             started,
	}

	switch {
	case err != nil:
		event.Status = statusFor(err)
		event.ErrorMessage = err.Error()
	case resp != nil:
		event.Status = "success"
		event.Model = resp.Model
		event.RequestID = resp.RequestID
		event.InputTokens = resp.InputTok
		event.OutputTokens = resp.OutputTok
		event.CostNanoUSD = resp.CostNanoUSD
		// A cached answer is genuinely free, and that is a known price. An
		// uncached answer the Gateway priced at zero is a missing price, not a
		// gift.
		event.CostKnown = resp.CostNanoUSD > 0 || resp.FromCache
		event.FinishReason = resp.FinishReason
		event.FromCache = resp.FromCache
		event.Fallback = resp.Fallback
	default:
		event.Status = "failed"
	}

	c.record(ctx, event)
	return resp, err
}

func (c *recordingClient) Stream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	started := time.Now()
	upstream, err := c.inner.Stream(ctx, req)
	orgID, userID := resolveActor(ctx, req.OrgID, req.UserID)
	if err != nil {
		c.record(ctx, UsageEvent{
			OrganizationID: orgID,
			UserID:         userID,
			Capability:     string(CapCatalogChat),
			Feature:        req.Feature,
			Duration:       time.Since(started),
			At:             started,
			Status:         statusFor(err),
			ErrorMessage:   err.Error(),
		})
		return nil, err
	}

	// A stream's token counts arrive in its final event, so the ledger entry
	// cannot be written until the conversation turn is over. Relaying through
	// our own channel is what lets that happen without the caller doing
	// anything differently — and without leaking a goroutine when the caller
	// stops reading, because the relay closes when upstream does.
	out := make(chan StreamEvent, 8)
	go func() {
		defer close(out)
		defer func() {
			_ = recover()
		}()
		event := UsageEvent{
			OrganizationID: orgID,
			UserID:         userID,
			Capability:     string(CapCatalogChat),
			Feature:        req.Feature,
			At:             started,
			Status:         "success",
		}
		for ev := range upstream {
			if ev.Usage != nil {
				event.InputTokens = ev.Usage.PromptTokens
				event.OutputTokens = ev.Usage.CompletionTokens
			}
			if ev.Err != nil {
				event.Status = statusFor(ev.Err)
				event.ErrorMessage = ev.Err.Error()
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				// The caller has gone. Record what the turn cost anyway: the
				// tokens were spent whether or not anyone read the answer.
				event.Status = "abandoned"
				event.Duration = time.Since(started)
				c.record(context.WithoutCancel(ctx), event)
				return
			}
		}
		event.Duration = time.Since(started)
		c.record(context.WithoutCancel(ctx), event)
	}()
	return out, nil
}

// Transcribe records the call. Speech-to-text is billed per minute of audio by
// the Gateway, and the tenant's usage screen showed none of it because this
// decorator used to pass the call straight through.
func (c *recordingClient) Transcribe(ctx context.Context, req TranscribeRequest) (string, error) {
	started := time.Now()
	text, err := c.inner.Transcribe(ctx, req)
	orgID, userID := resolveActor(ctx, req.OrgID, req.UserID)
	event := UsageEvent{
		OrganizationID: orgID,
		UserID:         userID,
		Capability:     string(RoleTranscribe),
		Feature:        req.Feature,
		Model:          req.Model,
		Duration:       time.Since(started),
		At:             started,
		Status:         "success",
	}
	if err != nil {
		event.Status = statusFor(err)
		event.ErrorMessage = err.Error()
	}
	c.record(ctx, event)
	return text, err
}

func (c *recordingClient) Capabilities(ctx context.Context, role Role) (ModelCapabilities, error) {
	return c.inner.Capabilities(ctx, role)
}

func (c *recordingClient) Health(ctx context.Context) error { return c.inner.Health(ctx) }
func (c *recordingClient) Enabled() bool                    { return c.inner.Enabled() }

// record writes one entry, and never lets doing so affect the caller.
func (c *recordingClient) record(ctx context.Context, event UsageEvent) {
	// A call made with no organisation is platform work — the admin panel's own
	// assistant, a maintenance job. It has no tenant to attribute to and the
	// ledger is per-tenant, so it is not recorded here rather than being
	// recorded against organisation zero, which no screen could ever show.
	if event.OrganizationID <= 0 {
		return
	}
	if event.At.IsZero() {
		event.At = time.Now()
	}

	// A ledger that cannot record must not be able to fail the call it was
	// recording. The AI work is done and the tenant is already billed; turning
	// a successful completion into a panicked request over bookkeeping would be
	// the worst possible trade. The package comment claims this guarantee, so
	// it is enforced here rather than left to every recorder implementation to
	// honour.
	defer func() {
		if r := recover(); r != nil {
			// Nothing is logged from here: the recorder owns the logger, and a
			// recorder broken enough to panic is not one to call again on the
			// way out.
			_ = r
		}
	}()
	c.recorder.RecordAIUsage(ctx, event)
}

// statusFor maps a Gateway error onto the ledger's vocabulary.
//
// The distinctions are the ones an operator reading the log needs: a tenant
// whose calls all say "disabled" has a configuration problem, one seeing
// "circuit_open" has an unhealthy Gateway, and one seeing "timeout" has a
// budget too small for the work being asked of it.
func statusFor(err error) string {
	switch {
	case errors.Is(err, ErrDisabled):
		return "disabled"
	case errors.Is(err, ErrTimeout):
		return "timeout"
	case errors.Is(err, ErrCircuitOpen):
		return "circuit_open"
	case errors.Is(err, ErrQuotaExceeded):
		return "quota_exceeded"
	case errors.Is(err, ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, ErrUnavailable):
		return "unavailable"
	default:
		return "failed"
	}
}

var _ Client = (*recordingClient)(nil)
