package compare

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/arabic"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// extractRowFromRecord extracts a CompareFileRow from a parsed record using the mapping config.
func (s *Service) extractRowFromRecord(record []string, headers []string, file *CompareFile, rowNumber int) *CompareFileRow {
	cfg := file.MappingConfig

	// Extract required fields using mapping config
	getValue := func(col *int) string {
		if col == nil {
			return ""
		}
		if *col >= 0 && *col < len(record) {
			return strings.TrimSpace(record[*col])
		}
		return ""
	}

	name := getValue(cfg.NameCol)
	if name == "" {
		return nil // Skip rows without product name
	}

	price := ParseImportPrice(getValue(cfg.PriceCol))
	// price and price_after_discount are numeric(12,2): ten integer digits.
	// A barcode read as a price would overflow and fail the batch.
	if price.Minor() < 0 || price.Minor() >= maxPriceMinor {
		price = money.Zero
	}

	discount := ParseImportDiscount(getValue(cfg.DiscountCol))

	// Parse code/SKU
	code := getValue(cfg.CodeCol)

	// Calculate price after discount
	priceAfterDiscount := CalculatePriceAfterDiscount(price, discount)

	// Check validity before creating and returning row
	if !IsValidProductRow(name, code, price, discount) {
		return nil
	}

	// Normalize name for matching
	normalizedName := arabic.Normalize(name)
	normalizedName = strings.ToLower(strings.TrimSpace(normalizedName))

	return &CompareFileRow{
		FileID:             file.ID,
		OrganizationID:     file.OrganizationID,
		RowNumber:          rowNumber,
		RawName:            name,
		NormalizedName:     normalizedName,
		SKU:                code,
		Price:              price,
		Discount:           discount,
		PriceAfterDiscount: priceAfterDiscount,
		MatchMethod:        MatchMethodNone,
	}
}

// maxPriceMinor is the exclusive ceiling for a price in minor units, set by
// compare.file_rows.price being numeric(12,2).
const maxPriceMinor int64 = 10_000_000_000

// GetFileRowsPaginated retrieves paginated rows for a specific warehouse file.
func (s *Service) GetFileRowsPaginated(ctx context.Context, fileID int64, page, limit int) ([]*CompareFileRow, int64, error) {
	return s.repo.GetFileRowsPaginated(ctx, fileID, page, limit)
}

// DeleteFileRow deletes a single item from a temporary warehouse and decrements row_count.
func (s *Service) DeleteFileRow(ctx context.Context, rowID int64) error {
	return s.repo.DeleteFileRow(ctx, rowID)
}

// DeleteFileRowOwnedBy deletes a single row only when its parent file belongs to
// ownerUserID. Used by the "my uploaded warehouses" screen.
func (s *Service) DeleteFileRowOwnedBy(ctx context.Context, rowID, ownerUserID int64) error {
	return s.repo.DeleteFileRowOwnedBy(ctx, rowID, ownerUserID)
}

// DeleteFileRows deletes all rows for a compare/warehouse file.
func (s *Service) DeleteFileRows(ctx context.Context, fileID int64) error {
	return s.repo.DeleteFileRows(ctx, fileID)
}

// CreateFile creates a new compare/warehouse file record.
func (s *Service) CreateFile(ctx context.Context, f *CompareFile) error {
	return s.repo.CreateFile(ctx, f)
}

// UpdateFile updates compare/warehouse file metadata.
func (s *Service) UpdateFile(ctx context.Context, f *CompareFile) error {
	return s.repo.UpdateFile(ctx, f)
}

// InsertFileRows inserts rows for a compare/warehouse file.
func (s *Service) InsertFileRows(ctx context.Context, rows []*CompareFileRow) error {
	return s.repo.InsertFileRows(ctx, rows)
}

// PurgeExpiredFiles runs the retention cleanup pass for expired compare files.
func (s *Service) PurgeExpiredFiles(ctx context.Context, defaultRetentionDays int) (int64, error) {
	return s.repo.PurgeExpiredCompareFiles(ctx, defaultRetentionDays)
}

// AutoArchiveTempWarehouses moves active compare files/warehouses older than hours to archived status.
func (s *Service) AutoArchiveTempWarehouses(ctx context.Context, olderThanHours int) (int64, error) {
	return s.repo.AutoArchiveTempWarehouses(ctx, olderThanHours)
}

// PurgeArchivedTempWarehouses purges files and file rows older than days.
func (s *Service) PurgeArchivedTempWarehouses(ctx context.Context, olderThanDays int) ([]string, int64, int64, error) {
	return s.repo.PurgeArchivedTempWarehouses(ctx, olderThanDays)
}

// RunWarehouseLifecycle executes the automated auto-archiving and retention purging cycle.
func (s *Service) RunWarehouseLifecycle(
	ctx context.Context,
	autoArchiveHours int,
	autoArchiveEnabled bool,
	autoDeleteDays int,
	autoDeleteEnabled bool,
	uploadBaseDir string,
) (*TempWarehouseLifecycleResult, error) {
	result := &TempWarehouseLifecycleResult{}

	// 1. Auto-Archive expired active compare files / temp warehouses
	if autoArchiveEnabled && autoArchiveHours > 0 {
		archived, err := s.repo.AutoArchiveTempWarehouses(ctx, autoArchiveHours)
		if err != nil {
			s.log.ErrorContext(ctx, "warehouse lifecycle: auto-archive failed", "error", err, "hours", autoArchiveHours)
			return result, fmt.Errorf("auto-archive: %w", err)
		}
		result.ArchivedFilesCount = archived
		if archived > 0 {
			s.log.InfoContext(ctx, "warehouse lifecycle: auto-archived files", "count", archived, "hours", autoArchiveHours)
		}
	}

	// 2. Auto-Delete / Purge expired archived compare files / temp warehouses
	if autoDeleteEnabled && autoDeleteDays > 0 {
		storageKeys, filesCount, rowsCount, err := s.repo.PurgeArchivedTempWarehouses(ctx, autoDeleteDays)
		if err != nil {
			s.log.ErrorContext(ctx, "warehouse lifecycle: purge failed", "error", err, "days", autoDeleteDays)
			return result, fmt.Errorf("purge: %w", err)
		}
		result.DeletedFilesCount = filesCount
		result.PurgedRowsCount = rowsCount

		// Delete physical files from disk / storage
		var physicalDeleted int64
		for _, key := range storageKeys {
			if key == "" {
				continue
			}
			if uploadBaseDir != "" {
				candidates := []string{
					filepath.Join(uploadBaseDir, "temp_warehouses", filepath.Base(key)),
					filepath.Join(uploadBaseDir, "compare", filepath.Base(key)),
					filepath.Join(uploadBaseDir, filepath.Base(key)),
				}
				for _, p := range candidates {
					if err := os.Remove(p); err == nil { // nolint:errswallow: probing candidate paths; a miss tries the next
						physicalDeleted++
						break
					}
				}
			}
			if s.storage != nil {
				_ = s.storage.Delete(ctx, key)
			}
		}
		result.PurgedStorageFiles = physicalDeleted

		if filesCount > 0 || rowsCount > 0 {
			s.log.InfoContext(ctx, "warehouse lifecycle: purged expired files and rows",
				"files", filesCount, "rows", rowsCount, "physical_files", physicalDeleted, "retention_days", autoDeleteDays)
		}
	}

	return result, nil
}
