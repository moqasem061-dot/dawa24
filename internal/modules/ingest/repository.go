package ingest

import (
	"context"
)

// Repository defines storage operations for bulk catalog ingestion.
type Repository interface {
	CreateFileUpload(ctx context.Context, f *FileUpload) error
	GetFileUploadByID(ctx context.Context, id int64) (*FileUpload, error)

	CreateImportSession(ctx context.Context, s *ImportSession) error
	GetImportSessionByID(ctx context.Context, id int64) (*ImportSession, error)
	ListImportSessions(ctx context.Context, orgID int64, limit, offset int) ([]*ImportSession, error)
	UpdateImportSessionProgress(ctx context.Context, id int64, processed, matched int, status SessionStatus, errMsg string) error
	UpdateImportSessionConfig(ctx context.Context, id int64, warehouseID *int64, mode ImportMode, aiMatching, savingsMatching bool) error
	UpdateImportSessionStats(ctx context.Context, id int64, total, processed, matched, review, unmatched int, status SessionStatus, errMsg string) error
	UpdateColumnMapping(ctx context.Context, id int64, mapping map[string]string) error
	UpdateSessionStatus(ctx context.Context, id int64, status SessionStatus) error

	InsertImportRows(ctx context.Context, rows []*ImportRow) error
	ListImportRows(ctx context.Context, sessionID int64, status string, limit, offset int) ([]*ImportRow, error)
	GetImportRowByID(ctx context.Context, id int64) (*ImportRow, error)
	UpdateImportRowMatch(ctx context.Context, rowID int64, matchedProductID *int64, score float64, status string) error
	UpdateImportRowMatchDetailed(ctx context.Context, rowID int64, matchedProductID *int64, score float64, confLevel ConfidenceLevel, reason string, candidates []CandidateMatch, isApproved bool, status string) error
	UpdateImportRowApproval(ctx context.Context, rowID int64, isApproved bool) error
	UpdateImportRowAction(ctx context.Context, rowID int64, action, errorDetails string) error
	BatchUpdateImportRowMatches(ctx context.Context, updates []RowMatchUpdate) error
	BatchUpdateImportRowActions(ctx context.Context, updates []RowActionUpdate) error
}
