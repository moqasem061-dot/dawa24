package compare

import (
	"context"
	"fmt"
	"io"

	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

// ReviewFileColumns retains saved choices; missing required columns must be
// recognized from headers before any existing rows can be replaced.
func (s *Service) ReviewFileColumns(ctx context.Context, fileID int64) error {
	file, err := s.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return err
	}
	if file == nil || file.DeletedAt != nil || file.Status == FileProcessing {
		return fmt.Errorf("compare: file unavailable or still processing")
	}
	if file.MappingConfig.NameCol == nil || (file.MappingConfig.PriceCol == nil && file.MappingConfig.DiscountCol == nil) {
		if err := s.detectReviewColumns(ctx, file); err != nil {
			return err
		}
	}
	return s.processMappedFile(ctx, file)
}

func (s *Service) detectReviewColumns(ctx context.Context, file *CompareFile) error {
	reader := s.openStoredUpload(ctx, file)
	if reader == nil {
		return fmt.Errorf("compare: stored spreadsheet unavailable")
	}
	defer reader.Close()
	contents, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	rows, err := sheet.ReadRows(contents, file.OriginalFilename, sheet.WithRawCellValues())
	if err != nil {
		return err
	}
	if len(rows) < 2 {
		return fmt.Errorf("compare: no usable product rows")
	}
	_, fields, _ := FindBestHeaderRow(rows)
	config := &file.MappingConfig
	for col, field := range fields {
		idx := col
		switch field {
		case FieldProductName:
			if config.NameCol == nil {
				config.NameCol = &idx
			}
		case FieldPrice:
			if config.PriceCol == nil {
				config.PriceCol = &idx
			}
		case FieldDiscount:
			if config.DiscountCol == nil {
				config.DiscountCol = &idx
			}
		case FieldSKU, FieldProductID:
			if config.CodeCol == nil {
				config.CodeCol = &idx
			}
		}
	}
	if config.NameCol == nil || (config.PriceCol == nil && config.DiscountCol == nil) {
		return fmt.Errorf("compare: column mapping needs manual review")
	}
	return nil
}
