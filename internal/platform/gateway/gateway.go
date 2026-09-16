// Package gateway is the Store's only connection to AI functionality.
//
// Architectural invariant, enforced by a CI grep (see Makefile target
// "check-provider-isolation"): the words openai, anthropic, deepseek, gemini,
// groq and openrouter must not appear anywhere in this repository outside this
// package's documentation. The Store does not know which provider answers a
// request, does not hold provider credentials, and does not choose models. It
// asks for a capability; the MuhiyaLLM Gateway decides everything else.
//
// That boundary is what makes provider changes a Gateway admin action rather
// than a Store deployment.
//
// Every capability has a deterministic fallback (see fallback.go). A pharmacy
// must be able to place an order and a supplier must be able to import a
// catalogue when the Gateway is unreachable. AI is an enhancement here, never a
// dependency of commerce.
package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

// Capability names a business task, not a model. The mapping from capability to
// model alias lives in Gateway configuration.
type Capability string

const (
	CapProductMatch  Capability = "product.match"
	CapColumnDetect  Capability = "import.detect_columns"
	CapProductEnrich Capability = "product.enrich"
	CapOrderOptimize Capability = "order.optimize"
	CapCatalogChat   Capability = "catalog.chat"
	CapSearchExpand  Capability = "search.expand_query"
	// CapMatchAdjudicate resolves a *batch* of ambiguous product rows against a
	// shortlist supplied with the request. It is separate from CapProductMatch
	// because that one answers a single row: sending ten thousand of those is how
	// a weekly budget disappears in one import.
	CapMatchAdjudicate Capability = "matching.adjudicate"
	// CapMatchEnhance resolves every unsettled row of ONE import against a
	// de-duplicated catalogue window, in as few requests as the payload allows.
	//
	// It supersedes CapMatchAdjudicate for smart ordering. The difference is not
	// batch size but what is on the table: adjudication offers each row the five
	// products the scorer that already failed picked for it, while enhancement
	// sends a retrieved window shared across the whole batch and lets the model
	// answer from any of it. That is the difference between a second opinion and
	// a second vote on the same shortlist.
	CapMatchEnhance Capability = "matching.enhance"
	// CapMemoryExtract distils persistent organizational rules and preferences
	// from chat turns in the background, keeping assistant memory up-to-date.
	CapMemoryExtract Capability = "memory.extract"
)

// Tier is the class of model a capability needs, not a model itself.
//
// The Store asks for a tier; which model serves it is Gateway configuration an
// operator sets in إعدادات النظام. That indirection is the whole point of this
// package: swapping the model behind "fast" must be a settings change, never a
// Store deployment.
type Tier string

const (
	TierFast    Tier = "fast"
	TierQuality Tier = "quality"
)

// Default models, used when an operator has not chosen one.
//
// They live here rather than in any caller because this package is the only
// place allowed to know a provider's vocabulary — the boundary a CI grep
// enforces (see the package comment). A deployment whose Gateway publishes
// different names overrides both from the settings screen.
//
// Both tiers point at the same model, and that is not laziness. The matching
// capability is the one that sends large prompts, and what it needs from a model
// is a wide context and a low price — a 262k-token window at $0.08 per million
// input tokens is what lets two hundred products be judged against a couple of
// thousand catalogue rows in a single request, routinely rather than as an
// exception. An operator who wants a stronger model behind the quality tier sets
// it in إعدادات النظام and nothing here changes.
//
// Whatever is chosen, note that most models the Gateway publishes are reasoning
// models. See budget.think: leaving their chain of thought on is not a cost
// question but a correctness one.
const (
	defaultFastModel    = "qwen3.7-flash"
	defaultQualityModel = "qwen3.7-flash"
)

// budget is the per-capability latency ceiling and retry policy.
//
// These are far below the Gateway's own 30-minute upstream timeout, which is
// tuned for long agentic coding turns. A vendor waiting on a column-detection
// call needs an answer or a fallback in seconds.
type budget struct {
	timeout time.Duration
	retries int
	tier    Tier
	// think allows the model to produce a chain of thought before its answer.
	//
	// It is off by default and that default is load-bearing. Several models the
	// Gateway publishes are reasoning models: they emit a `reasoning` field that
	// is billed as output and drawn from the same max_tokens budget as the
	// answer. A batch of three hundred product matches reasoned through
	// individually would exceed any output ceiling, and when it does the model
	// returns an EMPTY answer with finish_reason "length" — a silent, total
	// failure of the stage that looks exactly like a model with nothing to say.
	//
	// Structured extraction against a prompt that already spells out the
	// decision procedure does not need it. A capability that genuinely benefits
	// from open-ended reasoning sets this true and sizes its own budget.
	think bool
}

var budgets = map[Capability]budget{
	CapColumnDetect:  {timeout: 60 * time.Second, retries: 2, tier: TierFast},
	CapProductMatch:  {timeout: 120 * time.Second, retries: 2, tier: TierFast},
	CapProductEnrich: {timeout: 240 * time.Second, retries: 2, tier: TierQuality},
	CapOrderOptimize: {timeout: 15 * time.Second, retries: 1, tier: TierFast},
	// Chat is a conversation with a person, which is the one place where a model
	// thinking out loud is worth what it costs.
	CapCatalogChat:  {timeout: 120 * time.Second, retries: 0, tier: TierQuality, think: true},
	CapSearchExpand: {timeout: 5 * time.Second, retries: 0, tier: TierFast},
	// A batch of 25 rows with five candidates each is a large prompt but a small
	// completion, so the timeout is generous and the retry count low: a retry
	// costs a whole batch again, and the pipeline's bisection handles failure
	// better than a blind repeat would.
	CapMatchAdjudicate: {timeout: 90 * time.Second, retries: 1, tier: TierFast},
	// Enhancement sends the largest prompt this application produces and expects
	// compact JSON. A batch is intentionally single-attempt: retrying a large
	// matching request repeats the full bill and can keep a run waiting for
	// minutes. The pipeline keeps its deterministic result when this attempt
	// fails.
	CapMatchEnhance:  {timeout: 150 * time.Second, retries: 0, tier: TierQuality},
	CapMemoryExtract: {timeout: 30 * time.Second, retries: 1, tier: TierFast},
}

// modelFor resolves a capability's tier to the model name to send.
//
// An unconfigured tier falls back to the default rather than sending an empty
// model, which the Gateway would reject as a bad request — and a 400 here reads
// to a caller as "AI is broken" rather than "nobody chose a model".
func modelFor(s Settings, b budget) string {
	if b.tier == TierQuality && strings.TrimSpace(s.QualityModel) != "" {
		return strings.TrimSpace(s.QualityModel)
	}
	if b.tier == TierFast && strings.TrimSpace(s.FastModel) != "" {
		return strings.TrimSpace(s.FastModel)
	}
	if b.tier == TierQuality {
		return defaultQualityModel
	}
	return defaultFastModel
}

// Request is a capability invocation.
type Request struct {
	Capability     Capability
	Role           Role           // optional specific role override (WO-39)
	System         string         // prompt template, versioned in this repo
	Input          string         // rendered user content
	Schema         map[string]any // optional JSON schema for structured output
	OrganizationID int64          // cost attribution and per-tenant quota
	UserID         int64
	// Feature names the screen or tool that asked, in the tenant's own
	// vocabulary — "الطلب الذكي", "استيراد أصناف المورد". The capability is
	// what the Gateway is asked for; the feature is what the pharmacy
	// recognises on its usage log. A capability alone tells a reader nothing
	// about which of their own actions spent the money.
	Feature        string
	VirtualKey     string // optional tenant virtual key for per-tenant quotas
	IdempotencyKey string // required for anything that mutates domain state
	MaxTokens      int
}

// Response is a capability result.
type Response struct {
	Content string
	// FinishReason is the upstream's own word for why generation stopped —
	// "stop" for a complete answer, "length" for one cut off by the token
	// budget. A caller parsing structured output needs to tell a model that
	// declined from one that was interrupted mid-array.
	FinishReason string
	Model        string // Gateway's resolved model id, for logging only
	RequestID    string // Gateway request_logs.id, stored alongside the domain row
	InputTok     int
	OutputTok    int
	CostNanoUSD  int64
	FromCache    bool
	Fallback     bool // true when this came from the deterministic path
}

// Client is the Store-facing interface. Modules depend on this, never on the
// concrete implementation, so tests inject a stub or a black-hole.
type Client interface {
	Invoke(ctx context.Context, req Request) (*Response, error)
	Stream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
	Transcribe(ctx context.Context, req TranscribeRequest) (string, error)
	Capabilities(ctx context.Context, role Role) (ModelCapabilities, error)
	Health(ctx context.Context) error
	Enabled() bool
}

// HTTPClient talks to the Gateway over its OpenAI-compatible surface.
type HTTPClient struct {
	cfg        config.Gateway
	http       *http.Client
	log        *slog.Logger
	breaker    *breaker
	source     SettingsSource
	cache      *settingsCache
	capCache   *capabilityCache
	sourceMu   sync.Mutex
	lastSource string
}

func New(cfg config.Gateway, log *slog.Logger) *HTTPClient {
	return &HTTPClient{
		cfg: cfg,
		http: &http.Client{
			// No whole-request timeout: the overall deadline is the capability
			// budget, applied to the context in Invoke. What is bounded here is
			// only REACHING the Gateway.
			Transport: &http.Transport{
				DialContext:         (&net.Dialer{Timeout: reachTimeout(cfg.Timeout)}).DialContext,
				TLSHandshakeTimeout: reachTimeout(cfg.Timeout),
				MaxIdleConns:        50,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		log:      log.With("component", "gateway"),
		breaker:  newBreaker(5, 30*time.Second, 60*time.Second),
		cache:    newSettingsCache(30 * time.Second),
		capCache: newCapabilityCache(5 * time.Minute),
	}
}

// reachTimeout bounds getting a connection to the Gateway, and nothing else.
//
// The distinction matters and getting it wrong cost three of four batches on a
// real import. http.Client.Timeout covers the WHOLE request including
// generation, so setting it from the configured sixty seconds silently capped
// every capability budget: match enhancement is allowed three hundred seconds
// and died at sixty with "Client.Timeout exceeded while awaiting headers", which
// reads like an unreachable Gateway rather than a ceiling of our own making.
// Non-streaming completions send no headers until the answer is ready, so any
// header-level deadline is a generation deadline in disguise.
//
// Connecting, on the other hand, should be quick or not at all: an unreachable
// Gateway must not hold an import open for the length of a capability budget.
// So the configured timeout governs the dial and the handshake, the context
// governs generation, and the two no longer fight.
func reachTimeout(configured time.Duration) time.Duration {
	if configured <= 0 || configured > 30*time.Second {
		return 30 * time.Second
	}
	return configured
}

func (c *HTTPClient) Enabled() bool {
	s := c.resolve(context.Background())
	return s.Enabled
}

// Invoke calls the Gateway, honouring the capability's timeout and retry budget.
//
// It returns ErrUnavailable rather than a transport error so that callers have
// exactly one condition to branch on when deciding to use their fallback.
func (c *HTTPClient) Invoke(ctx context.Context, req Request) (*Response, error) {
	settings := c.resolve(ctx)
	authKey := settings.VirtualKey
	if req.VirtualKey != "" {
		authKey = req.VirtualKey
	}
	if !settings.Enabled || authKey == "" {
		return nil, ErrDisabled
	}
	b, ok := budgets[req.Capability]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownCapability, req.Capability)
	}
	if !c.breaker.allow() {
		return nil, ErrCircuitOpen
	}

	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt <= b.retries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter, bounded by the remaining budget.
			delay := backoff(attempt)
			select {
			case <-ctx.Done():
				c.breaker.failure()
				return nil, ErrTimeout
			case <-time.After(delay):
			}
		}

		resp, err := c.do(ctx, req, b)
		if err == nil {
			c.breaker.success()
			return resp, nil
		}
		lastErr = err

		// Only transport and 5xx failures are worth retrying. A 400 will be a
		// 400 again, and a 402 means the budget is genuinely spent.
		if !isRetryable(err) {
			c.breaker.success() // the Gateway answered; it is not unhealthy
			return nil, err
		}
	}

	c.breaker.failure()
	c.log.WarnContext(ctx, "gateway call exhausted retries",
		"capability", req.Capability, "org_id", req.OrganizationID, "error", lastErr)
	return nil, fmt.Errorf("%w: %v", ErrUnavailable, lastErr)
}

// Health probes the Gateway for the readiness endpoint. A failing Gateway does
// not make the Store unhealthy — commerce continues on fallbacks — so callers
// report this as a degraded subsystem, not a failed one.
func (c *HTTPClient) Health(ctx context.Context) error {
	settings := c.resolve(ctx)
	if !settings.Enabled || settings.VirtualKey == "" {
		return ErrDisabled
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, settings.BaseURL+"/health", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+settings.VirtualKey)
	if settings.ClientApp != "" {
		req.Header.Set("X-Client-App", settings.ClientApp)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: health returned %d", ErrUnavailable, res.StatusCode)
	}
	return nil
}
