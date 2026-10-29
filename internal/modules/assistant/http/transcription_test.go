package http

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
)

type transcriptionProbe struct {
	gateway.Client
	called bool
}

func (g *transcriptionProbe) Transcribe(context.Context, gateway.TranscribeRequest) (string, error) {
	g.called = true
	return "unexpected call", nil
}

func TestTranscribeDoesNotGuessModelAfterDiscoveryFailure(t *testing.T) {
	for _, discoveryErr := range []error{gateway.ErrNoTranscriptionModel, errors.New("catalogue authentication failed")} {
		t.Run(discoveryErr.Error(), func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, err := writer.CreateFormFile("file", "voice.ogg")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write([]byte("OggSvoice")); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			gw := &transcriptionProbe{}
			h := NewHandler(nil, nil, gw, nil, nil)
			h.SetTranscriptionModelResolver(func(context.Context, string) (string, error) {
				return "", discoveryErr
			})
			ctx := authctx.WithActor(context.Background(), authctx.Actor{UserID: 10, OrgID: 5})
			r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/transcribe", &body)
			r.Header.Set("Content-Type", writer.FormDataContentType())
			w := httptest.NewRecorder()
			h.Transcribe(w, r)
			if w.Code != http.StatusServiceUnavailable || gw.called {
				t.Fatalf("status=%d, transcription called=%t; want 503 without transcription", w.Code, gw.called)
			}
		})
	}
}
