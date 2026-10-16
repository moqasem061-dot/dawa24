package compare

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/filesecurity"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

// RenameFile updates the user-assigned supplier label for a file.
func (s *Service) RenameFile(ctx context.Context, fileID int64, newSupplierName string) error {
	newSupplierName = strings.TrimSpace(newSupplierName)
	if newSupplierName == "" {
		return apperr.Validation("file.supplier_name_required", "Supplier name cannot be empty.", nil)
	}
	return s.repo.RenameFile(ctx, fileID, newSupplierName)
}

// ArchiveFile manually archives a file.
func (s *Service) ArchiveFile(ctx context.Context, fileID int64, reason string) error {
	if reason == "" {
		reason = i18n.T("ar", "err.manual_archive_reason")
	}
	return s.repo.ArchiveFile(ctx, fileID, reason)
}

// ArchiveActiveFiles soft-archives all currently active compare files for a user/organization.
func (s *Service) ArchiveActiveFiles(ctx context.Context, userID int64, orgID *int64, reason string) ([]string, error) {
	if reason == "" {
		reason = "تمت الأرشفة تلقائياً لاستبدالها بملفات مقارنة جديدة"
	}
	return s.repo.ArchiveActiveFiles(ctx, userID, orgID, reason)
}

// UnarchiveFile restores an archived file.
func (s *Service) UnarchiveFile(ctx context.Context, fileID int64) error {
	return s.repo.UnarchiveFile(ctx, fileID)
}

// DeleteFile soft-deletes a file.
func (s *Service) DeleteFile(ctx context.Context, fileID int64) error {
	return s.repo.DeleteFile(ctx, fileID)
}

// BulkDeleteFiles deletes multiple files in an atomic query.
func (s *Service) BulkDeleteFiles(ctx context.Context, ids []int64, ownerID *int64) (int64, error) {
	return s.repo.BulkDeleteFiles(ctx, ids, ownerID)
}

// BulkArchiveFiles archives multiple files in an atomic query.
func (s *Service) BulkArchiveFiles(ctx context.Context, ids []int64, ownerID *int64, reason string) (int64, error) {
	return s.repo.BulkArchiveFiles(ctx, ids, ownerID, reason)
}

// BulkUnarchiveFiles unarchives multiple files in an atomic query.
func (s *Service) BulkUnarchiveFiles(ctx context.Context, ids []int64, ownerID *int64) (int64, error) {
	return s.repo.BulkUnarchiveFiles(ctx, ids, ownerID)
}

// ListFiles lists files for the given tenant / user.
func (s *Service) ListFiles(ctx context.Context, userID int64, orgID *int64, status *CompareFileStatus) ([]*CompareFile, error) {
	return s.repo.ListFiles(ctx, userID, orgID, status)
}

// ListAllFiles lists all compare files across the system with optional search and status filter.
func (s *Service) ListAllFiles(ctx context.Context, search string, status *CompareFileStatus) ([]*CompareFile, error) {
	return s.repo.ListAllFiles(ctx, search, status)
}

// ListAdminTempWarehouses lists moderator temporary warehouses plus vendor
// compare-tool files, enriched with uploader / vendor labels, for the admin
// oversight and "my uploads" pages.
func (s *Service) ListAdminTempWarehouses(ctx context.Context, filter AdminTempWarehouseFilter) ([]*AdminTempWarehouse, error) {
	return s.repo.ListAdminTempWarehouses(ctx, filter)
}

// ListAdminTempWarehousesWithTotal returns paginated temporary warehouses and total count.
func (s *Service) ListAdminTempWarehousesWithTotal(ctx context.Context, filter AdminTempWarehouseFilter, limit, offset int) ([]*AdminTempWarehouse, int, error) {
	return s.repo.ListAdminTempWarehousesWithTotal(ctx, filter, limit, offset)
}

// AdminTempWarehouseStats aggregates total rows, active count, and archived count for temp warehouses.
func (s *Service) AdminTempWarehouseStats(ctx context.Context, filter AdminTempWarehouseFilter) (totalRows int64, activeCount, archivedCount int, err error) {
	return s.repo.AdminTempWarehouseStats(ctx, filter)
}

// ListTempWarehouseUploaders returns the distinct uploaders for the admin
// listing's filter dropdown.
func (s *Service) ListTempWarehouseUploaders(ctx context.Context) ([]FileUploader, error) {
	return s.repo.ListTempWarehouseUploaders(ctx)
}

// GetFile retrieves a file by ID.
func (s *Service) GetFile(ctx context.Context, fileID int64) (*CompareFile, error) {
	return s.repo.GetFileByID(ctx, fileID)
}

// GetFileByPublicID retrieves a file by public UUID.
func (s *Service) GetFileByPublicID(ctx context.Context, publicID string) (*CompareFile, error) {
	return s.repo.GetFileByPublicID(ctx, publicID)
}

// ListFileRows retrieves extracted rows for a compare file.
func (s *Service) ListFileRows(ctx context.Context, fileID int64, limit, offset int) ([]*CompareFileRow, error) {
	return s.repo.ListFileRows(ctx, fileID, limit, offset)
}

// SaveFileMapping validates and persists user-defined column mapping for a spreadsheet (Plan V5 Phase 2 §2.3.3).
// After saving the mapping, it automatically processes the file to extract rows.
func (s *Service) SaveFileMapping(ctx context.Context, fileID int64, config MappingConfig) error {
	if config.NameCol == nil {
		return apperr.Validation("mapping.name_required", i18n.T("ar", "err.name_col_required"), map[string]string{
			"name_col": i18n.T("ar", "err.name_col_missing"),
		})
	}
	if config.PriceCol == nil && config.DiscountCol == nil {
		return apperr.Validation("mapping.price_or_discount_required", i18n.TDefault("w4_mod.w4str_165_165"), map[string]string{
			"price_col": i18n.TDefault("w4_mod.s_374_374"),
		})
	}

	file, err := s.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return err
	}

	// Fast path: if the file is already ready with extracted rows, and the mapping hasn't changed,
	// avoid re-reading the whole spreadsheet from disk, re-parsing tens of thousands of rows,
	// and re-running catalog matching synchronously.
	if file.Status == FileReady && file.RowCount > 0 && mappingConfigEqual(file.MappingConfig, config) {
		return nil
	}

	file.MappingConfig = config
	return s.processMappedFile(ctx, file)
}

func mappingConfigEqual(a, b MappingConfig) bool {
	ptrEq := func(x, y *int) bool {
		if x == nil && y == nil {
			return true
		}
		if x == nil || y == nil {
			return false
		}
		return *x == *y
	}
	return ptrEq(a.NameCol, b.NameCol) &&
		ptrEq(a.PriceCol, b.PriceCol) &&
		ptrEq(a.DiscountCol, b.DiscountCol) &&
		ptrEq(a.CodeCol, b.CodeCol)
}

// UploadAndProcessCompareFile uploads a spreadsheet, detects columns, extracts rows, and stores them in compare.file_rows immediately.
func (s *Service) UploadAndProcessCompareFile(
	ctx context.Context, userID int64, orgID *int64, supplierName, originalFilename, mimeType string,
	sizeBytes int64, storageKey string, fileBytes []byte,
) (*CompareFile, []string, error) {
	if len(fileBytes) > 0 {
		if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, originalFilename); err != nil {
			return nil, nil, err
		}
	}

	file, archived, err := s.UploadCompareFile(ctx, userID, orgID, supplierName, originalFilename, mimeType, sizeBytes, storageKey)
	if err != nil {
		return nil, nil, err
	}

	if len(fileBytes) == 0 {
		return file, archived, nil
	}

	if err := s.StageUploadedRows(ctx, file, fileBytes); err != nil {
		return file, archived, err
	}
	return file, archived, nil
}

// StageUploadedRows reads a freshly uploaded spreadsheet and stores its rows.
//
// Split out of UploadAndProcessCompareFile so the same work can run detached
// from the request that uploaded the file — see RegisterAndStage. Everything
// below this line used to be inline there, and is unchanged: it detects the
// header row, auto-maps the columns, extracts the rows and records the outcome
// on the file.
func (s *Service) StageUploadedRows(ctx context.Context, file *CompareFile, fileBytes []byte) error {
	// 1. Read all rows from file using universal spreadsheet reader
	allRows, err := sheet.ReadRows(fileBytes, file.OriginalFilename, sheet.WithRawCellValues())
	if err != nil || len(allRows) == 0 {
		processing := file.Status == FileProcessing
		file.ErrorMessage = i18n.TDefault("w4_mod.s_375_375")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			file.ErrorMessage = filesecurity.SecurityErrorMessage
		}
		if processing {
			s.failStaging(ctx, file, file.ErrorMessage)
		} else {
			file.Status = FileFailed
			if updateErr := s.repo.UpdateFile(ctx, file); updateErr != nil {
				return fmt.Errorf("compare: record parse failure: %w", updateErr)
			}
		}
		file.Status = FileFailed
		return err
	}

	// 2. Find best header row and detect columns
	headerRowIdx, fieldMapping, _ := FindBestHeaderRow(allRows)
	var config MappingConfig
	colMapping := make(map[TargetField]*int)
	for colIdx, field := range fieldMapping {
		idx := colIdx
		colMapping[field] = &idx
	}
	config.NameCol = colMapping[FieldProductName]
	config.PriceCol = colMapping[FieldPrice]
	config.DiscountCol = colMapping[FieldDiscount]
	config.CodeCol = colMapping[FieldSKU]
	if config.CodeCol == nil {
		config.CodeCol = colMapping[FieldProductID]
	}

	// Heuristic fallbacks if columns were not auto-detected
	headerRow := allRows[headerRowIdx]
	if config.NameCol == nil && len(headerRow) > 0 {
		idx := 0
		if len(headerRow) > 1 {
			idx = 1
		}
		config.NameCol = &idx
	}
	if config.PriceCol == nil && len(headerRow) > 2 {
		idx := 2
		config.PriceCol = &idx
	}
	if config.DiscountCol == nil && len(headerRow) > 3 {
		idx := 3
		config.DiscountCol = &idx
	}
	if config.CodeCol == nil && len(headerRow) > 0 {
		idx := 0
		config.CodeCol = &idx
	}

	file.MappingConfig = config

	// 3. Extract and insert rows
	var rows []*CompareFileRow
	rowNum := 1
	for i := headerRowIdx + 1; i < len(allRows); i++ {
		record := allRows[i]
		if len(record) == 0 {
			continue
		}
		row := s.extractRowFromRecord(record, headerRow, file, rowNum)
		if row != nil {
			rows = append(rows, row)
			rowNum++
		}
	}

	if len(rows) == 0 {
		return fmt.Errorf("compare: no usable product rows")
	}
	return s.MatchAndReplaceFileRows(ctx, file, rows)
}

// ProcessCompareFile downloads the uploaded spreadsheet from storage or local disk, parses it using the
// saved column mapping, extracts rows, and inserts them into compare.file_rows.
func (s *Service) ProcessCompareFile(ctx context.Context, fileID int64) error {
	file, err := s.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return err
	}

	return s.processMappedFile(ctx, file)
}

func (s *Service) processMappedFile(ctx context.Context, file *CompareFile) error {
	fileID := file.ID

	if file.MappingConfig.NameCol == nil {
		return apperr.Validation("compare.no_mapping", "Column mapping not configured for this file.", nil)
	}

	reader := s.openStoredUpload(ctx, file)

	if reader == nil {
		return apperr.Internal(fmt.Errorf("stored spreadsheet unavailable for file %d", fileID))
	}
	defer reader.Close()

	// Parse the spreadsheet by content, not by extension. Suppliers send
	// legacy BIFF .xls, HTML tables named .xls, and CSVs named .xlsx; the
	// universal reader sniffs the real container, where excelize alone rejects
	// everything that is not a true .xlsx.
	rows, err := s.parseSpreadsheet(reader, file)
	if err != nil {

		return fmt.Errorf("parse spreadsheet: %w", err)
	}

	if len(rows) == 0 {
		return fmt.Errorf("compare: no usable product rows")
	}
	return s.MatchAndReplaceFileRows(ctx, file, rows)
}

// parseSpreadsheet parses any supported workbook - .xlsx, legacy .xls, the XML
// 2003 dialect, an HTML table, or a delimited text file - using the column
// mapping saved for the file.
func (s *Service) parseSpreadsheet(reader io.Reader, file *CompareFile) ([]*CompareFileRow, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read spreadsheet data: %w", err)
	}

	allRows, err := sheet.ReadRows(data, file.OriginalFilename, sheet.WithRawCellValues())
	if err != nil {
		return nil, fmt.Errorf("read spreadsheet: %w", err)
	}
	if len(allRows) == 0 {
		return nil, fmt.Errorf("empty sheet")
	}

	headerRowIdx, _, _ := FindBestHeaderRow(allRows)
	dataStart := headerRowIdx + 1
	if file.IsTempWarehouse {
		layout, _ := productmatch.AnalyzeLayout(allRows)
		if layout.HeaderRow >= 0 && layout.FirstDataRow > 0 && layout.FirstDataRow <= len(allRows) {
			headerRowIdx, dataStart = layout.HeaderRow, layout.FirstDataRow
		}
	}
	headers := allRows[headerRowIdx]

	var rows []*CompareFileRow
	rowNumber := 1

	for i := dataStart; i < len(allRows); i++ {
		columns := allRows[i]
		if len(columns) == 0 {
			continue
		}

		row := s.extractRowFromRecord(columns, headers, file, rowNumber)
		if row != nil {
			rows = append(rows, row)
			rowNumber++
		}
	}

	return rows, nil
}
