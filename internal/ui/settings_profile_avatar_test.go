package ui_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockIdentityRepoProfile struct {
	identity.Repository
	user *identity.User
}

func (m *mockIdentityRepoProfile) GetUserByID(ctx context.Context, id int64) (*identity.User, error) {
	if m.user != nil && m.user.ID == id {
		return m.user, nil
	}
	return &identity.User{
		ID:        id,
		Email:     "pharmacy@example.com",
		Name:      i18n.New("د. أحمد", "Dr. Ahmed"),
		Role:      "customer",
		Status:    identity.StatusActive,
		AvatarURL: "/uploads/avatars/old.png",
	}, nil
}

func (m *mockIdentityRepoProfile) UpdateUser(ctx context.Context, u *identity.User) error {
	m.user = u
	return nil
}

func TestSettingsProfileAvatarUpload(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := &mockIdentityRepoProfile{}
	idSvc := identity.NewService(repo, nil, logger)

	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, idSvc, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	ui.RegisterUploadRoutes(r)
	r.Post("/settings/profile", handler.SettingsProfileSubmit)

	t.Run("Upload avatar file and update profile", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		_ = writer.WriteField("name", "د. أحمد عصام")
		_ = writer.WriteField("phone", "01094167168")

		part, err := writer.CreateFormFile("avatar_file", "avatar.png")
		if err != nil {
			t.Fatalf("failed to create form file: %v", err)
		}
		_, _ = part.Write([]byte("fake-png-data"))
		_ = writer.Close()

		req := httptest.NewRequest("POST", "/settings/profile", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		actor := authctx.Actor{UserID: 42, Role: "customer"}
		req = req.WithContext(authctx.WithActor(req.Context(), actor))

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}

		if repo.user == nil {
			t.Fatalf("expected repo.user to be updated")
		}
		if repo.user.AvatarURL == "" || repo.user.AvatarURL == "/uploads/avatars/old.png" {
			t.Errorf("expected AvatarURL to be updated to a new upload path, got %q", repo.user.AvatarURL)
		}
	})

	t.Run("Remove avatar updates profile with empty avatar", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("name", "د. أحمد عصام")
		_ = writer.WriteField("remove_avatar", "1")
		_ = writer.Close()

		req := httptest.NewRequest("POST", "/settings/profile", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		actor := authctx.Actor{UserID: 42, Role: "customer"}
		req = req.WithContext(authctx.WithActor(req.Context(), actor))

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}

		if repo.user.AvatarURL != "" {
			t.Errorf("expected AvatarURL to be empty after remove, got %q", repo.user.AvatarURL)
		}
	})
}
