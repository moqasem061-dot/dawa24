package gateway

import (
	"context"
	"errors"
	"testing"
)

func TestTranscriptionPinMustBeEligible(t *testing.T) {
	for _, tc := range []struct {
		name string
		pin  GatewayModel
	}{
		{"chat model", GatewayModel{Name: "chosen", Status: "active", ModelType: "llm"}},
		{"inactive transcription", GatewayModel{Name: "chosen", Status: "inactive", Transcribe: true}},
		{"unsupported voice format", GatewayModel{Name: "chosen", Status: "active", Transcribe: true, AcceptedMimeTypes: "audio/mpeg"}},
		{"unknown model", GatewayModel{Name: "another", Status: "active", ModelType: "llm"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := NewTranscriptionModelCache(func(context.Context) ([]GatewayModel, error) {
				return []GatewayModel{tc.pin, {Name: "voice-stt", Status: "active", Transcribe: true, AcceptedMimeTypes: "audio/ogg"}}, nil
			}, func(context.Context) string { return "chosen" })
			model, err := cache.Resolve(context.Background(), "audio/ogg")
			if err != nil || model != "voice-stt" {
				t.Fatalf("resolve = %q, %v; want voice-stt", model, err)
			}
		})
	}
}

func TestTranscriptionPinRequiresCatalogue(t *testing.T) {
	unavailable := errors.New("catalogue unavailable")
	cache := NewTranscriptionModelCache(func(context.Context) ([]GatewayModel, error) {
		return nil, unavailable
	}, func(context.Context) string { return "unverified-model" })
	if _, err := cache.Resolve(context.Background(), "audio/ogg"); !errors.Is(err, unavailable) {
		t.Fatalf("resolve error = %v; want catalogue failure", err)
	}
}

func TestTranscriptionPinByIDReturnsModelName(t *testing.T) {
	cache := NewTranscriptionModelCache(func(context.Context) ([]GatewayModel, error) {
		return []GatewayModel{{ID: "model-id", Name: "voice-stt", Status: "active", Transcribe: true}}, nil
	}, func(context.Context) string { return "model-id" })
	model, err := cache.Resolve(context.Background(), "audio/ogg")
	if err != nil || model != "voice-stt" {
		t.Fatalf("resolve = %q, %v; want voice-stt", model, err)
	}
}
