package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Regression for the bug that made attachments silently impossible.
//
// The client used to decode /v1/models into a nested "capabilities" object.
// That key exists only in the MuhiyaCode catalog-v2 document; /v1/models
// publishes the fields FLAT (supports_vision, max_attachment_mb, …). The decode
// therefore succeeded, produced an all-false capability set, cached it as a
// SUCCESS for five minutes, and every attachment was refused as
// "no_capable_model" with nothing logged, in every environment, forever.
//
// This test pins the wire shape the Gateway actually serves.
func TestCapabilitiesDecodeFlatWireShape(t *testing.T) {
	// Copied from the Gateway's own response builder (proxy/media.go
	// modelCapabilityFields plus the model row fields around it).
	body := map[string]any{
		"object": "list",
		"data": []map[string]any{
			{
				"id":                  "some-other-model",
				"supports_vision":     false,
				"context_window":      8000,
				"max_attachment_mb":   0,
				"accepted_mime_types": []string{},
			},
			{
				"id":                  "primary-model",
				"object":              "model",
				"owned_by":            "MuhiyaLLM",
				"context_window":      262144,
				"max_output_tokens":   16384,
				"supports_vision":     true,
				"supports_thinking":   true,
				"supports_audio":      false,
				"supports_video":      false,
				"supports_documents":  true,
				"max_attachment_mb":   20,
				"accepted_mime_types": []string{"image/png", "application/pdf"},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	c := &HTTPClient{
		http:     srv.Client(),
		log:      testLogger(),
		breaker:  newBreaker(5, time.Second, time.Second),
		cache:    newSettingsCache(time.Minute),
		capCache: newCapabilityCache(time.Minute),
	}
	settings := Settings{BaseURL: srv.URL, VirtualKey: "sk-virt-test", Enabled: true}

	caps, err := c.fetchModelCapabilities(context.Background(), settings, "primary-model")
	if err != nil {
		t.Fatalf("fetch capabilities: %v", err)
	}

	if !caps.Vision {
		t.Error("Vision is false; the flat supports_vision field was not read")
	}
	if !caps.Documents {
		t.Error("Documents is false; the flat supports_documents field was not read")
	}
	if caps.Audio || caps.Video {
		t.Error("Audio/Video reported true for a model that declares neither")
	}
	if caps.MaxAttachmentMB != 20 {
		t.Errorf("MaxAttachmentMB = %d, want 20", caps.MaxAttachmentMB)
	}
	if caps.ContextWindow != 262144 {
		t.Errorf("ContextWindow = %d, want 262144 (the usage meter reads this)",
			caps.ContextWindow)
	}
	if len(caps.AcceptedMIMETypes) != 2 {
		t.Errorf("AcceptedMIMETypes = %v", caps.AcceptedMIMETypes)
	}
}

// A model the catalogue does not list must yield the conservative default —
// refusing attachments rather than guessing that they will work.
func TestUnknownModelIsConservative(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"other","supports_vision":true}]}`))
	}))
	defer srv.Close()

	c := &HTTPClient{
		http:     srv.Client(),
		log:      testLogger(),
		breaker:  newBreaker(5, time.Second, time.Second),
		cache:    newSettingsCache(time.Minute),
		capCache: newCapabilityCache(time.Minute),
	}

	caps, err := c.fetchModelCapabilities(context.Background(),
		Settings{BaseURL: srv.URL, VirtualKey: "sk-virt-test", Enabled: true}, "missing")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if caps.Vision || caps.Documents || caps.MaxAttachmentMB != 0 {
		t.Fatalf("unknown model reported capabilities: %+v", caps)
	}
}

// Transcription model selection: the Gateway hides these from /v1/models, so
// discovery reads the admin catalogue and must pick an ACTIVE one that accepts
// the upload, preferring the cheaper row.
func TestTranscriptionModelSelection(t *testing.T) {
	catalogue := []GatewayModel{
		{Name: "whisper-1", Status: "inactive", Transcribe: true, PricePerMinute: 1000},
		{Name: "expensive-stt", Status: "active", ModelType: "transcript",
			AcceptedMimeTypes: "audio/webm,audio/mpeg", PricePerMinute: 9000},
		{Name: "cheap-stt", Status: "active", Transcribe: true,
			AcceptedMimeTypes: "audio/webm", PricePerMinute: 3000},
		{Name: "chat-model", Status: "active", ModelType: "llm"},
	}
	cache := NewTranscriptionModelCache(
		func(context.Context) ([]GatewayModel, error) { return catalogue, nil },
		func(context.Context) string { return "" },
	)

	got, err := cache.Resolve(context.Background(), "audio/webm")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "cheap-stt" {
		t.Fatalf("model = %q, want cheap-stt (active, accepts webm, cheapest)", got)
	}

	// A format only the pricier model accepts must still resolve.
	got, err = cache.Resolve(context.Background(), "audio/mpeg")
	if err != nil {
		t.Fatalf("resolve mpeg: %v", err)
	}
	if got != "expensive-stt" {
		t.Fatalf("model = %q, want expensive-stt", got)
	}
}

// A Gateway with no active transcription model must say so plainly, so the UI
// can tell the user to type instead of showing a generic failure.
func TestNoActiveTranscriptionModel(t *testing.T) {
	cache := NewTranscriptionModelCache(
		func(context.Context) ([]GatewayModel, error) {
			// whisper-1 as the Gateway seeds it: present, and inactive.
			return []GatewayModel{{Name: "whisper-1", Status: "inactive", Transcribe: true}}, nil
		},
		func(context.Context) string { return "" },
	)

	if _, err := cache.Resolve(context.Background(), "audio/webm"); err != ErrNoTranscriptionModel {
		t.Fatalf("err = %v, want ErrNoTranscriptionModel", err)
	}
}

// An operator's pin wins over the price heuristic.
func TestPinnedTranscriptionModelWins(t *testing.T) {
	cache := NewTranscriptionModelCache(
		func(context.Context) ([]GatewayModel, error) {
			return []GatewayModel{
				{Name: "cheap-stt", Status: "active", Transcribe: true, PricePerMinute: 1},
				{Name: "chosen-stt", Status: "active", Transcribe: true, PricePerMinute: 100},
			}, nil
		},
		func(context.Context) string { return "chosen-stt" },
	)

	got, err := cache.Resolve(context.Background(), "audio/webm")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "chosen-stt" {
		t.Fatalf("model = %q, want the pinned chosen-stt", got)
	}
}

// testLogger discards output: these tests assert on values, not on log lines.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// The assistant's model choices are measured, not assumed.
//
// Both were verified against the live Gateway before being set here, and both
// replaced a default that was actively broken:
//
//	whisper-1                 404 "not found or inactive"   (voice never worked)
//	whisper-large-v3-turbo    200 with a transcript
//	qwen3.7-flash             finish_reason=length, no tool call, no answer
//	gemma-4-31b-it            finish_reason=tool_calls, correct arguments
//
// This test does not reach the network. It pins the choice so that reverting it
// is a deliberate act with a reason, rather than a merge.
func TestAssistantRoleDefaults(t *testing.T) {
	cases := map[Role]string{
		RolePrimary:    "gemma-4-31b-it",
		RoleAttachment: "gemma-4-31b-it",
		RoleTranscribe: "whisper-large-v3-turbo",
	}
	for role, want := range cases {
		if got := defaultRoleModels[role]; got != want {
			t.Errorf("role %s = %q, want %q", role, got, want)
		}
	}

	// whisper-1 is seeded inactive on the Gateway. Anything that points a role
	// at it has re-broken voice input.
	for role, model := range defaultRoleModels {
		if model == "whisper-1" {
			t.Errorf("role %s points at whisper-1, which the Gateway ships inactive", role)
		}
	}
}
