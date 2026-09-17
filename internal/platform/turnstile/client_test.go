package turnstile_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/turnstile"
)

func TestTurnstileMockMode(t *testing.T) {
	ctx := context.Background()

	// Empty secret key => mock mode
	client := turnstile.New(config.Turnstile{Enabled: true, SecretKey: ""}, nil)
	if client.Enabled() {
		t.Fatal("expected mock client to report disabled")
	}
	if err := client.Verify(ctx, "", "127.0.0.1", "login", "dawa24.com"); err != nil {
		t.Fatalf("expected mock client to succeed on empty token, got: %v", err)
	}

	// Secret key "mock" => mock mode
	clientMock := turnstile.New(config.Turnstile{Enabled: true, SecretKey: "mock"}, nil)
	if err := clientMock.Verify(ctx, "any-token", "127.0.0.1", "login", "dawa24.com"); err != nil {
		t.Fatalf("expected mock client to succeed, got: %v", err)
	}
}

func TestTurnstileVerifyTable(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		expectedAction string
		expectedHost   string
		serverStatus   int
		serverResponse map[string]any
		wantErr        error
	}{
		{
			name:    "empty token rejected without request",
			token:   "",
			wantErr: turnstile.ErrMissingToken,
		},
		{
			name:           "successful verification",
			token:          "valid-token",
			expectedAction: "login",
			expectedHost:   "dawa24.com",
			serverStatus:   http.StatusOK,
			serverResponse: map[string]any{
				"success":      true,
				"action":       "login",
				"hostname":     "dawa24.com",
				"challenge_ts": time.Now().Format(time.RFC3339),
			},
			wantErr: nil,
		},
		{
			name:           "action mismatch rejected",
			token:          "valid-token",
			expectedAction: "register",
			expectedHost:   "dawa24.com",
			serverStatus:   http.StatusOK,
			serverResponse: map[string]any{
				"success":      true,
				"action":       "login",
				"hostname":     "dawa24.com",
				"challenge_ts": time.Now().Format(time.RFC3339),
			},
			wantErr: turnstile.ErrInvalidAction,
		},
		{
			name:           "hostname mismatch rejected",
			token:          "valid-token",
			expectedAction: "login",
			expectedHost:   "dawa24.com",
			serverStatus:   http.StatusOK,
			serverResponse: map[string]any{
				"success":      true,
				"action":       "login",
				"hostname":     "attacker.com",
				"challenge_ts": time.Now().Format(time.RFC3339),
			},
			wantErr: turnstile.ErrInvalidHostname,
		},
		{
			name:           "expired token rejected",
			token:          "valid-token",
			expectedAction: "login",
			expectedHost:   "dawa24.com",
			serverStatus:   http.StatusOK,
			serverResponse: map[string]any{
				"success":      true,
				"action":       "login",
				"hostname":     "dawa24.com",
				"challenge_ts": time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
			},
			wantErr: turnstile.ErrExpiredToken,
		},
		{
			name:           "cloudflare rejected challenge",
			token:          "bad-token",
			expectedAction: "login",
			expectedHost:   "dawa24.com",
			serverStatus:   http.StatusOK,
			serverResponse: map[string]any{
				"success":     false,
				"error-codes": []string{"invalid-input-response"},
			},
			wantErr: turnstile.ErrVerificationFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if err := r.ParseForm(); err != nil {
					t.Errorf("parse form error: %v", err)
				}
				if r.FormValue("secret") != "test-secret" {
					t.Errorf("expected secret 'test-secret', got %s", r.FormValue("secret"))
				}
				w.WriteHeader(tt.serverStatus)
				_ = json.NewEncoder(w).Encode(tt.serverResponse)
			}))
			defer server.Close()

			client := turnstile.NewWithConfig(turnstile.Config{
				SiteKey:    "test-site-key",
				SecretKey:  "test-secret",
				Enabled:    true,
				VerifyURL:  server.URL,
				HTTPClient: server.Client(),
			})

			err := client.Verify(context.Background(), tt.token, "192.168.1.100", tt.expectedAction, tt.expectedHost)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
