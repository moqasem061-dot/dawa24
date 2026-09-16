package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// The wire. Everything in this file is the shape of one HTTP call to the
// Gateway, and it is the only place in the repository that knows that shape.
//
// Two of the fields here exist because of what the Gateway publishes rather than
// what the Store wants. `response_format` carries a JSON schema when the caller
// supplies one, and `reasoning` switches off a reasoning model chain of
// thought — both are optional upstream features that some models honour and
// others ignore, so both are written defensively and neither is required for a
// call to succeed.

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	ResponseFormat any           `json:"response_format,omitempty"`
	Reasoning      *reasoning    `json:"reasoning,omitempty"`
	Stream         bool          `json:"stream"`
}

// reasoning switches a reasoning model's chain of thought off.
//
// Note that "enabled": false is the only setting that actually stops the tokens
// being generated. Measured against this Gateway: "exclude": true still produced
// 192 reasoning tokens and merely hid them from the response, and an "effort"
// setting produced 300 of them and truncated the answer away entirely.
type reasoning struct {
	Enabled bool `json:"enabled"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *HTTPClient) do(ctx context.Context, req Request, b budget) (*Response, error) {
	settings := c.resolve(ctx)
	role := RoleForRequest(req)
	if role != "" && IsRoleDisabled(settings, role) {
		return nil, ErrDisabled
	}
	model := ""
	if role != "" {
		model = ResolveRoleModel(settings, role)
	}
	if model == "" {
		model = modelFor(settings, b)
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 && role != "" {
		if configured := GetRoleMaxTokens(settings, role); configured > 0 {
			maxTokens = configured
		}
	}
	payload := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: req.System},
			{Role: "user", Content: req.Input},
		},
		MaxTokens: maxTokens,
		Stream:    false,
	}
	if req.Schema != nil {
		payload.ResponseFormat = map[string]any{
			"type":        "json_schema",
			"json_schema": req.Schema,
		}
	}
	if !b.think {
		payload.Reasoning = &reasoning{Enabled: false}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gateway: marshal request: %w", err)
	}

	authKey := settings.VirtualKey
	if req.VirtualKey != "" {
		authKey = req.VirtualKey
	}
	if !settings.Enabled || authKey == "" {
		return nil, ErrDisabled
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		settings.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gateway: build request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+authKey)
	httpReq.Header.Set("X-Client-App", settings.ClientApp)
	// Per-tenant attribution: lets the Gateway report and cap AI spend by
	// organisation without the Store having to meter tokens itself.
	httpReq.Header.Set("X-Dawa-Org-ID", strconv.FormatInt(req.OrganizationID, 10))
	httpReq.Header.Set("X-Dawa-User-ID", strconv.FormatInt(req.UserID, 10))
	httpReq.Header.Set("X-Dawa-Capability", string(req.Capability))
	if req.IdempotencyKey != "" {
		httpReq.Header.Set("Idempotency-Key", req.IdempotencyKey)
	}
	if tp := traceparentFrom(ctx); tp != "" {
		httpReq.Header.Set("traceparent", tp)
	}

	res, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, classifyStatus(res.StatusCode, raw)
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("gateway: decode response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("gateway: upstream refused: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("gateway: response contained no choices")
	}

	choice := parsed.Choices[0]
	if strings.TrimSpace(choice.Message.Content) == "" {
		// An empty answer is not an answer, and it must never be reported as a
		// successful call. The commonest cause is a reasoning model spending the
		// whole token budget before writing anything, which arrives here as a
		// "length" finish reason and no content — silent, total, and impossible
		// to diagnose from a caller that only sees an empty string.
		c.log.WarnContext(ctx, "gateway returned an empty answer",
			"capability", req.Capability, "model", parsed.Model,
			"finish_reason", choice.FinishReason,
			"max_tokens", req.MaxTokens,
			"output_tokens", parsed.Usage.CompletionTokens)
		return nil, fmt.Errorf("%w: model returned no content (finish_reason %q)",
			ErrUnavailable, choice.FinishReason)
	}

	return &Response{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		Model:        parsed.Model,
		RequestID:    parsed.ID,
		InputTok:     parsed.Usage.PromptTokens,
		OutputTok:    parsed.Usage.CompletionTokens,
	}, nil
}
