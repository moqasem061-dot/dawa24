package ingest

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"log/slog"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
)

// ProductCandidate represents a potential matching product in the master catalogue.
type ProductCandidate struct {
	ID   int64
	Name string
}

// StorageClient provides object storage presigning operations.
type StorageClient interface {
	PresignPut(ctx context.Context, key string, contentType string, lifetime time.Duration) (string, error)
}

// PresignedUpload holds presigned upload credentials for the browser.
type PresignedUpload struct {
	FileUploadID int64  `json:"file_upload_id"`
	UploadURL    string `json:"upload_url"`
	StorageKey   string `json:"storage_key"`
	Method       string `json:"method"`
}

// Service manages vendor bulk catalog file processing and Arabic product matching.
type Service struct {
	repo    Repository
	storage StorageClient
	log     *slog.Logger

	// The rebuilt vendor catalogue import. The three ports are optional: a
	// deployment that has not wired them simply does not offer the screen,
	// which is better than offering one that fails at the last step.
	imports   ImportStore
	catalog   CatalogPort
	inventory InventoryPort
	runs      runRegistry
	// enhancer settles the rows the deterministic engine could not. Optional:
	// unset means the stage is skipped and the import keeps its deterministic
	// answer, which is the same path a disabled gateway takes.
	enhancer Enhancer
	// memory is the shared decision cache. Optional: without it the stage pays
	// for every question instead of reusing answers bought earlier — by this
	// vendor, by another, or by a pharmacy's smart order.
	memory MatchMemory
}

// NewService creates a new ingest service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// SetStorage configures the object storage client.
func (s *Service) SetStorage(storage StorageClient) {
	s.storage = storage
}

// PresignUpload generates a presigned S3/MinIO upload URL and registers the file record.
func (s *Service) PresignUpload(ctx context.Context, userID int64, filename, mimeType string, sizeBytes int64) (*PresignedUpload, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	if s.storage == nil {
		return nil, apperr.Unavailable("storage", nil)
	}

	cleanFilename := filename
	if cleanFilename == "" {
		cleanFilename = "upload.csv"
	}
	key := fmt.Sprintf("orgs/%d/uploads/%d_%s", orgID, time.Now().UnixNano(), cleanFilename)

	uploadURL, err := s.storage.PresignPut(ctx, key, mimeType, 15*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("presign upload: %w", err)
	}

	f := &FileUpload{
		OrganizationID: orgID,
		UserID:         userID,
		Filename:       cleanFilename,
		StorageKey:     key,
		FileSizeBytes:  sizeBytes,
		MimeType:       mimeType,
		CreatedAt:      time.Now().UTC(),
	}

	if err := s.repo.CreateFileUpload(ctx, f); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "presigned upload created", "upload_id", f.ID, "key", key)
	return &PresignedUpload{
		FileUploadID: f.ID,
		UploadURL:    uploadURL,
		StorageKey:   key,
		Method:       "PUT",
	}, nil
}

// RegisterUpload creates a file upload metadata pointer.
func (s *Service) RegisterUpload(ctx context.Context, f *FileUpload) (*FileUpload, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	f.OrganizationID = orgID

	if f.Filename == "" || f.StorageKey == "" {
		return nil, apperr.Validation("upload.invalid", "Filename and storage key are required.", nil)
	}

	if err := s.repo.CreateFileUpload(ctx, f); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "file upload registered", "upload_id", f.ID, "storage_key", f.StorageKey)
	return f, nil
}

// StartSession initiates an ingest session with detected column mappings.
func (s *Service) StartSession(
	ctx context.Context,
	uploadID int64,
	headers []string,
	minScore float64,
) (*ImportSession, error) {
	return s.StartSessionWithConfig(ctx, uploadID, headers, nil, ModeUpdateAndAdd, true, true, minScore)
}

// StartSessionWithConfig creates an import session with target warehouse and matching parameters.
func (s *Service) StartSessionWithConfig(
	ctx context.Context,
	uploadID int64,
	headers []string,
	warehouseID *int64,
	mode ImportMode,
	enableAI, enableSavings bool,
	minScore float64,
) (*ImportSession, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}

	if minScore <= 0 {
		minScore = 0.85
	}
	if mode == "" {
		mode = ModeUpdateAndAdd
	}

	mapping := DetectColumns(headers)

	session := &ImportSession{
		OrganizationID:        orgID,
		FileUploadID:          uploadID,
		WarehouseID:           warehouseID,
		ImportMode:            mode,
		EnableAIMatching:      enableAI,
		EnableSavingsMatching: enableSavings,
		Status:                StatusPending,
		ColumnMapping:         mapping,
		MinSimilarityScore:    minScore,
	}

	if err := s.repo.CreateImportSession(ctx, session); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "import session created", "session_id", session.ID, "warehouse_id", warehouseID, "mode", mode)
	return session, nil
}

// ConfigureSession updates warehouse, import mode, and matching switches for an existing session.
func (s *Service) ConfigureSession(
	ctx context.Context,
	sessionID int64,
	warehouseID *int64,
	mode ImportMode,
	enableAI, enableSavings bool,
) error {
	if mode == "" {
		mode = ModeUpdateAndAdd
	}
	return s.repo.UpdateImportSessionConfig(ctx, sessionID, warehouseID, mode, enableAI, enableSavings)
}

// StageRows inserts raw spreadsheet rows into the staging table with normalized names.
func (s *Service) StageRows(
	ctx context.Context,
	sessionID int64,
	nameColumn string,
	rawRows []map[string]any,
) error {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return database.ErrNoTenant
	}

	var importRows []*ImportRow
	for idx, raw := range rawRows {
		var norm string
		if rawVal, exists := raw[nameColumn]; exists && rawVal != nil {
			norm = arabic.Normalize(fmt.Sprintf("%v", rawVal))
		}

		importRows = append(importRows, &ImportRow{
			SessionID:       sessionID,
			OrganizationID:  orgID,
			RowNumber:       idx + 1,
			RawData:         raw,
			NormalizedName:  norm,
			ConfidenceLevel: ConfidenceUnmatched,
			IsApproved:      true,
			ImportAction:    "pending",
			Status:          "pending",
		})
	}

	return s.repo.InsertImportRows(ctx, importRows)
}

// MatchRowDeterministic satisfies legacy deterministic matching for compatibility.
func (s *Service) MatchRowDeterministic(
	ctx context.Context,
	row *ImportRow,
	candidates []ProductCandidate,
	minScore float64,
) (bool, *int64, float64, error) {
	if row.NormalizedName == "" {
		return false, nil, 0, nil
	}

	var bestMatchID *int64
	var highestScore float64

	for _, cand := range candidates {
		candNorm := arabic.Normalize(cand.Name)
		score := arabic.Similarity(row.NormalizedName, candNorm)
		if score > highestScore {
			highestScore = score
			candID := cand.ID
			bestMatchID = &candID
		}
	}

	// Deterministic only. This used to fall through to a model, one request per
	// row, matching on the *name it returned* rather than on an id — so a
	// reworded answer silently matched nothing and a plausible one could match
	// the wrong product. Batched adjudication against an explicit shortlist
	// replaced it; see catalog_adjudicate.go.
	matched := highestScore >= minScore

	status := "unmatched"
	if matched {
		status = "matched"
	}

	err := s.repo.UpdateImportRowMatch(ctx, row.ID, bestMatchID, highestScore, status)
	return matched, bestMatchID, highestScore, err
}

// GetSessionProgress retrieves the progress and statistics of an import session.
func (s *Service) GetSessionProgress(ctx context.Context, sessionID int64) (*ImportSession, error) {
	return s.repo.GetImportSessionByID(ctx, sessionID)
}

// ListSessions returns import sessions for an organization.
func (s *Service) ListSessions(ctx context.Context, orgID int64, limit, offset int) ([]*ImportSession, error) {
	return s.repo.ListImportSessions(ctx, orgID, limit, offset)
}

// UpdateColumnMapping updates the detected column map.
func (s *Service) UpdateColumnMapping(ctx context.Context, sessionID int64, mapping map[string]string) error {
	return s.repo.UpdateColumnMapping(ctx, sessionID, mapping)
}

// ToggleRowApproval toggles whether a staged row should be included in the commit.
func (s *Service) ToggleRowApproval(ctx context.Context, rowID int64, isApproved bool) error {
	return s.repo.UpdateImportRowApproval(ctx, rowID, isApproved)
}

// OverrideRowMatchDetailed overrides a row's master product match with a manual user selection.
func (s *Service) OverrideRowMatchDetailed(ctx context.Context, rowID int64, matchedProductID int64) error {
	if matchedProductID <= 0 {
		return apperr.Validation("product_id.invalid", "Invalid product ID", nil)
	}
	return s.repo.UpdateImportRowMatchDetailed(
		ctx,
		rowID,
		&matchedProductID,
		1.0,
		ConfidenceHigh,
		i18n.TDefault("w4_mod.100_220"),
		nil,
		true,
		"matched",
	)
}

// OverrideRowMatch sets a manual product match for a staged row (compatibility).
func (s *Service) OverrideRowMatch(ctx context.Context, rowID int64, matchedProductID int64) error {
	return s.OverrideRowMatchDetailed(ctx, rowID, matchedProductID)
}

// GetImportRow returns one staged row.
func (s *Service) GetImportRow(ctx context.Context, rowID int64) (*ImportRow, error) {
	return s.repo.GetImportRowByID(ctx, rowID)
}

// CommitSession finalizes the import session and marks it completed.
func (s *Service) CommitSession(ctx context.Context, sessionID int64) error {
	return s.repo.UpdateSessionStatus(ctx, sessionID, StatusCompleted)
}

// CancelSession marks the session cancelled.
func (s *Service) CancelSession(ctx context.Context, sessionID int64) error {
	return s.repo.UpdateSessionStatus(ctx, sessionID, StatusFailed)
}

// ListImportRows returns staged rows for review.
func (s *Service) ListImportRows(ctx context.Context, sessionID int64, status string, limit, offset int) ([]*ImportRow, error) {
	return s.repo.ListImportRows(ctx, sessionID, status, limit, offset)
}

func getRawString(m map[string]any, key string) string {
	if key == "" || m == nil {
		return ""
	}
	if val, ok := m[key]; ok && val != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", val))
	}
	return ""
}

func getRawStringWithFallback(m map[string]any, colName, targetField string) string {
	if val := getRawString(m, colName); val != "" {
		return val
	}
	if m == nil {
		return ""
	}
	for _, rule := range columnRules {
		if rule.field == targetField {
			for _, ex := range rule.exact {
				for k, v := range m {
					if strings.EqualFold(strings.TrimSpace(k), ex) && v != nil {
						str := strings.TrimSpace(fmt.Sprintf("%v", v))
						if str != "" {
							return str
						}
					}
				}
			}
			for _, st := range rule.strong {
				for k, v := range m {
					if strings.Contains(strings.ToLower(strings.TrimSpace(k)), strings.ToLower(st)) && v != nil {
						str := strings.TrimSpace(fmt.Sprintf("%v", v))
						if str != "" {
							return str
						}
					}
				}
			}
		}
	}
	return ""
}
