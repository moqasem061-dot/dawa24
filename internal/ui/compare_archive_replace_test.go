package ui_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// TestCompareTool_BulkUpload_WhenSpaceAvailable_NoFilesArchived verifies that when
// the user holds 8 files on a 10-file plan and uploads 2 new files (8 + 2 = 10 <= 10),
// all existing 8 files are kept intact without archiving any files.
func TestCompareTool_BulkUpload_WhenSpaceAvailable_NoFilesArchived(t *testing.T) {
	mockRepo := newMockBulkCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	compareSvc := compare.NewService(mockRepo, logger)

	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	handler.SetCompareService(compareSvc)

	r := chi.NewRouter()
	handler.RegisterPublicRoutes(r)
	handler.RegisterAdminRoutes(r)

	userID := int64(99)
	orgID := int64(199)
	actor := authctx.Actor{
		UserID:         userID,
		OrganizationID: orgID,
		OrgType:        "vendor",
		Role:           "vendor",
		Permissions:    []string{"compare.view", "compare.upload"},
	}

	ctx := context.Background()

	// 1. Pre-seed 8 active files (space available: 10 - 8 = 2)
	seededFiles := make([]*compare.CompareFile, 8)
	for i := 0; i < 8; i++ {
		seededFiles[i] = &compare.CompareFile{
			UserID:           userID,
			OrganizationID:   &orgID,
			SupplierName:     fmt.Sprintf("المورد رقم %d", i+1),
			OriginalFilename: fmt.Sprintf("supplier_%d.xlsx", i+1),
			Status:           compare.FileReady,
			RowCount:         50,
			CreatedAt:        time.Now().Add(time.Duration(-(80 - i*10)) * time.Hour),
			UpdatedAt:        time.Now().Add(time.Duration(-(80 - i*10)) * time.Hour),
		}
		_ = mockRepo.CreateFile(ctx, seededFiles[i])
	}

	activeBefore, _ := mockRepo.CountActiveFiles(ctx, userID, &orgID)
	if activeBefore != 8 {
		t.Fatalf("expected 8 active files before upload, got %d", activeBefore)
	}

	// 2. Upload 2 new supplier files (8 + 2 = 10 <= 10) -> fits without archiving
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	p1, _ := writer.CreateFormFile("compare_files", "new_supplier_1.csv")
	_, _ = p1.Write([]byte("كود,اسم,سعر,خصم\n101,بنادول,50.0,15.0\n"))
	p2, _ := writer.CreateFormFile("compare_files", "new_supplier_2.csv")
	_, _ = p2.Write([]byte("كود,اسم,سعر,خصم\n102,كونجستال,35.0,20.0\n"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/compare/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303 on upload, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "notice=success") {
		t.Fatalf("expected success redirect, got %s", loc)
	}
	// Verify notice does NOT mention auto-archiving because space was available
	if strings.Contains(loc, url.QueryEscape("أرشفة")) {
		t.Errorf("notice must NOT mention auto-archiving when space is available, got %s", loc)
	}

	// Allow background workers to settle
	time.Sleep(1500 * time.Millisecond)

	// Verify all 8 original files remain active
	for i, f := range seededFiles {
		cur, err := mockRepo.GetFileByID(ctx, f.ID)
		if err != nil || cur.Status == compare.FileArchived {
			t.Errorf("expected file %d (%s) to remain active, got status=%s", i+1, f.SupplierName, cur.Status)
		}
	}

	activeAfter, _ := mockRepo.CountActiveFiles(ctx, userID, &orgID)
	if activeAfter != 10 {
		t.Fatalf("expected total 10 active files (8 old + 2 new), got %d", activeAfter)
	}
}

// TestCompareTool_BulkUpload_WhenSpaceExceeded_ArchivesOnlyExactOldestOverflow verifies
// that when a user holds 8 files on a 10-file plan and uploads 3 files (8 + 3 = 11 > 10,
// only 2 spaces available), the system archives ONLY the single oldest file (overflow = 1),
// leaving the other 7 files active.
func TestCompareTool_BulkUpload_WhenSpaceExceeded_ArchivesOnlyExactOldestOverflow(t *testing.T) {
	mockRepo := newMockBulkCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	compareSvc := compare.NewService(mockRepo, logger)

	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	handler.SetCompareService(compareSvc)

	r := chi.NewRouter()
	handler.RegisterPublicRoutes(r)
	handler.RegisterAdminRoutes(r)

	userID := int64(99)
	orgID := int64(199)
	actor := authctx.Actor{
		UserID:         userID,
		OrganizationID: orgID,
		OrgType:        "vendor",
		Role:           "vendor",
		Permissions:    []string{"compare.view", "compare.upload"},
	}

	ctx := context.Background()

	// 1. Pre-seed 8 active files with file 0 being the oldest (-80h) and file 7 the newest (-10h)
	seededFiles := make([]*compare.CompareFile, 8)
	for i := 0; i < 8; i++ {
		seededFiles[i] = &compare.CompareFile{
			UserID:           userID,
			OrganizationID:   &orgID,
			SupplierName:     fmt.Sprintf("مورد قديم %d", i+1),
			OriginalFilename: fmt.Sprintf("old_supplier_%d.xlsx", i+1),
			Status:           compare.FileReady,
			RowCount:         50,
			CreatedAt:        time.Now().Add(time.Duration(-(80 - i*10)) * time.Hour),
			UpdatedAt:        time.Now().Add(time.Duration(-(80 - i*10)) * time.Hour),
		}
		_ = mockRepo.CreateFile(ctx, seededFiles[i])
	}

	oldestFile := seededFiles[0] // مورد قديم 1 (-80h)

	// 2. Upload 3 new files (8 + 3 = 11 > 10) -> exactly 1 oldest file must be archived
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	p1, _ := writer.CreateFormFile("compare_files", "batch_new_1.csv")
	_, _ = p1.Write([]byte("كود,اسم,سعر,خصم\n101,بنادول,50.0,15.0\n"))
	p2, _ := writer.CreateFormFile("compare_files", "batch_new_2.csv")
	_, _ = p2.Write([]byte("كود,اسم,سعر,خصم\n102,كونجستال,35.0,20.0\n"))
	p3, _ := writer.CreateFormFile("compare_files", "batch_new_3.csv")
	_, _ = p3.Write([]byte("كود,اسم,سعر,خصم\n103,أوجمنتين,70.0,10.0\n"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/compare/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303 on upload, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "notice=success") {
		t.Fatalf("expected success redirect, got %s", loc)
	}
	// Verify notice mentions archiving exactly 1 file
	if !strings.Contains(loc, url.QueryEscape("أرشفة")) {
		t.Errorf("expected redirect message to mention auto-archiving, got %s", loc)
	}

	// 3. Wait for the oldest file to be archived by the replacement supervisor
	waitForArchived(t, mockRepo, oldestFile.ID)

	fOld, _ := mockRepo.GetFileByID(ctx, oldestFile.ID)
	if fOld.Status != compare.FileArchived || fOld.ArchivedAt == nil || fOld.ArchiveReason == "" {
		t.Errorf("expected oldest file 1 to be archived with timestamp and reason, got status=%s", fOld.Status)
	}
	if fOld.DeletedAt != nil {
		t.Errorf("archived file must not be deleted from database")
	}

	// Verify that the other 7 seeded files (seededFiles[1..7]) are NOT archived!
	for i := 1; i < 8; i++ {
		f, _ := mockRepo.GetFileByID(ctx, seededFiles[i].ID)
		if f.Status == compare.FileArchived {
			t.Errorf("file %d (%s) should NOT be archived; only the single oldest file should be archived", i+1, f.SupplierName)
		}
	}

	// Total active count must be 10 (7 old + 3 new)
	activeAfter, _ := mockRepo.CountActiveFiles(ctx, userID, &orgID)
	if activeAfter != 10 {
		t.Fatalf("expected 10 active files after replacement, got %d", activeAfter)
	}

	// 4. Verify Super Admin can view the archived file in temporary warehouses with archive filter
	adminActor := authctx.Actor{
		UserID:      1,
		Email:       "superadmin@dawa24.com",
		Role:        "super_admin",
		IsStaff:     true,
		IsOwner:     true,
		Permissions: []string{"*"},
	}

	reqAdmin := httptest.NewRequest(http.MethodGet, "/admin/user/temparte-warehouses?status=archived", nil)
	reqAdmin = reqAdmin.WithContext(authctx.WithActor(reqAdmin.Context(), adminActor))
	recAdmin := httptest.NewRecorder()
	r.ServeHTTP(recAdmin, reqAdmin)

	if recAdmin.Code != http.StatusOK {
		t.Fatalf("expected Super Admin page to return 200 OK, got %d", recAdmin.Code)
	}
	bodyAdmin := recAdmin.Body.String()
	if !strings.Contains(bodyAdmin, oldestFile.SupplierName) {
		t.Errorf("expected Super Admin to see archived file %q", oldestFile.SupplierName)
	}
	if !strings.Contains(bodyAdmin, "مؤرشف") {
		t.Errorf("expected Super Admin page to show 'مؤرشف' badge")
	}

	// 5. Verify Compare Tool workspace GET /compare/tool only presents the active files
	reqTool := httptest.NewRequest(http.MethodGet, "/compare/tool", nil)
	reqTool = reqTool.WithContext(authctx.WithActor(reqTool.Context(), actor))
	recTool := httptest.NewRecorder()
	r.ServeHTTP(recTool, reqTool)

	if recTool.Code != http.StatusOK {
		t.Fatalf("expected Compare Tool page to return 200 OK, got %d", recTool.Code)
	}
	bodyTool := recTool.Body.String()
	if strings.Contains(bodyTool, "name=\"supplier_ids\" value=\""+fmt.Sprintf("%d", oldestFile.ID)+"\"") {
		t.Errorf("archived file should NOT be in active comparison selection checkbox")
	}
	if strings.Contains(bodyTool, oldestFile.SupplierName) {
		t.Errorf("archived list %q must not appear in the vendor file centre", oldestFile.SupplierName)
	}
}

// waitForArchived blocks until every id is archived, or fails the test.
//
// The replacement runs in a goroutine that outlives the upload request, so the
// assertion has to wait for it the way the vendor's screen does.
func waitForArchived(t *testing.T, repo interface {
	GetFileByID(ctx context.Context, id int64) (*compare.CompareFile, error)
}, ids ...int64) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		pending := 0
		for _, id := range ids {
			f, err := repo.GetFileByID(context.Background(), id)
			if err != nil || f == nil || f.Status != compare.FileArchived {
				pending++
			}
		}
		if pending == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d superseded file(s) were never archived after the batch was staged", pending)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
