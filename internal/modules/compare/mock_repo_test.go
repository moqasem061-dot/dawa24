package compare_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Compare files mock store
func (m *mockCompareRepo) CreateFile(ctx context.Context, f *compare.CompareFile) error {
	if f.ID == 0 {
		m.nextID++
		f.ID = m.nextID
	} else if f.ID >= m.nextID {
		m.nextID = f.ID + 1
	}
	if f.PublicID == "" {
		f.PublicID = fmt.Sprintf("pub-file-%d", f.ID)
	}
	m.files[f.ID] = f
	return nil
}

func (m *mockCompareRepo) GetFileByID(ctx context.Context, id int64) (*compare.CompareFile, error) {
	if f, ok := m.files[id]; ok {
		return f, nil
	}
	return nil, apperr.NotFound("compare file")
}

func (m *mockCompareRepo) GetFileByPublicID(ctx context.Context, publicID string) (*compare.CompareFile, error) {
	for _, f := range m.files {
		if f.PublicID == publicID {
			return f, nil
		}
	}
	return nil, apperr.NotFound("compare file")
}

func (m *mockCompareRepo) ListFiles(ctx context.Context, userID int64, orgID *int64, status *compare.CompareFileStatus) ([]*compare.CompareFile, error) {
	var list []*compare.CompareFile
	for _, f := range m.files {
		if status != nil && f.Status != *status {
			continue
		}
		if (orgID != nil && f.OrganizationID != nil && *f.OrganizationID == *orgID) || (f.OrganizationID == nil && f.UserID == userID) {
			list = append(list, f)
		}
	}
	return list, nil
}

func (m *mockCompareRepo) ListAllFiles(ctx context.Context, search string, status *compare.CompareFileStatus) ([]*compare.CompareFile, error) {
	var list []*compare.CompareFile
	for _, f := range m.files {
		if status != nil && f.Status != *status {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(f.SupplierName), strings.ToLower(search)) && !strings.Contains(strings.ToLower(f.OriginalFilename), strings.ToLower(search)) {
			continue
		}
		list = append(list, f)
	}
	return list, nil
}

func (m *mockCompareRepo) ListAdminTempWarehouses(ctx context.Context, filter compare.AdminTempWarehouseFilter) ([]*compare.AdminTempWarehouse, error) {
	var list []*compare.AdminTempWarehouse
	for _, f := range m.files {
		if filter.OwnerOnly != nil && f.UserID != *filter.OwnerOnly {
			continue
		}
		list = append(list, &compare.AdminTempWarehouse{CompareFile: f})
	}
	return list, nil
}

func (m *mockCompareRepo) ListAdminTempWarehousesWithTotal(ctx context.Context, filter compare.AdminTempWarehouseFilter, _, _ int) ([]*compare.AdminTempWarehouse, int, error) {
	list, err := m.ListAdminTempWarehouses(ctx, filter)
	return list, len(list), err
}

func (m *mockCompareRepo) AdminTempWarehouseStats(ctx context.Context, filter compare.AdminTempWarehouseFilter) (int64, int, int, error) {
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

func (m *mockCompareRepo) ListTempWarehouseUploaders(ctx context.Context) ([]compare.FileUploader, error) {
	return nil, nil
}

func (m *mockCompareRepo) SetFileVisibility(ctx context.Context, id int64, visibility string) error {
	if f, ok := m.files[id]; ok {
		f.Visibility = visibility
	}
	return nil
}

func (m *mockCompareRepo) CountActiveFiles(ctx context.Context, userID int64, orgID *int64) (int, error) {
	count := 0
	for _, f := range m.files {
		if f.Status != compare.FileArchived {
			if (orgID != nil && f.OrganizationID != nil && *f.OrganizationID == *orgID) || (f.OrganizationID == nil && f.UserID == userID) {
				count++
			}
		}
	}
	return count, nil
}

func (m *mockCompareRepo) UpdateFile(ctx context.Context, f *compare.CompareFile) error {
	m.files[f.ID] = f
	return nil
}

func (m *mockCompareRepo) RenameFile(ctx context.Context, id int64, newSupplierName string) error {
	if f, ok := m.files[id]; ok {
		f.SupplierName = newSupplierName
		return nil
	}
	return apperr.NotFound("compare file")
}

func (m *mockCompareRepo) ArchiveOldestFiles(ctx context.Context, userID int64, orgID *int64, keepCount int, reason string) ([]string, error) {
	var active []*compare.CompareFile
	for _, f := range m.files {
		if f.Status != compare.FileArchived {
			if (orgID != nil && f.OrganizationID != nil && *f.OrganizationID == *orgID) || (f.OrganizationID == nil && f.UserID == userID) {
				active = append(active, f)
			}
		}
	}
	if len(active) <= keepCount {
		return nil, nil
	}
	sort.Slice(active, func(i, j int) bool { return active[i].ID < active[j].ID })
	toArchiveCount := len(active) - keepCount
	var archivedNames []string
	for i := 0; i < toArchiveCount; i++ {
		f := active[i]
		f.Status = compare.FileArchived
		f.ArchiveReason = reason
		now := time.Now().UTC()
		f.ArchivedAt = &now
		archivedNames = append(archivedNames, f.SupplierName)
		f.SupplierName = f.SupplierName + " - مؤرشف 1"
	}
	return archivedNames, nil
}

func (m *mockCompareRepo) ArchiveActiveFiles(ctx context.Context, userID int64, orgID *int64, reason string) ([]string, error) {
	var archived []string
	for _, f := range m.files {
		if f.Status != compare.FileArchived && !f.IsTempWarehouse && f.DeletedAt == nil {
			if (orgID != nil && f.OrganizationID != nil && *f.OrganizationID == *orgID) || (f.OrganizationID == nil && f.UserID == userID) {
				f.Status = compare.FileArchived
				now := time.Now().UTC()
				f.ArchivedAt = &now
				f.ArchiveReason = reason
				archived = append(archived, f.SupplierName)
			}
		}
	}
	return archived, nil
}

func (m *mockCompareRepo) ArchiveFile(ctx context.Context, id int64, reason string) error {
	if f, ok := m.files[id]; ok {
		f.Status = compare.FileArchived
		f.ArchiveReason = reason
		now := time.Now().UTC()
		f.ArchivedAt = &now
		return nil
	}
	return apperr.NotFound("compare file")
}

func (m *mockCompareRepo) UnarchiveFile(ctx context.Context, id int64) error {
	if f, ok := m.files[id]; ok {
		f.Status = compare.FileReady
		f.ArchivedAt = nil
		f.ArchiveReason = ""
		return nil
	}
	return apperr.NotFound("compare file")
}

func (m *mockCompareRepo) DeleteFile(ctx context.Context, id int64) error {
	delete(m.files, id)
	return nil
}

func (m *mockCompareRepo) BulkDeleteFiles(ctx context.Context, ids []int64, ownerID *int64) (int64, error) {
	var count int64
	for _, id := range ids {
		if f, ok := m.files[id]; ok {
			if ownerID == nil || *ownerID <= 0 || f.UserID == *ownerID {
				delete(m.files, id)
				count++
			}
		}
	}
	return count, nil
}

func (m *mockCompareRepo) BulkArchiveFiles(ctx context.Context, ids []int64, ownerID *int64, reason string) (int64, error) {
	var count int64
	for _, id := range ids {
		if f, ok := m.files[id]; ok {
			if ownerID == nil || *ownerID <= 0 || f.UserID == *ownerID {
				f.Status = compare.FileArchived
				f.ArchiveReason = reason
				count++
			}
		}
	}
	return count, nil
}

func (m *mockCompareRepo) BulkUnarchiveFiles(ctx context.Context, ids []int64, ownerID *int64) (int64, error) {
	var count int64
	for _, id := range ids {
		if f, ok := m.files[id]; ok {
			if ownerID == nil || *ownerID <= 0 || f.UserID == *ownerID {
				f.Status = compare.FileReady
				f.ArchiveReason = ""
				count++
			}
		}
	}
	return count, nil
}

func (m *mockCompareRepo) PurgeExpiredCompareFiles(ctx context.Context, defaultRetentionDays int) (int64, error) {
	var count int64
	cutoff := time.Now().AddDate(0, 0, -defaultRetentionDays)
	for id, f := range m.files {
		if !f.IsTempWarehouse && f.DeletedAt == nil && f.CreatedAt.Before(cutoff) {
			delete(m.files, id)
			delete(m.fileRows, id)
			count++
		}
	}
	return count, nil
}

func (m *mockCompareRepo) AutoArchiveTempWarehouses(ctx context.Context, olderThanHours int) (int64, error) {
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

func (m *mockCompareRepo) PurgeArchivedTempWarehouses(ctx context.Context, olderThanDays int) ([]string, int64, int64, error) {
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

func (m *mockCompareRepo) InsertFileRows(ctx context.Context, rows []*compare.CompareFileRow) error {
	for _, r := range rows {
		m.nextID++
		r.ID = m.nextID
		m.fileRows[r.FileID] = append(m.fileRows[r.FileID], r)
	}
	return nil
}

func (m *mockCompareRepo) ListFileRows(ctx context.Context, fileID int64, limit, offset int) ([]*compare.CompareFileRow, error) {
	return m.fileRows[fileID], nil
}

func (m *mockCompareRepo) GetFileRowsPaginated(ctx context.Context, fileID int64, page, limit int) ([]*compare.CompareFileRow, int64, error) {
	rows := m.fileRows[fileID]
	return rows, int64(len(rows)), nil
}

func (m *mockCompareRepo) DeleteFileRows(ctx context.Context, fileID int64) error {
	delete(m.fileRows, fileID)
	return nil
}

func (m *mockCompareRepo) DeleteFileRowOwnedBy(ctx context.Context, rowID int64, ownerUserID int64) error {
	return nil
}

func (m *mockCompareRepo) DeleteFileRow(ctx context.Context, rowID int64) error {
	for fileID, rows := range m.fileRows {
		for i, r := range rows {
			if r.ID == rowID {
				m.fileRows[fileID] = append(rows[:i], rows[i+1:]...)
				return nil
			}
		}
	}
	return nil
}

func (m *mockCompareRepo) BulkUpdateFileRowMatches(ctx context.Context, fileID int64, matches []compare.RowMatch) error {
	for _, match := range matches {
		if err := m.UpdateFileRowMatch(ctx, match.RowID, match.ProductID, match.Method, match.Confidence); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockCompareRepo) UpdateFileRowMatch(ctx context.Context, rowID int64, matchedProductID *int64, method compare.MatchMethod, confidence float64) error {
	for _, rows := range m.fileRows {
		for _, r := range rows {
			if r.ID == rowID {
				r.MatchedProductID = matchedProductID
				r.MatchMethod = method
				r.MatchConfidence = confidence
				return nil
			}
		}
	}
	return nil
}

func (m *mockCompareRepo) SaveCustomerProductMapping(ctx context.Context, orgID *int64, rawName string, productID int64, source string) error {
	m.savedMappings[rawName] = productID
	return nil
}

func (m *mockCompareRepo) GetSavedProductMapping(ctx context.Context, orgID *int64, rawName string) (*int64, error) {
	if id, ok := m.savedMappings[rawName]; ok {
		return &id, nil
	}
	return nil, nil
}

func (m *mockCompareRepo) FindCandidateProducts(ctx context.Context, orgID *int64, query, sku string, limit int) ([]*compare.CandidateProduct, error) {
	var candidates []*compare.CandidateProduct
	cleanQ := strings.ToLower(strings.TrimSpace(query))
	// Mock master catalog product
	if cleanQ == "" || strings.Contains("panadol extra", cleanQ) || strings.Contains("بانادول اكسترا", cleanQ) {
		candidates = append(candidates, &compare.CandidateProduct{
			ID:             45,
			SKU:            "1001",
			NameAr:         "بانادول اكسترا 24 قرص",
			NameEn:         "Panadol Extra 24 Tab",
			ScientificName: "Paracetamol + Caffeine",
		})
	}
	if cleanQ == "" || strings.Contains("congestal", cleanQ) || strings.Contains("كونجستال", cleanQ) {
		candidates = append(candidates, &compare.CandidateProduct{
			ID:             88,
			SKU:            "1003",
			NameAr:         "كونجستال 20 قرص",
			NameEn:         "Congestal 20 Tab",
			ScientificName: "Paracetamol + Pseudoephedrine",
		})
	}
	return candidates, nil
}

func (m *mockCompareRepo) SearchFileRows(ctx context.Context, userID int64, orgID *int64, query string, limit int) ([]*compare.CompareFileRowWithSupplier, error) {
	var results []*compare.CompareFileRowWithSupplier
	cleanQ := strings.ToLower(strings.TrimSpace(query))

	for fileID, rows := range m.fileRows {
		file, ok := m.files[fileID]
		if !ok || file.DeletedAt != nil || file.Status != compare.FileReady {
			continue
		}
		for _, r := range rows {
			if cleanQ == "" || strings.Contains(strings.ToLower(r.RawName), cleanQ) || strings.Contains(strings.ToLower(r.NormalizedName), cleanQ) || strings.Contains(strings.ToLower(r.SKU), cleanQ) || strings.Contains(strings.ToLower(file.SupplierName), cleanQ) {
				results = append(results, &compare.CompareFileRowWithSupplier{
					CompareFileRow: *r,
					SupplierName:   file.SupplierName,
				})
			}
		}
	}
	return results, nil
}

func (m *mockCompareRepo) ListDistinctSuppliers(ctx context.Context) ([]string, error) {
	var suppliers []string
	seen := make(map[string]bool)
	for _, f := range m.files {
		if f.DeletedAt == nil && f.Status == compare.FileReady && f.IsTempWarehouse && !seen[f.SupplierName] {
			seen[f.SupplierName] = true
			suppliers = append(suppliers, f.SupplierName)
		}
	}
	sort.Strings(suppliers)
	return suppliers, nil
}
