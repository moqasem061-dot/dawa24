package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Stream opens a streaming completion to the Gateway. The returned channel
// yields stream events as SSE frames arrive. The caller must drain the channel
// or cancel ctx to abort upstream.
func (c *HTTPClient) Stream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	settings := c.resolve(ctx)
	authKey := settings.VirtualKey
	if req.VirtualKey != "" {
		authKey = req.VirtualKey
	}
	if !settings.Enabled || authKey == "" {
		return nil, ErrDisabled
	}
	if !c.breaker.allow() {
		return nil, ErrCircuitOpen
	}

	modelName := ResolveRoleModel(settings, req.Role)
	wireMsgs := buildWireMessages(req.Messages)

	payload := wireChatRequest{
		Model:       modelName,
		Messages:    wireMsgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      true,
		Tools:       buildWireTools(req.Tools),
	}
	if len(payload.Tools) > 0 {
		// "auto" and not "required": most turns are a plain answer, and forcing
		// a call on a greeting spends a round trip to be told nothing.
		payload.ToolChoice = "auto"
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gateway: marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		settings.BaseURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("gateway: build stream request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+authKey)
	httpReq.Header.Set("X-Client-App", settings.ClientApp)
	httpReq.Header.Set("X-Dawa-Org-ID", strconv.FormatInt(req.OrgID, 10))
	httpReq.Header.Set("X-Dawa-User-ID", strconv.FormatInt(req.UserID, 10))
	httpReq.Header.Set("X-Dawa-Role", string(req.Role))
	if tp := traceparentFrom(ctx); tp != "" {
		httpReq.Header.Set("traceparent", tp)
	}

	res, err := c.http.Do(httpReq)
	if err != nil {
		c.breaker.failure()
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		classified := classifyStatus(res.StatusCode, raw)
		if !isRetryable(classified) {
			c.breaker.success()
		} else {
			c.breaker.failure()
		}
		return nil, classified
	}

	events := make(chan StreamEvent, 64)
	go c.consumeSSE(ctx, res.Body, events)
	return events, nil
}

func (c *HTTPClient) consumeSSE(ctx context.Context, body io.ReadCloser, events chan<- StreamEvent) {
	defer body.Close()
	defer close(events)

	// Tool calls arrive as fragments spread across chunks: the first carries an
	// id and a name, the rest append characters to the arguments string. They
	// are keyed by the index the upstream assigns, never by arrival order.
	acc := newToolCallAccumulator()
	finishReason := ""

	emit := func(ev StreamEvent) bool {
		select {
		case events <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}

	reader := bufio.NewReader(body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(ctx.Err(), context.Canceled) {
				return
			}
			c.breaker.failure()
			emit(StreamEvent{Err: fmt.Errorf("%w: %v", ErrUnavailable, err)})
			return
		}

		line = strings.TrimRight(line, "\r\n")
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			c.breaker.success()
			emit(StreamEvent{Done: true, FinishReason: finishReason, ToolCalls: acc.finish()})
			return
		}

		var chunk wireStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Error != nil {
			c.breaker.failure()
			emit(StreamEvent{Err: fmt.Errorf("gateway: %s", chunk.Error.Message)})
			return
		}

		var delta, reasoning string
		if len(chunk.Choices) > 0 {
			ch := chunk.Choices[0]
			delta = ch.Delta.Content
			reasoning = ch.Delta.ReasoningContent
			if reasoning == "" {
				reasoning = ch.Delta.Thinking
			}
			if ch.FinishReason != "" {
				finishReason = ch.FinishReason
			}
			acc.add(ch.Delta.ToolCalls)
		}

		var usage *Usage
		if chunk.Usage != nil {
			usage = &Usage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				ReasoningTokens:  chunk.Usage.CompletionTokensDetails.ReasoningTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
		}

		if delta != "" || reasoning != "" || usage != nil {
			if !emit(StreamEvent{Delta: delta, Reasoning: reasoning, Usage: usage}) {
				return
			}
		}
	}
}

// wireStreamChunk is one decoded SSE frame of a streaming completion.
type wireStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string             `json:"content"`
			ReasoningContent string             `json:"reasoning_content"`
			Thinking         string             `json:"thinking"`
			ToolCalls        []wireToolCallItem `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens            int `json:"prompt_tokens"`
		CompletionTokens        int `json:"completion_tokens"`
		TotalTokens             int `json:"total_tokens"`
		CompletionTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type wireToolCallItem struct {
	Index    *int   `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// toolCallAccumulator reassembles streamed tool-call fragments.
//
// Order of arrival is not order of index, and a provider may repeat an index
// with only an arguments fragment, so the slots are a map and the emitted
// order is by index.
type toolCallAccumulator struct {
	slots map[int]*ToolCall
	order []int
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{slots: make(map[int]*ToolCall)}
}

func (a *toolCallAccumulator) add(items []wireToolCallItem) {
	for _, item := range items {
		idx := 0
		if item.Index != nil {
			idx = *item.Index
		}
		slot, ok := a.slots[idx]
		if !ok {
			slot = &ToolCall{}
			a.slots[idx] = slot
			a.order = append(a.order, idx)
		}
		if item.ID != "" {
			slot.ID = item.ID
		}
		if item.Function.Name != "" {
			slot.Name = item.Function.Name
		}
		slot.Arguments += item.Function.Arguments
	}
}

// finish returns the completed calls, dropping any slot that never received a
// name — a fragment with no function is not a call anyone can dispatch.
func (a *toolCallAccumulator) finish() []ToolCall {
	if len(a.order) == 0 {
		return nil
	}
	sorted := append([]int(nil), a.order...)
	sort.Ints(sorted)
	out := make([]ToolCall, 0, len(sorted))
	for _, idx := range sorted {
		slot := a.slots[idx]
		if slot == nil || slot.Name == "" {
			continue
		}
		out = append(out, *slot)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
