package gateway

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

// Tools reach the wire, and tool-call fragments reassemble.
//
// This replaces T2.8, which asserted that a non-empty Tools slice was REFUSED.
// That invariant was correct while the assistant had no tools; it is now the
// opposite of what the product does, and the Gateway forwards an OpenAI body
// verbatim (proxy/handler.go: "bodyBytes directly preserves every field the
// client sent") so nothing upstream stands in the way.
//
// What matters instead is the two things that were never true before: the tool
// definitions must appear in the request, and the streamed call fragments must
// be reassembled in index order rather than arrival order.
func TestToolsAreSentAndToolCallsReassemble(t *testing.T) {
	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Fragments arrive out of order and split mid-argument, which is what
		// every provider actually does.
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_b","function":{"name":"spend_summary","arguments":"{\"from\":"}}]}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"orders_list","arguments":"{}"}}]}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"\"2026-01-01\"}"}}]}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := New(config.Gateway{BaseURL: server.URL, VirtualKey: "sk-test", Enabled: true},
		slog.Default())

	events, err := client.Stream(context.Background(), ChatRequest{
		Role:     RolePrimary,
		Messages: []ChatMessage{{Role: "user", Text: "كم أنفقت؟"}},
		Tools: []ToolSpec{{
			Name:        "spend_summary",
			Description: "إجمالي المشتريات",
			Parameters:  map[string]any{"type": "object", "additionalProperties": false},
		}},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	var calls []ToolCall
	var finish string
	for ev := range events {
		if ev.Err != nil {
			t.Fatalf("stream event error: %v", ev.Err)
		}
		if ev.Done {
			calls = ev.ToolCalls
			finish = ev.FinishReason
		}
	}

	body := string(received)
	if !strings.Contains(body, `"tools"`) || !strings.Contains(body, "spend_summary") {
		t.Fatalf("tool definitions did not reach the wire: %s", body)
	}
	if !strings.Contains(body, `"tool_choice":"auto"`) {
		t.Fatalf("tool_choice missing: %s", body)
	}

	if finish != "tool_calls" {
		t.Errorf("finish reason = %q, want tool_calls", finish)
	}
	if len(calls) != 2 {
		t.Fatalf("reassembled %d calls, want 2: %+v", len(calls), calls)
	}
	// Index order, not arrival order.
	if calls[0].Name != "orders_list" || calls[0].ID != "call_a" {
		t.Errorf("first call = %+v, want orders_list/call_a", calls[0])
	}
	if calls[1].Name != "spend_summary" {
		t.Errorf("second call = %+v, want spend_summary", calls[1])
	}
	if calls[1].Arguments != `{"from":"2026-01-01"}` {
		t.Errorf("arguments = %q, want the two fragments joined", calls[1].Arguments)
	}
}

// A turn with no tools must not send an empty tools array: some providers
// treat its presence as "you may call something" and behave differently.
func TestNoToolsMeansNoToolsField(t *testing.T) {
	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := New(config.Gateway{BaseURL: server.URL, VirtualKey: "sk-test", Enabled: true},
		slog.Default())
	events, err := client.Stream(context.Background(), ChatRequest{
		Role:     RolePrimary,
		Messages: []ChatMessage{{Role: "user", Text: "مرحبا"}},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	for range events {
	}

	if strings.Contains(string(received), `"tools"`) {
		t.Fatalf("tools field present with no tools: %s", received)
	}
}
