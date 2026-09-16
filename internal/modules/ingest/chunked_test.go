package ingest_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

type mockIngestRepo struct {
	uploads  map[int64]*ingest.FileUpload
	sessions map[int64]*ingest.ImportSession
	rows     []*ingest.ImportRow
	nextID   int64
}

func newMockIngestRepo() *mockIngestRepo {
	return &mockIngestRepo{
		uploads:  make(map[int64]*ingest.FileUpload),
		sessions: make(map[int64]*ingest.ImportSession),
		nextID:   1,
	}
}

func (m *mockIngestRepo) CreateFileUpload(ctx context.Context, f *ingest.FileUpload) error {
	f.ID = m.nextID
	m.nextID++
	f.PublicID = fmt.Sprintf("upload-%d", f.ID)
	m.uploads[f.ID] = f
	return nil
}

func (m *mockIngestRepo) GetFileUploadByID(ctx context.Context, id int64) (*ingest.FileUpload, error) {
	if f, ok := m.uploads[id]; ok {
		return f, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockIngestRepo) CreateImportSession(ctx context.Context, s *ingest.ImportSession) error {
	s.ID = m.nextID
	m.nextID++
	m.sessions[s.ID] = s
	return nil
}

func (m *mockIngestRepo) GetImportSessionByID(ctx context.Context, id int64) (*ingest.ImportSession, error) {
	if s, ok := m.sessions[id]; ok {
		return s, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockIngestRepo) UpdateSessionStatus(ctx context.Context, id int64, status ingest.SessionStatus) error {
	if s, ok := m.sessions[id]; ok {
		s.Status = status
		return nil
	}
	return fmt.Errorf("not found")
}

func (m *mockIngestRepo) UpdateColumnMapping(ctx context.Context, id int64, mapping map[string]string) error {
	if s, ok := m.sessions[id]; ok {
		s.ColumnMapping = mapping
		return nil
	}
	return fmt.Errorf("not found")
}

func (m *mockIngestRepo) ListImportSessions(ctx context.Context, orgID int64, limit, offset int) ([]*ingest.ImportSession, error) {
	var list []*ingest.ImportSession
	for _, s := range m.sessions {
		if s.OrganizationID == orgID {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockIngestRepo) UpdateImportSessionProgress(ctx context.Context, id int64, processed, matched int, status ingest.SessionStatus, errMsg string) error {
	if s, ok := m.sessions[id]; ok {
		s.ProcessedRows = processed
		s.MatchedRows = matched
		s.Status = status
		s.ErrorMessage = errMsg
		return nil
	}
	return fmt.Errorf("not found")
}

func (m *mockIngestRepo) InsertImportRows(ctx context.Context, rows []*ingest.ImportRow) error {
	m.rows = append(m.rows, rows...)
	return nil
}

func (m *mockIngestRepo) ListImportRows(ctx context.Context, sessionID int64, status string, limit, offset int) ([]*ingest.ImportRow, error) {
	var list []*ingest.ImportRow
	for _, r := range m.rows {
		if r.SessionID == sessionID {
			list = append(list, r)
		}
	}
	return list, nil
}

func (m *mockIngestRepo) GetImportRowByID(ctx context.Context, id int64) (*ingest.ImportRow, error) {
	for _, r := range m.rows {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockIngestRepo) UpdateImportRowMatch(ctx context.Context, rowID int64, matchedProductID *int64, score float64, status string) error {
	return nil
}

func (m *mockIngestRepo) UpdateImportSessionConfig(ctx context.Context, id int64, warehouseID *int64, mode ingest.ImportMode, aiMatching, savingsMatching bool) error {
	return nil
}

func (m *mockIngestRepo) UpdateImportSessionStats(ctx context.Context, id int64, total, processed, matched, review, unmatched int, status ingest.SessionStatus, errMsg string) error {
	return nil
}

func (m *mockIngestRepo) UpdateImportRowMatchDetailed(ctx context.Context, rowID int64, matchedProductID *int64, score float64, confLevel ingest.ConfidenceLevel, reason string, candidates []ingest.CandidateMatch, isApproved bool, status string) error {
	return nil
}

func (m *mockIngestRepo) UpdateImportRowApproval(ctx context.Context, rowID int64, isApproved bool) error {
	return nil
}

func (m *mockIngestRepo) UpdateImportRowAction(ctx context.Context, rowID int64, action, errorDetails string) error {
	return nil
}

func (m *mockIngestRepo) BatchUpdateImportRowMatches(ctx context.Context, updates []ingest.RowMatchUpdate) error {
	return nil
}

func (m *mockIngestRepo) BatchUpdateImportRowActions(ctx context.Context, updates []ingest.RowActionUpdate) error {
	return nil
}

func TestChunkedUploadFullFlow(t *testing.T) {
	repo := newMockIngestRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := ingest.NewService(repo, logger)

	ctx := database.WithTenant(context.Background(), 10)
	userID := int64(100)
	fileUUID := "test-uuid-12345"
	filename := "catalog_products.csv"

	chunk0 := []byte("اسم الصنف,السعر,الكمية\nبانادول إكسترا,50.00,100\n")
	chunk1 := []byte("كونجستال أقراص,30.00,50\n")

	// Step 1: Upload Chunk 0
	res0, err := svc.UploadChunk(ctx, userID, fileUUID, filename, 0, 2, chunk0)
	if err != nil {
		t.Fatalf("UploadChunk 0 failed: %v", err)
	}
	if res0.Completed {
		t.Errorf("expected chunk 0 to be incomplete")
	}

	// Verify Chunk Status
	status, err := svc.GetChunkStatus(ctx, fileUUID)
	if err != nil || len(status) != 1 || status[0] != 0 {
		t.Fatalf("unexpected chunk status: %v", status)
	}

	// Step 2: Re-send Chunk 0 (idempotency check)
	_, err = svc.UploadChunk(ctx, userID, fileUUID, filename, 0, 2, chunk0)
	if err != nil {
		t.Fatalf("idempotent chunk 0 re-upload failed: %v", err)
	}

	// Step 3: Upload Chunk 1 (final)
	res1, err := svc.UploadChunk(ctx, userID, fileUUID, filename, 1, 2, chunk1)
	if err != nil {
		t.Fatalf("UploadChunk 1 failed: %v", err)
	}
	if !res1.Completed {
		t.Errorf("expected completed upload after chunk 1")
	}
	if res1.FileUploadID <= 0 {
		t.Errorf("expected file upload ID to be generated")
	}
}

func TestProcessSpreadsheetStreamCSV(t *testing.T) {
	repo := newMockIngestRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := ingest.NewService(repo, logger)

	ctx := database.WithTenant(context.Background(), 10)
	sessionID := int64(1)

	var sb strings.Builder
	sb.WriteString("اسم الصنف,السعر,الكمية\n")
	for i := 1; i <= 1200; i++ {
		sb.WriteString(fmt.Sprintf("صنف رقم %d,100.00,10\n", i))
	}

	progressCalls := 0
	totalProcessed, err := svc.ProcessSpreadsheetStream(ctx, sessionID, strings.NewReader(sb.String()), "items.csv", "اسم الصنف", func(ctx context.Context, processedRows, totalEstimated int) {
		progressCalls++
	})

	if err != nil {
		t.Fatalf("ProcessSpreadsheetStream failed: %v", err)
	}
	if totalProcessed != 1200 {
		t.Errorf("expected 1200 rows, got %d", totalProcessed)
	}
	if len(repo.rows) != 1200 {
		t.Errorf("expected repo to have 1200 staged rows, got %d", len(repo.rows))
	}
	if progressCalls < 3 {
		t.Errorf("expected at least 3 progress callbacks (for 500, 1000, 1200 rows), got %d", progressCalls)
	}
}
