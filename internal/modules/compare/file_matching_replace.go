package compare

import (
	"context"
	"fmt"
)

// MatchAndReplaceFileRows keeps the previous upload intact until every new row
// has a matching decision and the replacement can commit atomically.
func (s *Service) MatchAndReplaceFileRows(ctx context.Context, file *CompareFile, rows []*CompareFileRow) error {
	if file == nil || len(rows) == 0 {
		return fmt.Errorf("compare: empty replacement")
	}
	for _, row := range rows {
		if row == nil || row.ID != 0 || row.FileID != file.ID || !sameFileOrganization(row.OrganizationID, file.OrganizationID) {
			return fmt.Errorf("compare: replacement row belongs to another file or organization")
		}
	}
	if err := s.MatchFileRowsToCatalog(ctx, file.ID, rows); err != nil {
		return err
	}
	file.RowCount, file.ErrorMessage = len(rows), ""
	if file.Status != FileArchived {
		file.Status = FileReady
	}
	return s.repo.ReplaceFileRows(ctx, file, rows)
}

func sameFileOrganization(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// FailFileStaging must not overwrite a completed remap or restore an archived file.
func (s *Service) FailFileStaging(ctx context.Context, file *CompareFile, message string) error {
	return s.repo.FailFileStaging(ctx, file, message)
}
