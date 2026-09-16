package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Discovering which model can turn speech into text.
//
// /v1/models cannot answer this. The Gateway excludes every transcription
// model from discovery on purpose (a transcribe row short-circuits
// discoverableModelFor), and the one it ships — whisper-1 — is seeded
// `inactive`, so a deployment that hardcodes that name answers 404 and the
// microphone button just fails.
//
// The admin catalogue at GET /api/models lists everything, including the
// transcription rows and their status. That is the only source that can answer
// "what can this Gateway transcribe with today", so it is the one we ask, on a
// fifteen-minute cache. An operator who swaps the model on the Gateway needs no
// Store deployment.

// GatewayModel is one row of the Gateway's admin model catalogue.
type GatewayModel struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	DisplayName       string `json:"display_name"`
	Status            string `json:"status"`
	ModelType         string `json:"model_type"`
	Transcribe        bool   `json:"transcribe"`
	AcceptedMimeTypes string `json:"accepted_mime_types"`
	PricePerMinute    int64  `json:"price_per_minute_nano_usd"`
	ContextWindow     int    `json:"context_window"`
}

// IsTranscription reports whether this row does speech to text.
func (m GatewayModel) IsTranscription() bool {
	return m.Transcribe || strings.EqualFold(m.ModelType, "transcript")
}

// Accepts reports whether the model declares support for a MIME type. An empty
// declaration means the operator did not restrict it, which we read as "any".
func (m GatewayModel) Accepts(mime string) bool {
	list := strings.TrimSpace(m.AcceptedMimeTypes)
	if list == "" {
		return true
	}
	mime = strings.ToLower(strings.TrimSpace(mime))
	if idx := strings.Index(mime, ";"); idx != -1 {
		mime = strings.TrimSpace(mime[:idx])
	}
	if mime == "" {
		return true
	}
	for _, entry := range strings.Split(list, ",") {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if entry == mime || entry == "*/*" {
			return true
		}
		// "audio/*" style wildcards.
		if strings.HasSuffix(entry, "/*") && strings.HasPrefix(mime, strings.TrimSuffix(entry, "*")) {
			return true
		}
	}
	return false
}

// ListModels returns the Gateway's full model catalogue, transcription rows
// included.
func (c *AdminClient) ListModels(ctx context.Context) ([]GatewayModel, error) {
	status, raw, err := c.send(ctx, http.MethodGet, "/api/models", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("gateway admin: list models returned %d: %s", status, truncateBody(raw))
	}

	// The endpoint has been seen returning both a bare array and an object with
	// a "models" or "data" key. Accept all three rather than break on a shape
	// change nobody would notice until the microphone stopped working.
	var direct []GatewayModel
	if err := json.Unmarshal(raw, &direct); err == nil && len(direct) > 0 {
		return direct, nil
	}
	var wrapped struct {
		Models []GatewayModel `json:"models"`
		Data   []GatewayModel `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("gateway admin: decode models: %w", err)
	}
	if len(wrapped.Models) > 0 {
		return wrapped.Models, nil
	}
	return wrapped.Data, nil
}

// TranscriptionModelResolver names the transcription model to use for one
// upload, or returns an error when the Gateway has none active.
type TranscriptionModelResolver func(ctx context.Context, mime string) (string, error)

// ErrNoTranscriptionModel means the Gateway publishes no active speech-to-text
// model. It is a configuration state, not a failure, and callers should say so
// in words the user can act on.
var ErrNoTranscriptionModel = fmt.Errorf("gateway: no active transcription model")

// TranscriptionModelCache resolves and caches the transcription model choice.
//
// Preference order: an operator's explicit pin, then the cheapest active model
// that accepts the upload's MIME type. Cheapest and not "first" because these
// are priced per minute of audio and the difference between rows is real money
// on a platform where people dictate their shortage lists.
type TranscriptionModelCache struct {
	list   func(ctx context.Context) ([]GatewayModel, error)
	pinned func(ctx context.Context) string
	ttl    time.Duration

	mu        sync.Mutex
	models    []GatewayModel
	expiresAt time.Time
}

// NewTranscriptionModelCache builds a resolver over a catalogue lister. pinned
// may be nil.
func NewTranscriptionModelCache(
	list func(ctx context.Context) ([]GatewayModel, error),
	pinned func(ctx context.Context) string,
) *TranscriptionModelCache {
	return &TranscriptionModelCache{list: list, pinned: pinned, ttl: 15 * time.Minute}
}

// Resolve returns the model name to send for an upload of this MIME type.
func (c *TranscriptionModelCache) Resolve(ctx context.Context, mime string) (string, error) {
	if c == nil || c.list == nil {
		return "", ErrNoTranscriptionModel
	}

	models, err := c.catalogue(ctx)
	if err != nil {
		return "", err
	}

	pin := ""
	if c.pinned != nil {
		pin = strings.TrimSpace(c.pinned(ctx))
	}

	var best *GatewayModel
	for i := range models {
		m := &models[i]
		if !m.IsTranscription() || !strings.EqualFold(m.Status, "active") {
			continue
		}
		if pin != "" && (m.Name == pin || m.ID == pin) {
			return m.Name, nil
		}
		if !m.Accepts(mime) {
			continue
		}
		if best == nil || m.PricePerMinute < best.PricePerMinute {
			best = m
		}
	}
	if best == nil {
		return "", ErrNoTranscriptionModel
	}
	return best.Name, nil
}

func (c *TranscriptionModelCache) catalogue(ctx context.Context) ([]GatewayModel, error) {
	c.mu.Lock()
	if c.models != nil && time.Now().Before(c.expiresAt) {
		models := c.models
		c.mu.Unlock()
		return models, nil
	}
	c.mu.Unlock()

	models, err := c.list(ctx)
	if err != nil {
		// Serve a stale catalogue rather than lose the microphone over a blip
		// in the admin API. The model set changes rarely; the API does not.
		c.mu.Lock()
		stale := c.models
		c.mu.Unlock()
		if stale != nil {
			return stale, nil
		}
		return nil, err
	}

	c.mu.Lock()
	c.models = models
	c.expiresAt = time.Now().Add(c.ttl)
	c.mu.Unlock()
	return models, nil
}
