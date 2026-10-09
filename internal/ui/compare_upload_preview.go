package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

// Reading the head of an uploaded spreadsheet, for the mapping preview.
//
// Split from compare_upload_handlers.go, which was over the 400-line ceiling.

// loadFileHeadersAndPreview extracts headers and sample preview rows from local disk or database.
func (h *UIHandler) loadFileHeadersAndPreview(ctx context.Context, file *compare.CompareFile) ([]string, [][]string) {
	var headers []string
	var preview [][]string

	if file != nil && file.StorageKey != "" {
		cleanKey := strings.TrimPrefix(filepath.FromSlash(file.StorageKey), string(filepath.Separator))
		candidates := []string{
			file.StorageKey,
			filepath.Join("data", cleanKey),
			filepath.Join(UploadBaseDir, "temp_warehouses", filepath.Base(file.StorageKey)),
			filepath.Join("data", "uploads", "temp_warehouses", filepath.Base(file.StorageKey)),
			filepath.Join(UploadBaseDir, "temp_warehouses", filepath.Base(file.OriginalFilename)),
			filepath.Join("data", "uploads", "temp_warehouses", filepath.Base(file.OriginalFilename)),
			filepath.Join(UploadBaseDir, filepath.FromSlash(strings.TrimPrefix(file.StorageKey, "/uploads/"))),
			filepath.Join(UploadBaseDir, "compare", filepath.Base(file.StorageKey)),
			filepath.Join("data", "uploads", "compare", filepath.Base(file.StorageKey)),
			filepath.Join(UploadBaseDir, "compare", filepath.Base(file.OriginalFilename)),
			filepath.Join("data", "uploads", "compare", filepath.Base(file.OriginalFilename)),
			"data" + file.StorageKey,
		}
		for _, cand := range candidates {
			if f, err := os.Open(cand); err == nil { // nolint:errswallow: probing candidate paths; a miss tries the next
				if loaded, loaded2, err := h.parseFilePreview(f, file.OriginalFilename); err != nil {
					h.logLoadErr(ctx, "parseFilePreview", err)
				} else {
					headers, preview = loaded, loaded2
				}
				f.Close()
				if len(headers) > 0 {
					break
				}
			}
		}
	}

	// Fallback to already extracted rows in DB if disk read didn't populate headers
	if len(headers) == 0 && file != nil && h.compareSvc != nil {
		if dbRows, loadErr := h.compareSvc.ListFileRows(ctx, file.ID, 5, 0); loadErr != nil {
			h.logLoadErr(ctx, "compareSvc.ListFileRows", loadErr)
		} else if len(dbRows) > 0 {
			headers = []string{i18n.T("ar", "compare.col.sku"), i18n.T("ar", "compare.col.name"), i18n.T("ar", "compare.col.price"), i18n.T("ar", "compare.col.discount")}
			for _, dr := range dbRows {
				preview = append(preview, []string{
					dr.SKU, dr.RawName, dr.Price.String(), fmt.Sprintf("%.1f%%", dr.Discount),
				})
			}
		}
	}

	if len(headers) == 0 {
		headers = []string{i18n.T("ar", "compare.col.sku"), i18n.T("ar", "compare.col.name"), i18n.T("ar", "compare.col.price"), i18n.T("ar", "compare.col.discount"), i18n.T("ar", "compare.col.notes")}
	}

	return headers, preview
}

// parseFilePreview reads the first few rows of a spreadsheet for mapping preview.
func (h *UIHandler) parseFilePreview(reader io.Reader, filename string) ([]string, [][]string, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, nil, err
	}
	book, err := sheet.Open(data, filename, sheet.WithRawCellValues())
	if err != nil {
		return nil, nil, err
	}
	defer book.Close()
	sample, err := book.Peek(200)
	if err != nil {
		return nil, nil, err
	}
	allRows := sample.Rows
	if len(allRows) == 0 {
		return nil, nil, fmt.Errorf("empty spreadsheet")
	}

	headerRowIdx, _, _ := compare.FindBestHeaderRow(allRows)
	headers := allRows[headerRowIdx]

	var preview [][]string
	for i := headerRowIdx + 1; i < len(allRows) && len(preview) < 5; i++ {
		if len(allRows[i]) > 0 {
			preview = append(preview, allRows[i])
		}
	}
	return headers, preview, nil
}
