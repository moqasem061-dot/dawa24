package ui

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
)

type emailLookupRepository struct {
	identity.Repository
	user *identity.User
	err  error
}

func (r *emailLookupRepository) GetUserByEmail(context.Context, string) (*identity.User, error) {
	return r.user, r.err
}

func TestSendEmailOTPRejectsAlreadyRegisteredAddress(t *testing.T) {
	handler := emailOTPTestHandler(&emailLookupRepository{user: &identity.User{Email: "registered@example.com"}})
	response := sendEmailOTPRequest(handler)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusConflict, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"already_registered"`) {
		t.Fatalf("response = %s, want already_registered error", response.Body.String())
	}
}

func TestSendEmailOTPStopsWhenRegistrationLookupFails(t *testing.T) {
	handler := emailOTPTestHandler(&emailLookupRepository{err: errors.New("database unavailable")})
	response := sendEmailOTPRequest(handler)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"service_unavailable"`) {
		t.Fatalf("response = %s, want service_unavailable error", response.Body.String())
	}
}

func emailOTPTestHandler(repo *emailLookupRepository) *UIHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &UIHandler{idSvc: identity.NewService(repo, nil, logger), log: logger}
}

func sendEmailOTPRequest(handler *UIHandler) *httptest.ResponseRecorder {
	form := url.Values{"email": {"registered@example.com"}}
	request := httptest.NewRequestWithContext(
		context.Background(), http.MethodPost, "/auth/email/send-otp", strings.NewReader(form.Encode()),
	)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.SendEmailOTP(response, request)
	return response
}
