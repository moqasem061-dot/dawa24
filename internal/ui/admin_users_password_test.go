package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func TestAdminUserPasswordSubmit_Success(t *testing.T) {
	h, _, repo, _ := setupTestMFAHandler(t)

	// Create user in repo
	user := &identity.User{
		ID:    42,
		Email: "target@dawa24.eg",
		Name:  i18n.Text{i18n.AR: "مستخدم تجريبي", i18n.EN: "Target User"},
	}
	repo.users[user.ID] = user
	repo.usersByEm[user.Email] = user

	form := url.Values{
		"password":         {"StrongPassword123!"},
		"confirm_password": {"StrongPassword123!"},
		"redirect":         {"/admin/users"},
	}

	req := httptest.NewRequest("POST", "/admin/users/42/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "42")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)

	actor := authctx.Actor{
		UserID:  1,
		IsStaff: true,
	}
	ctx = authctx.WithActor(ctx, actor)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.AdminUserPasswordSubmit(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status 303 SeeOther, got %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "kind=success") {
		t.Fatalf("expected redirect with kind=success, got %q", loc)
	}
}

func TestAdminUserPasswordSubmit_WeakPassword(t *testing.T) {
	h, _, repo, _ := setupTestMFAHandler(t)

	user := &identity.User{
		ID:    42,
		Email: "target@dawa24.eg",
		Name:  i18n.Text{i18n.AR: "مستخدم تجريبي", i18n.EN: "Target User"},
	}
	repo.users[user.ID] = user

	form := url.Values{
		"password":         {"weakpass"},
		"confirm_password": {"weakpass"},
		"redirect":         {"/admin/users"},
	}

	req := httptest.NewRequest("POST", "/admin/users/42/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "42")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)

	actor := authctx.Actor{
		UserID:  1,
		IsStaff: true,
	}
	ctx = authctx.WithActor(ctx, actor)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.AdminUserPasswordSubmit(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status 303 SeeOther, got %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "kind=error") {
		t.Fatalf("expected redirect with kind=error, got %q", loc)
	}
}

func TestAdminUserPasswordSubmit_Mismatch(t *testing.T) {
	h, _, repo, _ := setupTestMFAHandler(t)

	user := &identity.User{
		ID:    42,
		Email: "target@dawa24.eg",
		Name:  i18n.Text{i18n.AR: "مستخدم تجريبي", i18n.EN: "Target User"},
	}
	repo.users[user.ID] = user

	form := url.Values{
		"password":         {"StrongPassword123!"},
		"confirm_password": {"DifferentPassword123!"},
		"redirect":         {"/admin/users"},
	}

	req := httptest.NewRequest("POST", "/admin/users/42/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "42")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)

	actor := authctx.Actor{
		UserID:  1,
		IsStaff: true,
	}
	ctx = authctx.WithActor(ctx, actor)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.AdminUserPasswordSubmit(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status 303 SeeOther, got %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "kind=error") {
		t.Fatalf("expected redirect with kind=error, got %q", loc)
	}
}

func TestAdminUserPasswordSubmit_FallbackUserID(t *testing.T) {
	h, _, repo, _ := setupTestMFAHandler(t)

	user := &identity.User{
		ID:    42,
		Email: "target@dawa24.eg",
		Name:  i18n.Text{i18n.AR: "مستخدم تجريبي", i18n.EN: "Target User"},
	}
	repo.users[user.ID] = user

	// Form has user_id, URL param is empty (e.g. POST /admin/users/password)
	form := url.Values{
		"user_id":          {"42"},
		"password":         {"StrongPassword123!"},
		"confirm_password": {"StrongPassword123!"},
		"redirect":         {"/admin/users"},
	}

	req := httptest.NewRequest("POST", "/admin/users/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	actor := authctx.Actor{
		UserID:  1,
		IsStaff: true,
	}
	ctx := authctx.WithActor(req.Context(), actor)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.AdminUserPasswordSubmit(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status 303 SeeOther, got %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "kind=success") {
		t.Fatalf("expected redirect with kind=success, got %q", loc)
	}
}

func TestAdminUserEditSubmit_WithPassword(t *testing.T) {
	h, _, repo, _ := setupTestMFAHandler(t)

	user := &identity.User{
		ID:    42,
		Email: "target@dawa24.eg",
		Name:  i18n.Text{i18n.AR: "مستخدم تجريبي", i18n.EN: "Target User"},
	}
	repo.users[user.ID] = user

	form := url.Values{
		"name_ar":          {"اسم معدل"},
		"name_en":          {"Updated Name"},
		"email":            {"target@dawa24.eg"},
		"status":           {"active"},
		"password":         {"NewStrongPass123!"},
		"confirm_password": {"NewStrongPass123!"},
		"redirect":         {"/admin/users"},
	}

	req := httptest.NewRequest("POST", "/admin/users/42/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "42")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)

	actor := authctx.Actor{
		UserID:  1,
		IsStaff: true,
	}
	ctx = authctx.WithActor(ctx, actor)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.AdminUserEditSubmit(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status 303 SeeOther, got %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "kind=success") {
		t.Fatalf("expected redirect with kind=success, got %q", loc)
	}
}
