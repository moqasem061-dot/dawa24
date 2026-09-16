package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

func TestPhase2_SSEStreamingDecoding(t *testing.T) {
	// T2.1 & T2.2: multi-frame SSE decoding with reasoning and [DONE]
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		// Frame 1: reasoning
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking step 1\"}}]}\n\n")
		flusher.Flush()
		time.Sleep(10 * time.Millisecond)

		// Frame 2: content delta
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"مرحبا بك \"}}]}\n\n")
		flusher.Flush()
		time.Sleep(10 * time.Millisecond)

		// Frame 3: content delta 2 + usage
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"في دوا 24\"}}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":15,\"total_tokens\":25,\"completion_tokens_details\":{\"reasoning_tokens\":5}}}\n\n")
		flusher.Flush()
		time.Sleep(10 * time.Millisecond)

		// Frame 4: [DONE]
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	client := New(config.Gateway{
		BaseURL:    server.URL,
		VirtualKey: "sk-test",
		Enabled:    true,
	}, slog.Default())

	req := ChatRequest{
		Role: RolePrimary,
		Messages: []ChatMessage{
			{Role: "user", Text: "أهلا"},
		},
	}

	events, err := client.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}

	var gatheredText string
	var gatheredReasoning string
	var doneReceived bool
	var usageFound *Usage

	for ev := range events {
		if ev.Err != nil {
			t.Fatalf("unexpected event error: %v", ev.Err)
		}
		gatheredText += ev.Delta
		gatheredReasoning += ev.Reasoning
		if ev.Done {
			doneReceived = true
		}
		if ev.Usage != nil {
			usageFound = ev.Usage
		}
	}

	if !doneReceived {
		t.Errorf("T2.2 failed: expected Done event")
	}
	if gatheredReasoning != "thinking step 1" {
		t.Errorf("T2.1 failed: expected reasoning 'thinking step 1', got %q", gatheredReasoning)
	}
	if gatheredText != "مرحبا بك في دوا 24" {
		t.Errorf("T2.1 failed: expected full text 'مرحبا بك في دوا 24', got %q", gatheredText)
	}
	if usageFound == nil || usageFound.TotalTokens != 25 || usageFound.ReasoningTokens != 5 {
		t.Errorf("T2.1 failed: usage mismatch: %+v", usageFound)
	}
}

func TestPhase2_CancellationAbortsUpstream(t *testing.T) {
	// T2.3: cancelling context mid-stream aborts upstream body
	bodyClosed := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first token\"}}]}\n\n")
		flusher.Flush()

		// Wait until client context cancels
		<-r.Context().Done()
		close(bodyClosed)
	}))
	defer server.Close()

	client := New(config.Gateway{
		BaseURL:    server.URL,
		VirtualKey: "sk-test",
		Enabled:    true,
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	req := ChatRequest{
		Role: RolePrimary,
		Messages: []ChatMessage{
			{Role: "user", Text: "test cancel"},
		},
	}

	events, err := client.Stream(ctx, req)
	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}

	// Read first event
	ev := <-events
	if ev.Delta != "first token" {
		t.Fatalf("expected first token, got %q", ev.Delta)
	}

	// Cancel context
	cancel()

	// Ensure upstream received abort
	select {
	case <-bodyClosed:
		// Success: upstream request was aborted
	case <-time.After(2 * time.Second):
		t.Errorf("T2.3 failed: upstream did not receive abort within timeout")
	}
}

func TestPhase2_UnauthorizedAndForbiddenNotRetried(t *testing.T) {
	// T2.4: 401 returns ErrUnauthorized immediately without retry
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":{"message":"invalid virtual key","type":"auth_error"}}`)
	}))
	defer server.Close()

	client := New(config.Gateway{
		BaseURL:    server.URL,
		VirtualKey: "sk-bad-key",
		Enabled:    true,
	}, slog.Default())

	req := ChatRequest{
		Role: RolePrimary,
		Messages: []ChatMessage{
			{Role: "user", Text: "hi"},
		},
	}

	_, err := client.Stream(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("T2.4 failed: expected ErrUnauthorized, got %v", err)
	}
	if attempts != 1 {
		t.Errorf("T2.4 failed: expected exactly 1 attempt (no retries), got %d", attempts)
	}
}

func TestPhase2_MultimodalSerialization(t *testing.T) {
	// T2.6: Multimodal parts serialise to OpenAI/Gateway dialect
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := New(config.Gateway{
		BaseURL:    server.URL,
		VirtualKey: "sk-test",
		Enabled:    true,
	}, slog.Default())

	req := ChatRequest{
		Role: RolePrimary,
		Messages: []ChatMessage{
			{
				Role: "user",
				Parts: []ContentPart{
					{Kind: PartText, Text: "Check this photo and audio"},
					{Kind: PartImage, DataURL: "data:image/png;base64,iVBORw0KGgo="},
					{Kind: PartAudio, DataURL: "data:audio/wav;base64,UklGRg==", MIMEType: "audio/wav"},
					{Kind: PartVideo, DataURL: "https://example.com/demo.mp4"},
					{Kind: PartFile, DataURL: "https://example.com/doc.pdf", Filename: "doc.pdf", MIMEType: "application/pdf"},
				},
			},
		},
	}

	events, err := client.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range events {
	}

	var parsed wireChatRequest
	if err := json.Unmarshal(receivedBody, &parsed); err != nil {
		t.Fatalf("failed to decode wire payload: %v", err)
	}

	if len(parsed.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(parsed.Messages))
	}

	partsJSON, _ := json.Marshal(parsed.Messages[0].Content)
	partsStr := string(partsJSON)

	if !strings.Contains(partsStr, `"type":"image_url"`) {
		t.Errorf("T2.6 failed: missing image_url part in %s", partsStr)
	}
	if !strings.Contains(partsStr, `"type":"input_audio"`) {
		t.Errorf("T2.6 failed: missing input_audio part in %s", partsStr)
	}
	if !strings.Contains(partsStr, `"type":"video_url"`) {
		t.Errorf("T2.6 failed: missing video_url part in %s", partsStr)
	}
	if !strings.Contains(partsStr, `"type":"file"`) {
		t.Errorf("T2.6 failed: missing file part in %s", partsStr)
	}
}

func TestPhase2_TranscriptionEndpoint(t *testing.T) {
	// Transcription test: checks multipart field names
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		if r.FormValue("model") != "whisper-large-v3-turbo" {
			t.Errorf("expected model whisper-large-v3-turbo, got %q", r.FormValue("model"))
		}
		if r.FormValue("language") != "ar" {
			t.Errorf("expected language ar, got %q", r.FormValue("language"))
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected file field: %v", err)
		}
		defer file.Close()
		content, _ := io.ReadAll(file)
		if string(content) != "audio-data" {
			t.Errorf("expected audio-data, got %q", string(content))
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"text":"تفريغ صوتي تجريبي"}`)
	}))
	defer server.Close()

	client := New(config.Gateway{
		BaseURL:    server.URL,
		VirtualKey: "sk-test",
		Enabled:    true,
	}, slog.Default())

	text, err := client.Transcribe(context.Background(), TranscribeRequest{
		Audio:    strings.NewReader("audio-data"),
		Filename: "voice.webm",
		MIMEType: "audio/webm",
	})
	if err != nil {
		t.Fatalf("unexpected transcribe error: %v", err)
	}
	if text != "تفريغ صوتي تجريبي" {
		t.Errorf("expected transcript 'تفريغ صوتي تجريبي', got %q", text)
	}
}

func TestPhase2_CapabilitiesFallback(t *testing.T) {
	// T2.7: Capabilities error returns conservative default
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := New(config.Gateway{
		BaseURL:    server.URL,
		VirtualKey: "sk-test",
		Enabled:    true,
	}, slog.Default())

	caps, err := client.Capabilities(context.Background(), RolePrimary)
	if err != nil {
		t.Fatalf("expected nil error on conservative fallback, got %v", err)
	}
	if caps.Vision || caps.Audio || caps.Documents || caps.MaxAttachmentMB != 0 {
		t.Errorf("T2.7 failed: expected conservative default, got %+v", caps)
	}
}

// TestAudioPartIsBareBase64 pins the one part of this dialect that does not
// take a data URI.
//
// input_audio.data must be the base64 payload alone. Sending the whole
// "data:audio/webm;base64,…" string there meant every voice note reached the
// upstream with its own MIME header decoded as audio, and the format field —
// hardcoded to "wav" for anything that was not mp3 — mislabelled the container
// on top of it.
func TestAudioPartIsBareBase64(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mime       string
		dataURL    string
		wantData   string
		wantFormat string
	}{
		{"webm", "audio/webm", "data:audio/webm;base64,GkXfo0==", "GkXfo0==", "webm"},
		{"mp3", "audio/mpeg", "data:audio/mpeg;base64,SUQzBA==", "SUQzBA==", "mp3"},
		{"m4a", "audio/mp4", "data:audio/mp4;base64,AAAAHGZ0", "AAAAHGZ0", "m4a"},
		{"wav", "audio/wav", "data:audio/wav;base64,UklGRg==", "UklGRg==", "wav"},
		{"ogg", "audio/ogg", "data:audio/ogg;base64,T2dnUw==", "T2dnUw==", "ogg"},
		{"mime inferred from data uri", "", "data:audio/webm;base64,GkXfo0==", "GkXfo0==", "webm"},
		{"already bare base64 is untouched", "audio/wav", "UklGRg==", "UklGRg==", "wav"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msgs := buildWireMessages([]ChatMessage{{
				Role:  "user",
				Parts: []ContentPart{{Kind: PartAudio, MIMEType: tc.mime, DataURL: tc.dataURL}},
			}})

			parts, ok := msgs[0].Content.([]wireContentPart)
			if !ok || len(parts) != 1 {
				t.Fatalf("expected one content part, got %#v", msgs[0].Content)
			}
			if parts[0].InputAudio == nil {
				t.Fatal("expected an input_audio part")
			}
			if got := parts[0].InputAudio.Data; got != tc.wantData {
				t.Errorf("data = %q, want %q", got, tc.wantData)
			}
			if got := parts[0].InputAudio.Format; got != tc.wantFormat {
				t.Errorf("format = %q, want %q", got, tc.wantFormat)
			}
		})
	}
}

// TestImageAndFilePartsKeepDataURI is the other half of the rule: image_url,
// video_url and file DO take a data URI, and stripping the prefix there would
// break them.
func TestImageAndFilePartsKeepDataURI(t *testing.T) {
	const img = "data:image/png;base64,iVBORw0KGgo="
	msgs := buildWireMessages([]ChatMessage{{
		Role: "user",
		Parts: []ContentPart{
			{Kind: PartImage, MIMEType: "image/png", DataURL: img},
			{Kind: PartFile, MIMEType: "application/pdf", DataURL: img, Filename: "d.pdf"},
		},
	}})
	parts := msgs[0].Content.([]wireContentPart)
	if parts[0].ImageURL == nil || parts[0].ImageURL.URL != img {
		t.Errorf("image_url lost its data URI: %#v", parts[0].ImageURL)
	}
	if parts[1].File == nil || parts[1].File["url"] != img {
		t.Errorf("file part lost its data URI: %#v", parts[1].File)
	}
}
