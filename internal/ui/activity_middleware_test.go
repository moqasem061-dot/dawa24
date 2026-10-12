package ui

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

type operationAuditRepo struct {
	identity.Repository
	entries []database.AuditEntry
}

func (r *operationAuditRepo) RecordAudit(_ context.Context, entry database.AuditEntry) error {
	r.entries = append(r.entries, entry)
	return nil
}

func TestActivityCapturesAttributionAndRedactsSecrets(t *testing.T) {
	repo := &operationAuditRepo{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &UIHandler{idSvc: identity.NewService(repo, nil, logger), log: logger}
	router := chi.NewRouter()
	router.Use(h.ActivityMiddleware)
	router.Post("/vendor/products/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if database.AuditActorIDFrom(r.Context()) != 7 {
			t.Fatal("transactional actor missing")
		}
		h.redirectWithNotice(w, r, "/vendor/products", "error", "Rejected")
	})
	ctx := authctx.WithActor(context.Background(), authctx.Actor{UserID: 7, OrganizationID: 10})
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/vendor/products/9/edit", strings.NewReader("quantity=12&price=34.50&password=secret&token=private&notes=sensitive"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(httptest.NewRecorder(), req)
	if len(repo.entries) != 1 {
		t.Fatalf("audit entries: %d", len(repo.entries))
	}
	entry := repo.entries[0]
	if entry.ActorUserID != 7 || *entry.OrganizationID != 10 || entry.EntityID != "9" {
		t.Fatalf("wrong attribution: %+v", entry)
	}
	after := entry.After.(map[string]any)
	if after["outcome"] != "rejected" || after["route"] != "/vendor/products/{id}/edit" {
		t.Fatalf("wrong response: %+v", after)
	}
	params := after["parameters"].(map[string][]string)
	if params["price"][0] != "34.50" || len(params) != 2 {
		t.Fatalf("unsafe payload: %+v", params)
	}
}
