package ingest

import (
	"context"
	"fmt"
	"io"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

const DefaultBatchSize = 500

// ProgressCallback is invoked periodically during streaming row ingestion.
type ProgressCallback func(ctx context.Context, processedRows int, totalEstimated int)

// ProcessSpreadsheetStream streams rows from an io.Reader (XLSX, XLS, CSV, HTML) in memory-bounded batches of 500.
func (s *Service) ProcessSpreadsheetStream(
	ctx context.Context,
	sessionID int64,
	r io.Reader,
	filename string,
	nameColumn string,
	onProgress ProgressCallback,
) (int, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return 0, database.ErrNoTenant
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return 0, fmt.Errorf("read spreadsheet stream: %w", err)
	}

	allRows, err := sheet.ReadRows(data, filename)
	if err != nil || len(allRows) == 0 {
		return 0, fmt.Errorf("empty or unparseable spreadsheet: %w", err)
	}

	headers := allRows[0]

	// Automatically detect and store column mappings from real file headers
	detected := DetectColumns(headers)
	if len(detected) > 0 {
		_ = s.repo.UpdateColumnMapping(ctx, sessionID, detected)
	}

	if nameColumn == "" || detected[FieldProductName] != "" {
		if prodCol, ok := detected[FieldProductName]; ok && prodCol != "" {
			nameColumn = prodCol
		} else if len(headers) > 0 {
			nameColumn = headers[0]
		}
	}

	var batch []*ImportRow
	totalProcessed := 0
	rowNumber := 1

	for _, record := range allRows[1:] {
		if len(record) == 0 {
			continue
		}
		raw := make(map[string]any, len(headers))
		for i, h := range headers {
			if i < len(record) {
				raw[h] = record[i]
			}
		}

		var norm string
		if nameColumn != "" {
			if val, ok := raw[nameColumn]; ok && val != nil {
				norm = arabic.Normalize(fmt.Sprintf("%v", val))
			}
		}
		if norm == "" {
			for _, h := range headers {
				if val, ok := raw[h]; ok && val != nil {
					valStr := fmt.Sprintf("%v", val)
					if valStr != "" {
						norm = arabic.Normalize(valStr)
						break
					}
				}
			}
		}

		batch = append(batch, &ImportRow{
			SessionID:       sessionID,
			OrganizationID:  orgID,
			RowNumber:       rowNumber,
			RawData:         raw,
			NormalizedName:  norm,
			ConfidenceLevel: ConfidenceUnmatched,
			IsApproved:      true,
			ImportAction:    "pending",
			Status:          "pending",
		})
		rowNumber++

		if len(batch) >= DefaultBatchSize {
			if err := s.repo.InsertImportRows(ctx, batch); err != nil {
				return totalProcessed, fmt.Errorf("insert batch: %w", err)
			}
			totalProcessed += len(batch)
			batch = batch[:0]
			if onProgress != nil {
				onProgress(ctx, totalProcessed, totalProcessed)
			}
		}
	}

	if len(batch) > 0 {
		if err := s.repo.InsertImportRows(ctx, batch); err != nil {
			return totalProcessed, fmt.Errorf("insert final batch: %w", err)
		}
		totalProcessed += len(batch)
		if onProgress != nil {
			onProgress(ctx, totalProcessed, totalProcessed)
		}
	}

	_ = s.repo.UpdateImportSessionStats(ctx, sessionID, totalProcessed, totalProcessed, 0, 0, totalProcessed, StatusPending, "")

	s.log.InfoContext(ctx, "streamed spreadsheet processing complete", "session_id", sessionID, "total_rows", totalProcessed)
	return totalProcessed, nil
}
