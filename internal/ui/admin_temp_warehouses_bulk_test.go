package ui_test

import (
	"context"
	"sync"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
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
	m.files[f.ID] = snapshotBulkCompareFile(f)
	return nil
}

func (m *mockBulkCompareRepo) UpdateFile(ctx context.Context, f *compare.CompareFile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[f.ID] = snapshotBulkCompareFile(f)
	return nil
}

func (m *mockBulkCompareRepo) GetFileByID(ctx context.Context, id int64) (*compare.CompareFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return snapshotBulkCompareFile(m.files[id]), nil
}

func snapshotBulkCompareFile(file *compare.CompareFile) *compare.CompareFile {
	if file == nil {
		return nil
	}
	snapshot := *file
	return &snapshot
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
			list = append(list, snapshotBulkCompareFile(f))
		}
	}
	return list, nil
}

func (m *mockBulkCompareRepo) ListAllFiles(ctx context.Context, query string, status *compare.CompareFileStatus) ([]*compare.CompareFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*compare.CompareFile
	for _, f := range m.files {
		out = append(out, snapshotBulkCompareFile(f))
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
		switch f.Status {
		case compare.FileReady:
			activeCount++
		case compare.FileArchived:
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
