package ui_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/importrun"
	"github.com/moqasem061-dot/dawa24/internal/ui"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

type completedReviewRepo struct {
	*mockImportRunRepo
	done chan struct{}
}

func (m *completedReviewRepo) FailRun(ctx context.Context, id int64, message string) error {
	err := m.mockImportRunRepo.FailRun(ctx, id, message)
	close(m.done)
	return err
}

func (m *completedReviewRepo) TransitionState(ctx context.Context, id int64, state string) error {
	err := m.mockImportRunRepo.TransitionState(ctx, id, state)
	close(m.done)
	return err
}

func TestReviewAllIncludesOtherUploadersArchivedAndEveryPage(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, log)
	runs := &completedReviewRepo{mockImportRunRepo: newMockImportRunRepo(), done: make(chan struct{})}
	h.SetImportRunRepo(runs)
	repo := newMockBulkCompareRepo()
	path := filepath.Join(t.TempDir(), "warehouse.csv")
	if err := os.WriteFile(path, []byte("code,name,price,discount\nP1,Panadol 500 mg,123.45,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for id := int64(1); id <= 105; id++ {
		org := int64(100)
		file := &compare.CompareFile{ID: id, UserID: id + 1000, OrganizationID: &org, OriginalFilename: "warehouse.csv", StorageKey: path, Status: compare.FileReady}
		if id == 2 {
			now := time.Now()
			file.Status = compare.FileArchived
			file.ArchivedAt = &now
		}
		if id == 105 {
			file.StorageKey = "missing-original.csv"
		}
		repo.files[id] = file
	}
	h.SetCompareService(compare.NewService(repo, log))
	ctx := authctx.WithActor(context.Background(), authctx.Actor{UserID: 41, Role: "super_admin"})
	w := httptest.NewRecorder()
	h.AdminTempWarehouseReviewAllSubmit(w, httptest.NewRequestWithContext(ctx, http.MethodPost, "/admin/temporary-warehouses/review-all?status=archived&q=nonexistent&page=2", nil))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	select {
	case <-runs.done:
	case <-time.After(20 * time.Second):
		t.Fatal("review did not finish")
	}
	for _, run := range runs.runs {
		var payload struct {
			Completed, Succeeded int
			FailedIDs            []int64 `json:"failed_ids"`
			FileIDs              []int64 `json:"file_ids"`
		}
		if err := json.Unmarshal(run.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.FileIDs) != 105 || payload.Completed != 105 || payload.Succeeded != 104 || len(payload.FailedIDs) != 1 || payload.FailedIDs[0] != 105 || run.State != importrun.StateFailed {
			t.Fatalf("run=%+v payload=%+v", run, payload)
		}
	}
	file, _ := repo.GetFileByID(ctx, 2)
	if file.Status != compare.FileArchived || file.ArchivedAt == nil {
		t.Fatal("archived file was restored")
	}
	rows, err := repo.ListFileRows(ctx, 104, 10, 0)
	if err != nil || len(rows) != 1 || len(rows[0].MatchDecision) == 0 {
		t.Fatalf("last-page products not matched: %+v %v", rows, err)
	}
}

func TestReviewAllRequiresSuperAdmin(t *testing.T) {
	h, _, _ := setupResumableTestHandler(t)
	for _, role := range []string{"", "admin", "moderator", "vendor"} {
		ctx := authctx.WithActor(context.Background(), authctx.Actor{UserID: 41, Role: role})
		for _, handler := range []http.HandlerFunc{h.AdminTempWarehouseReviewAllSubmit, h.AdminTempWarehouseReviewAllPage} {
			w := httptest.NewRecorder()
			handler(w, httptest.NewRequestWithContext(ctx, http.MethodPost, "/admin/temporary-warehouses/review-all", nil))
			if w.Code != http.StatusForbidden {
				t.Fatalf("role %s status %d", role, w.Code)
			}
		}
	}
}

func TestWarehouseSearchPreservesStatusAndReviewButtonScope(t *testing.T) {
	for _, canReview := range []bool{false, true} {
		var html strings.Builder
		view := &pages.AdminTempWarehousesData{Base: "/admin/temporary-warehouses", PageURL: "/admin/user/temparte-warehouses", StatusFilter: "archived", CanReviewAll: canReview, PerPage: 25, Page: 1}
		if err := pages.AdminTempWarehousesPage(view, "en", "ltr").Render(context.Background(), &html); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(html.String(), `name="status" value="archived"`) {
			t.Fatal("search loses archive filter")
		}
		if strings.Contains(html.String(), `action="/admin/temporary-warehouses/review-all"`) != canReview {
			t.Fatal("review button visibility ignores super admin flag")
		}
		if !strings.Contains(html.String(), "status=active") {
			t.Fatal("active tab only selects ready warehouses")
		}
	}
}
