package ui_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/ui"
)

func TestAdminTempWarehouse_BulkActions_Archive_Unarchive_Delete(t *testing.T) {
	mockRepo := newMockBulkCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	compareSvc := compare.NewService(mockRepo, logger)

	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	handler.SetCompareService(compareSvc)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	adminActor := authctx.Actor{
		UserID:      1,
		Email:       "admin@dawa24.com",
		Role:        "super_admin",
		IsStaff:     true,
		IsOwner:     true,
		Permissions: []string{"*"},
	}

	// 1. Create test files in mockRepo
	f1 := &compare.CompareFile{
		UserID:       1,
		SupplierName: "مستودع 1",
		Status:       compare.FileReady,
	}
	_ = mockRepo.CreateFile(context.Background(), f1)

	f2 := &compare.CompareFile{
		UserID:       1,
		SupplierName: "مستودع 2",
		Status:       compare.FileReady,
	}
	_ = mockRepo.CreateFile(context.Background(), f2)

	f3 := &compare.CompareFile{
		UserID:       1,
		SupplierName: "مستودع 3",
		Status:       compare.FileReady,
	}
	_ = mockRepo.CreateFile(context.Background(), f3)

	// Test 1: Bulk Archive (Super Admin)
	formArchive := url.Values{
		"bulk_action":  {"archive"},
		"selected_ids": {fmt.Sprintf("%d", f1.ID), fmt.Sprintf("%d", f2.ID)},
	}
	reqArchive := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/admin/temporary-warehouses/bulk", strings.NewReader(formArchive.Encode()))
	reqArchive.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqArchive = reqArchive.WithContext(authctx.WithActor(reqArchive.Context(), adminActor))

	recArchive := httptest.NewRecorder()
	r.ServeHTTP(recArchive, reqArchive)

	if recArchive.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on bulk archive, got %d", recArchive.Code)
	}

	// Check status of f1 & f2
	f1Check, _ := mockRepo.GetFileByID(context.Background(), f1.ID)
	if f1Check.Status != compare.FileArchived {
		t.Errorf("expected f1 to be archived, got %s", f1Check.Status)
	}
	f2Check, _ := mockRepo.GetFileByID(context.Background(), f2.ID)
	if f2Check.Status != compare.FileArchived {
		t.Errorf("expected f2 to be archived, got %s", f2Check.Status)
	}

	// Test 2: Bulk Unarchive (Super Admin)
	formUnarchive := url.Values{
		"bulk_action":  {"unarchive"},
		"selected_ids": {fmt.Sprintf("%d", f1.ID), fmt.Sprintf("%d", f2.ID)},
	}
	reqUnarchive := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/admin/temporary-warehouses/bulk", strings.NewReader(formUnarchive.Encode()))
	reqUnarchive.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqUnarchive = reqUnarchive.WithContext(authctx.WithActor(reqUnarchive.Context(), adminActor))

	recUnarchive := httptest.NewRecorder()
	r.ServeHTTP(recUnarchive, reqUnarchive)

	if recUnarchive.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on bulk unarchive, got %d", recUnarchive.Code)
	}

	f1Unarchived, _ := mockRepo.GetFileByID(context.Background(), f1.ID)
	if f1Unarchived.Status != compare.FileReady {
		t.Errorf("expected f1 to be unarchived (ready), got %s", f1Unarchived.Status)
	}

	// Test 3: Bulk Delete (My Temp Warehouses)
	formDelete := url.Values{
		"bulk_action":  {"delete"},
		"selected_ids": {fmt.Sprintf("%d", f1.ID), fmt.Sprintf("%d", f3.ID)},
	}
	reqDelete := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/admin/my/temparte-warehouses/bulk", strings.NewReader(formDelete.Encode()))
	reqDelete.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqDelete = reqDelete.WithContext(authctx.WithActor(reqDelete.Context(), adminActor))
	recDelete := httptest.NewRecorder()
	r.ServeHTTP(recDelete, reqDelete)
	if recDelete.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on bulk delete, got %d", recDelete.Code)
	}

	deletedF1, _ := mockRepo.GetFileByID(context.Background(), f1.ID)
	if deletedF1 != nil {
		t.Errorf("expected f1 to be deleted from repo, got %+v", deletedF1)
	}
}

func TestAdminTempWarehouse_SortingAndColumns(t *testing.T) {
	mockRepo := newMockBulkCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	compareSvc := compare.NewService(mockRepo, logger)

	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	handler.SetCompareService(compareSvc)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	adminActor := authctx.Actor{
		UserID:         1,
		IsStaff:        true,
		Email:          "admin@dawa24.com",
		Role:           "super_admin",
		OrganizationID: 0,
		Permissions:    []string{"*"},
	}

	// 1. Test Super Admin GET with sorting
	req1 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/admin/user/temparte-warehouses?sort=supplier&order=asc", nil)
	req1 = req1.WithContext(authctx.WithActor(req1.Context(), adminActor))
	rec1 := httptest.NewRecorder()
	r.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec1.Code)
	}
	body1 := rec1.Body.String()
	if !strings.Contains(body1, "sort=supplier") {
		t.Errorf("expected HTML to contain sort=supplier link")
	}

	// 2. Test My Warehouses GET with sorting
	req2 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/admin/my/temparte-warehouses?sort=rows&order=desc", nil)
	req2 = req2.WithContext(authctx.WithActor(req2.Context(), adminActor))
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec2.Code)
	}

	// 3. Test Team Warehouses GET with sorting
	req3 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/admin/team/temparte-warehouses?sort=date&order=asc", nil)
	req3 = req3.WithContext(authctx.WithActor(req3.Context(), adminActor))
	rec3 := httptest.NewRecorder()
	r.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec3.Code)
	}
}
