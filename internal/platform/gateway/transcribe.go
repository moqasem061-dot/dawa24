package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
)

const maxAudioUploadBytes = 10 << 20 // 10 MiB client-side ceiling

// TranscribeRequest is one speech-to-text call.
//
// It carries the tenant's own virtual key and attribution, which the previous
// signature could not. Transcription used to authenticate with the PLATFORM
// key while chat used the tenant's: voice input was therefore billed to the
// operator, invisible on the tenant's usage screen, and exempt from the plan
// window the Gateway does enforce on this route.
type TranscribeRequest struct {
	Audio    io.Reader
	Filename string
	MIMEType string
	// Model names the transcription model to use. Empty falls back to the
	// configured role default. The assistant discovers this at runtime from the
	// Gateway's admin catalogue rather than hardcoding one, because /v1/models
	// deliberately hides transcription models and the shipped default is seeded
	// inactive.
	Model string
	// Language is an ISO code. Empty means Arabic, which is what this platform
	// speaks.
	Language   string
	OrgID      int64
	UserID     int64
	VirtualKey string
	Feature    string
}

// Transcribe sends an audio stream to the Gateway's transcription endpoint and
// returns the text.
func (c *HTTPClient) Transcribe(ctx context.Context, req TranscribeRequest) (string, error) {
	if !c.breaker.allow() {
		return "", ErrCircuitOpen
	}

	settings := c.resolve(ctx)
	authKey := settings.VirtualKey
	if req.VirtualKey != "" {
		authKey = req.VirtualKey
	}
	if !settings.Enabled || authKey == "" {
		return "", ErrDisabled
	}

	filename := req.Filename
	if filename == "" {
		filename = "audio.webm"
	}
	language := req.Language
	if language == "" {
		language = "ar"
	}
	modelName := req.Model
	if modelName == "" {
		modelName = ResolveRoleModel(settings, RoleTranscribe)
	}

	// Limit and buffer client-side to enforce the size ceiling before we spend
	// bandwidth on an upload the Gateway would refuse anyway.
	audioBytes, err := io.ReadAll(io.LimitReader(req.Audio, maxAudioUploadBytes+1))
	if err != nil {
		return "", fmt.Errorf("gateway: read audio buffer: %w", err)
	}
	if len(audioBytes) > maxAudioUploadBytes {
		return "", fmt.Errorf("%w: audio file exceeds 10MB limit", ErrBadRequest)
	}

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("gateway: create multipart file: %w", err)
	}
	if _, err := part.Write(audioBytes); err != nil {
		return "", fmt.Errorf("gateway: write audio part: %w", err)
	}
	for field, value := range map[string]string{
		"model":           modelName,
		"language":        language,
		"response_format": "verbose_json",
	} {
		if err := writer.WriteField(field, value); err != nil {
			return "", fmt.Errorf("gateway: write %s field: %w", field, err)
		}
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("gateway: close multipart writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		settings.BaseURL+"/v1/audio/transcriptions", &requestBody)
	if err != nil {
		return "", fmt.Errorf("gateway: build transcription request: %w", err)
	}

	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+authKey)
	httpReq.Header.Set("X-Client-App", settings.ClientApp)
	if req.OrgID > 0 {
		httpReq.Header.Set("X-Dawa-Org-ID", strconv.FormatInt(req.OrgID, 10))
	}
	if req.UserID > 0 {
		httpReq.Header.Set("X-Dawa-User-ID", strconv.FormatInt(req.UserID, 10))
	}
	if tp := traceparentFrom(ctx); tp != "" {
		httpReq.Header.Set("traceparent", tp)
	}

	res, err := c.http.Do(httpReq)
	if err != nil {
		c.breaker.failure()
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode != http.StatusOK {
		classified := classifyStatus(res.StatusCode, raw)
		if !isRetryable(classified) {
			c.breaker.success()
		} else {
			c.breaker.failure()
		}
		return "", classified
	}

	var parsed struct {
		Text  string `json:"text"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("gateway: decode transcription: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("gateway: transcription error: %s", parsed.Error.Message)
	}

	c.breaker.success()
	return parsed.Text, nil
}
