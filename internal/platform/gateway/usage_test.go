package gateway

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// captureRecorder collects what the decorator wrote.
type captureRecorder struct {
	mu     sync.Mutex
	events []UsageEvent
	// panicOnRecord proves that a broken ledger cannot break an AI call.
	panicOnRecord bool
}

func (r *captureRecorder) RecordAIUsage(_ context.Context, e UsageEvent) {
	if r.panicOnRecord {
		panic("ledger exploded")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *captureRecorder) all() []UsageEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]UsageEvent{}, r.events...)
}

// stubClient answers Invoke and Stream with whatever a test sets.
type stubClient struct {
	resp      *Response
	err       error
	streamErr error
	events    []StreamEvent
	// takes makes a call measurably slow. Windows' monotonic clock has about
	// half a millisecond of resolution, so an instant stub returns a genuine
	// zero and a duration assertion against it tests the clock, not the code.
	takes time.Duration
}

func (s *stubClient) Invoke(context.Context, Request) (*Response, error) {
	if s.takes > 0 {
		time.Sleep(s.takes)
	}
	return s.resp, s.err
}
func (s *stubClient) Stream(context.Context, ChatRequest) (<-chan StreamEvent, error) {
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	ch := make(chan StreamEvent, len(s.events))
	for _, e := range s.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}
func (*stubClient) Transcribe(context.Context, TranscribeRequest) (string, error) {
	return "", nil
}
func (*stubClient) Capabilities(context.Context, Role) (ModelCapabilities, error) {
	return ConservativeDefaultCapabilities(), nil
}
func (*stubClient) Health(context.Context) error { return nil }
func (*stubClient) Enabled() bool                { return true }

func TestInvokeRecordsWhatTheCallActuallyCost(t *testing.T) {
	rec := &captureRecorder{}
	client := WithUsageRecorder(&stubClient{resp: &Response{
		Content:      "{}",
		Model:        "some-model",
		RequestID:    "req-1",
		InputTok:     1200,
		OutputTok:    80,
		CostNanoUSD:  96000,
		FinishReason: "stop",
	}, takes: 5 * time.Millisecond}, rec)

	if _, err := client.Invoke(context.Background(), Request{
		Capability:     CapMatchEnhance,
		OrganizationID: 51,
		UserID:         7,
		Feature:        "variant_match",
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	events := rec.all()
	if len(events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(events))
	}
	e := events[0]
	if e.OrganizationID != 51 || e.UserID != 7 {
		t.Errorf("attribution = org %d user %d, want org 51 user 7", e.OrganizationID, e.UserID)
	}
	if e.Feature != "variant_match" {
		t.Errorf("feature = %q; without it a tenant cannot tell which of their own tools spent the money", e.Feature)
	}
	if e.Model != "some-model" || e.RequestID != "req-1" {
		t.Errorf("model/request id = %q/%q, want the values the Gateway returned", e.Model, e.RequestID)
	}
	if e.InputTokens != 1200 || e.OutputTokens != 80 || e.CostNanoUSD != 96000 {
		t.Errorf("tokens/cost = %d/%d/%d, want 1200/80/96000", e.InputTokens, e.OutputTokens, e.CostNanoUSD)
	}
	if !e.CostKnown {
		t.Error("CostKnown = false for a priced call")
	}
	if e.Status != "success" {
		t.Errorf("status = %q, want success", e.Status)
	}
	if e.Duration <= 0 {
		t.Error("duration not measured")
	}
}

func TestInvokeRecordsFailuresWithTheReasonAnOperatorNeeds(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{ErrDisabled, "disabled"},
		{ErrTimeout, "timeout"},
		{ErrQuotaExceeded, "quota_exceeded"},
		{ErrUnauthorized, "unauthorized"},
		{ErrCircuitOpen, "circuit_open"},
		{errors.New("something else"), "failed"},
	}
	for _, tc := range cases {
		rec := &captureRecorder{}
		client := WithUsageRecorder(&stubClient{err: tc.err}, rec)
		_, _ = client.Invoke(context.Background(), Request{
			Capability: CapProductMatch, OrganizationID: 51,
		})
		events := rec.all()
		if len(events) != 1 {
			t.Fatalf("%v: recorded %d events, want 1", tc.err, len(events))
		}
		if events[0].Status != tc.want {
			t.Errorf("%v: status = %q, want %q", tc.err, events[0].Status, tc.want)
		}
	}
}

func TestUnpricedCallIsNotRecordedAsFree(t *testing.T) {
	// The distinction the previous screens erased: a Gateway that publishes no
	// price is not a Gateway that charged nothing, and summing the two together
	// understates a tenant's bill.
	rec := &captureRecorder{}
	client := WithUsageRecorder(&stubClient{resp: &Response{CostNanoUSD: 0}}, rec)
	_, _ = client.Invoke(context.Background(), Request{Capability: CapProductMatch, OrganizationID: 51})

	if rec.all()[0].CostKnown {
		t.Error("CostKnown = true for a call the Gateway published no price for")
	}

	// A cached answer, by contrast, is genuinely free and that is a known price.
	rec = &captureRecorder{}
	client = WithUsageRecorder(&stubClient{resp: &Response{CostNanoUSD: 0, FromCache: true}}, rec)
	_, _ = client.Invoke(context.Background(), Request{Capability: CapProductMatch, OrganizationID: 51})

	if !rec.all()[0].CostKnown {
		t.Error("CostKnown = false for a cached answer, which is free at a known price")
	}
}

func TestPlatformCallsAreNotAttributedToAnyTenant(t *testing.T) {
	// Work done on nobody's behalf — the admin panel's own assistant, a
	// maintenance job — has no منشأة to bill. Recording it against organisation
	// zero would put rows in a ledger no screen can ever show.
	rec := &captureRecorder{}
	client := WithUsageRecorder(&stubClient{resp: &Response{}}, rec)
	_, _ = client.Invoke(context.Background(), Request{Capability: CapProductMatch})

	if n := len(rec.all()); n != 0 {
		t.Errorf("recorded %d events for a call with no organisation, want 0", n)
	}
}

func TestStreamRecordsTheTurnOnceItEnds(t *testing.T) {
	rec := &captureRecorder{}
	client := WithUsageRecorder(&stubClient{events: []StreamEvent{
		{Delta: "مرحبا"},
		{Delta: " بك"},
		{Done: true, Usage: &Usage{PromptTokens: 300, CompletionTokens: 45}},
	}}, rec)

	events, err := client.Stream(context.Background(), ChatRequest{
		OrgID: 50, UserID: 3, Feature: "assistant",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var relayed int
	for range events {
		relayed++
	}
	if relayed != 3 {
		t.Errorf("relayed %d events, want 3; the decorator must not swallow chunks", relayed)
	}

	// The ledger write happens in the relay goroutine after the channel closes.
	deadline := time.Now().Add(2 * time.Second)
	for len(rec.all()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	got := rec.all()
	if len(got) != 1 {
		t.Fatalf("recorded %d events, want 1", len(got))
	}
	if got[0].InputTokens != 300 || got[0].OutputTokens != 45 {
		t.Errorf("tokens = %d/%d, want 300/45 from the final usage frame",
			got[0].InputTokens, got[0].OutputTokens)
	}
	if got[0].Feature != "assistant" {
		t.Errorf("feature = %q, want assistant", got[0].Feature)
	}
}

func TestABrokenLedgerCannotBreakAnAICall(t *testing.T) {
	// Bookkeeping runs after the model has answered and the tenant has already
	// been billed. It must never be able to turn a successful completion into a
	// failed request.
	rec := &captureRecorder{panicOnRecord: true}
	client := WithUsageRecorder(&stubClient{resp: &Response{Content: "ok"}}, rec)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a failing ledger write propagated to the caller: %v", r)
		}
	}()

	resp, err := client.Invoke(context.Background(), Request{
		Capability: CapProductMatch, OrganizationID: 51,
	})
	if err != nil || resp == nil {
		t.Fatalf("Invoke returned %v / %v", resp, err)
	}
}

func TestWithoutARecorderTheClientIsUnchanged(t *testing.T) {
	inner := &stubClient{resp: &Response{Content: "x"}}
	if got := WithUsageRecorder(inner, nil); got != Client(inner) {
		t.Error("wrapping with a nil recorder returned a different client")
	}
	if got := WithUsageRecorder(nil, &captureRecorder{}); got != nil {
		t.Error("wrapping a nil client produced something non-nil")
	}
}
