package ui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockBulkCompareRepo struct {
	compare.Repository
	mu       sync.Mutex
	files    map[int64]*compare.CompareFile
	fileRows map[int64][]*compare.CompareFileRow
	nextID   int64
}

func newMockBulkCompareRepo() *mockBulkCompareRepo {
	return &mockBulkCompareRepo{
		files:    make(map[int64]*compare.CompareFile),
		fileRows: make(map[int64][]*compare.CompareFileRow),
		nextID:   1,
	}
}

func (m *mockBulkCompareRepo) CreateFile(ctx context.Context, f *compare.CompareFile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f.ID = m.nextID
	m.nextID++
	m.files[f.ID] = f
	return nil
}

func (m *mockBulkCompareRepo) UpdateFile(ctx context.Context, f *compare.CompareFile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[f.ID] = f
	return nil
}

func (m *mockBulkCompareRepo) GetFileByID(ctx context.Context, id int64) (*compare.CompareFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.files[id], nil
}

func (m *mockBulkCompareRepo) ListFiles(ctx context.Context, userID int64, orgID *int64, status *compare.CompareFileStatus) ([]*compare.CompareFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []*compare.CompareFile
	for _, f := range m.files {
		if status == nil {
			if f.Status == compare.FileArchived {
				continue
			}
		} else if f.Status != *status {
			continue
		}
		if (orgID != nil && f.OrganizationID != nil && *f.OrganizationID == *orgID) || (f.OrganizationID == nil && f.UserID == userID) {
			list = append(list, f)
		}
	}
	return list, nil
}

func (m *mockBulkCompareRepo) ListAllFiles(ctx context.Context, query string, status *compare.CompareFileStatus) ([]*compare.CompareFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*compare.CompareFile
	for _, f := range m.files {
		out = append(out, f)
	}
	return out, nil
}

func (m *mockBulkCompareRepo) CountActiveFiles(ctx context.Context, userID int64, orgID *int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, f := range m.files {
		if f.Status != compare.FileArchived && f.DeletedAt == nil && !f.IsTempWarehouse {
			if (orgID != nil && f.OrganizationID != nil && *f.OrganizationID == *orgID) || (f.OrganizationID == nil && f.UserID == userID) {
				count++
			}
		}
	}
	return count, nil
}

func (m *mockBulkCompareRepo) GetActiveSubscription(ctx context.Context, userID int64, orgID *int64) (*compare.Subscription, error) {
	return nil, nil
}

func (m *mockBulkCompareRepo) ListAdminTempWarehouses(ctx context.Context, filter compare.AdminTempWarehouseFilter) ([]*compare.AdminTempWarehouse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*compare.AdminTempWarehouse
	for _, f := range m.files {
		if filter.OwnerOnly != nil && f.UserID != *filter.OwnerOnly {
			continue
		}
		out = append(out, &compare.AdminTempWarehouse{CompareFile: f})
	}
	return out, nil
}

func (m *mockBulkCompareRepo) ListAdminTempWarehousesWithTotal(ctx context.Context, filter compare.AdminTempWarehouseFilter, _, _ int) ([]*compare.AdminTempWarehouse, int, error) {
	list, err := m.ListAdminTempWarehouses(ctx, filter)
	return list, len(list), err
}

func (m *mockBulkCompareRepo) AdminTempWarehouseStats(ctx context.Context, filter compare.AdminTempWarehouseFilter) (int64, int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var totalRows int64
	var activeCount, archivedCount int
	for _, f := range m.files {
		totalRows += int64(f.RowCount)
		if f.Status == compare.FileReady {
			activeCount++
		} else if f.Status == compare.FileArchived {
			archivedCount++
		}
	}
	return totalRows, activeCount, archivedCount, nil
}

func (m *mockBulkCompareRepo) ListTempWarehouseUploaders(ctx context.Context) ([]compare.FileUploader, error) {
	return nil, nil
}

func (m *mockBulkCompareRepo) SetFileVisibility(ctx context.Context, id int64, visibility string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f := m.files[id]; f != nil {
		f.Visibility = visibility
	}
	return nil
}

func (m *mockBulkCompareRepo) InsertFileRows(ctx context.Context, rows []*compare.CompareFileRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range rows {
		m.fileRows[r.FileID] = append(m.fileRows[r.FileID], r)
	}
	return nil
}

func (m *mockBulkCompareRepo) ListFileRows(ctx context.Context, fileID int64, limit, offset int) ([]*compare.CompareFileRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fileRows[fileID], nil
}

func (m *mockBulkCompareRepo) GetFileRowsPaginated(ctx context.Context, fileID int64, page, limit int) ([]*compare.CompareFileRow, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows := m.fileRows[fileID]
	return rows, int64(len(rows)), nil
}

func (m *mockBulkCompareRepo) DeleteFileRowOwnedBy(ctx context.Context, rowID int64, ownerUserID int64) error {
	return nil
}

func (m *mockBulkCompareRepo) DeleteFileRow(ctx context.Context, rowID int64) error {
	return nil
}

func (m *mockBulkCompareRepo) DeleteFileRows(ctx context.Context, fileID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.fileRows, fileID)
	return nil
}

func (m *mockBulkCompareRepo) DeleteFile(ctx context.Context, fileID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, fileID)
	delete(m.fileRows, fileID)
	return nil
}

func (m *mockBulkCompareRepo) RenameFile(ctx context.Context, id int64, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f, ok := m.files[id]; ok {
		f.SupplierName = newName
	}
	return nil
}

func (m *mockBulkCompareRepo) ArchiveActiveFiles(ctx context.Context, userID int64, orgID *int64, reason string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var archived []string
	for _, f := range m.files {
		if f.Status != compare.FileArchived && !f.IsTempWarehouse && f.DeletedAt == nil {
			if (orgID != nil && f.OrganizationID != nil && *f.OrganizationID == *orgID) || (f.OrganizationID == nil && f.UserID == userID) {
				f.Status = compare.FileArchived
				now := time.Now()
				f.ArchivedAt = &now
				f.ArchiveReason = reason
				archived = append(archived, f.SupplierName)
			}
		}
	}
	return archived, nil
}

func (m *mockBulkCompareRepo) ArchiveFile(ctx context.Context, id int64, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f, ok := m.files[id]; ok {
		f.Status = compare.FileArchived
		f.ArchiveReason = reason
		now := time.Now()
		f.ArchivedAt = &now
	}
	return nil
}

func (m *mockBulkCompareRepo) UnarchiveFile(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f, ok := m.files[id]; ok {
		f.Status = compare.FileReady
		f.ArchivedAt = nil
		f.ArchiveReason = ""
	}
	return nil
}

func (m *mockBulkCompareRepo) BulkDeleteFiles(ctx context.Context, ids []int64, ownerID *int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	for _, id := range ids {
		if f, ok := m.files[id]; ok {
			if ownerID == nil || *ownerID <= 0 || f.UserID == *ownerID {
				delete(m.files, id)
				delete(m.fileRows, id)
				count++
			}
		}
	}
	return count, nil
}

func (m *mockBulkCompareRepo) BulkArchiveFiles(ctx context.Context, ids []int64, ownerID *int64, reason string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	now := time.Now()
	for _, id := range ids {
		if f, ok := m.files[id]; ok {
			if ownerID == nil || *ownerID <= 0 || f.UserID == *ownerID {
				f.Status = compare.FileArchived
				f.ArchiveReason = reason
				f.ArchivedAt = &now
				count++
			}
		}
	}
	return count, nil
}

func (m *mockBulkCompareRepo) BulkUnarchiveFiles(ctx context.Context, ids []int64, ownerID *int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	for _, id := range ids {
		if f, ok := m.files[id]; ok {
			if ownerID == nil || *ownerID <= 0 || f.UserID == *ownerID {
				f.Status = compare.FileReady
				f.ArchivedAt = nil
				f.ArchiveReason = ""
				count++
			}
		}
	}
	return count, nil
}

func (m *mockBulkCompareRepo) PurgeExpiredCompareFiles(ctx context.Context, defaultRetentionDays int) (int64, error) {
	return 0, nil
}

func (m *mockBulkCompareRepo) AutoArchiveTempWarehouses(ctx context.Context, olderThanHours int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	cutoff := time.Now().Add(-time.Duration(olderThanHours) * time.Hour)
	for _, f := range m.files {
		if f.DeletedAt == nil && f.Status == compare.FileReady && f.CreatedAt.Before(cutoff) {
			f.Status = compare.FileArchived
			now := time.Now()
			f.ArchivedAt = &now
			f.ArchiveReason = "Auto Archived"
			count++
		}
	}
	return count, nil
}

func (m *mockBulkCompareRepo) PurgeArchivedTempWarehouses(ctx context.Context, olderThanDays int) ([]string, int64, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keys []string
	var fileCount, rowCount int64
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	for id, f := range m.files {
		if f.DeletedAt == nil && f.Status == compare.FileArchived && f.ArchivedAt != nil && f.ArchivedAt.Before(cutoff) {
			now := time.Now()
			f.DeletedAt = &now
			f.Status = "deleted"
			fileCount++
			if f.StorageKey != "" {
				keys = append(keys, f.StorageKey)
			}
			if rows, ok := m.fileRows[id]; ok {
				rowCount += int64(len(rows))
				delete(m.fileRows, id)
			}
		}
	}
	return keys, fileCount, rowCount, nil
}

func TestAdminTempWarehouse_BulkUpload_65Files_HighSpeed(t *testing.T) {
	mockRepo := newMockBulkCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	compareSvc := compare.NewService(mockRepo, logger)

	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	handler.SetCompareService(compareSvc)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	// Build a multipart form with 65 distinct warehouse CSV files
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	totalFiles := 65
	itemsPerFile := 20
	expectedTotalItems := totalFiles * itemsPerFile

	startGen := time.Now()
	for i := 1; i <= totalFiles; i++ {
		filename := fmt.Sprintf("مستودع_المورد_رقم_%02d.csv", i)
		part, err := writer.CreateFormFile("files", filename)
		if err != nil {
			t.Fatalf("failed to create form file part %d: %v", i, err)
		}

		// Write CSV content with headers and 20 items
		var csvContent strings.Builder
		csvContent.WriteString("كود الصنف (SKU),اسم الدواء والمنتج,سعر الجمهور,نسبة الخصم %\n")
		for j := 1; j <= itemsPerFile; j++ {
			csvContent.WriteString(fmt.Sprintf("SKU-%d-%d,دواء بنادول اكسترا تشغيلة %d-%d,%d.50,15.5%%\n", i, j, i, j, 50+j))
		}
		_, _ = part.Write([]byte(csvContent.String()))
	}
	_ = writer.Close()
	t.Logf("Generated 65 files multipart payload in %v (size: %.2f KB)", time.Since(startGen), float64(body.Len())/1024.0)

	// 1. Send bulk upload request with AJAX/JSON header
	req := httptest.NewRequest(http.MethodPost, "/admin/temporary-warehouses/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	adminActor := authctx.Actor{
		UserID:      1,
		Email:       "admin@dawa24.com",
		Role:        "super_admin",
		IsStaff:     true,
		IsOwner:     true,
		Permissions: []string{"inventory.warehouse.update", "inventory.temp_warehouse.view"},
	}
	req = req.WithContext(authctx.WithActor(req.Context(), adminActor))

	rec := httptest.NewRecorder()
	startProc := time.Now()
	r.ServeHTTP(rec, req)
	elapsed := time.Since(startProc)

	t.Logf("Processed 65 files bulk upload in %v", elapsed)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var jsonResp struct {
		Success         bool     `json:"success"`
		TotalFiles      int      `json:"total_files"`
		SuccessfulFiles int      `json:"successful_files"`
		FailedFiles     int      `json:"failed_files"`
		TotalItems      int64    `json:"total_items"`
		UploadedIDs     []string `json:"uploaded_ids"`
		Message         string   `json:"message"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &jsonResp); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v, raw: %s", err, rec.Body.String())
	}

	if !jsonResp.Success {
		t.Fatalf("expected success=true, got false")
	}
	if jsonResp.TotalFiles != totalFiles {
		t.Errorf("expected TotalFiles=%d, got %d", totalFiles, jsonResp.TotalFiles)
	}
	if jsonResp.SuccessfulFiles != totalFiles {
		t.Errorf("expected SuccessfulFiles=%d, got %d", totalFiles, jsonResp.SuccessfulFiles)
	}
	if jsonResp.FailedFiles != 0 {
		t.Errorf("expected FailedFiles=0, got %d", jsonResp.FailedFiles)
	}
	// The rows arrive AFTER the response: the parse is detached. Sixty-five
	// workbooks read inside the POST is what made this endpoint time out.
	waitForTempWarehouseStaging(t, mockRepo, jsonResp.UploadedIDs)

	// Verify in mock repository
	files, _ := mockRepo.ListAllFiles(context.Background(), "", nil)
	if len(files) != totalFiles {
		t.Errorf("expected %d files saved in database, got %d", totalFiles, len(files))
	}

	totalSavedRows := 0
	for _, f := range files {
		rows, _ := mockRepo.ListFileRows(context.Background(), f.ID, 100, 0)
		totalSavedRows += len(rows)
		if f.RowCount != itemsPerFile {
			t.Errorf("file %s has RowCount=%d, expected %d", f.SupplierName, f.RowCount, itemsPerFile)
		}
	}
	if totalSavedRows != expectedTotalItems {
		t.Errorf("expected %d total rows in database, got %d", expectedTotalItems, totalSavedRows)
	}
}
