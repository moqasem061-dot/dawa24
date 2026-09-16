package ingest

import (
	"context"
	"io"
	"log/slog"
	"sort"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type mockIngestRepo struct {
	uploads  map[int64]*FileUpload
	sessions map[int64]*ImportSession
	rows     map[int64]*ImportRow
	nextID   int64
}

func newMockIngestRepo() *mockIngestRepo {
	return &mockIngestRepo{
		uploads:  map[int64]*FileUpload{},
		sessions: map[int64]*ImportSession{},
		rows:     map[int64]*ImportRow{},
		nextID:   1,
	}
}

func (m *mockIngestRepo) CreateFileUpload(_ context.Context, f *FileUpload) error {
	f.ID = m.nextID
	m.nextID++
	m.uploads[f.ID] = f
	return nil
}

func (m *mockIngestRepo) GetFileUploadByID(_ context.Context, id int64) (*FileUpload, error) {
	f, ok := m.uploads[id]
	if !ok {
		return nil, apperr.NotFound("file_upload")
	}
	return f, nil
}

func (m *mockIngestRepo) CreateImportSession(_ context.Context, s *ImportSession) error {
	s.ID = m.nextID
	m.nextID++
	m.sessions[s.ID] = s
	return nil
}

func (m *mockIngestRepo) GetImportSessionByID(_ context.Context, id int64) (*ImportSession, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, apperr.NotFound("import_session")
	}
	return s, nil
}

func (m *mockIngestRepo) UpdateImportSessionProgress(_ context.Context, id int64, processed, matched int, status SessionStatus, errMsg string) error {
	s, ok := m.sessions[id]
	if !ok {
		return apperr.NotFound("import_session")
	}
	s.ProcessedRows = processed
	s.MatchedRows = matched
	s.Status = status
	s.ErrorMessage = errMsg
	return nil
}

func (m *mockIngestRepo) InsertImportRows(_ context.Context, rows []*ImportRow) error {
	for _, r := range rows {
		r.ID = m.nextID
		m.nextID++
		m.rows[r.ID] = r
	}
	return nil
}

func (m *mockIngestRepo) ListImportSessions(_ context.Context, orgID int64, limit, offset int) ([]*ImportSession, error) {
	var list []*ImportSession
	for _, s := range m.sessions {
		if s.OrganizationID == orgID {
			list = append(list, s)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})
	return list, nil
}

func (m *mockIngestRepo) UpdateColumnMapping(_ context.Context, id int64, mapping map[string]string) error {
	if s, ok := m.sessions[id]; ok {
		s.ColumnMapping = mapping
	}
	return nil
}

func (m *mockIngestRepo) UpdateSessionStatus(_ context.Context, id int64, status SessionStatus) error {
	if s, ok := m.sessions[id]; ok {
		s.Status = status
	}
	return nil
}

func (m *mockIngestRepo) ListImportRows(_ context.Context, sessionID int64, status string, limit, offset int) ([]*ImportRow, error) {
	var list []*ImportRow
	for _, r := range m.rows {
		if r.SessionID == sessionID && (status == "" || r.Status == status) {
			list = append(list, r)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].RowNumber < list[j].RowNumber
	})
	return list, nil
}

func (m *mockIngestRepo) GetImportRowByID(_ context.Context, id int64) (*ImportRow, error) {
	if r, ok := m.rows[id]; ok {
		return r, nil
	}
	return nil, apperr.NotFound("import_row")
}

func (m *mockIngestRepo) UpdateImportSessionConfig(_ context.Context, id int64, warehouseID *int64, mode ImportMode, aiMatching, savingsMatching bool) error {
	if s, ok := m.sessions[id]; ok {
		s.WarehouseID = warehouseID
		s.ImportMode = mode
		s.EnableAIMatching = aiMatching
		s.EnableSavingsMatching = savingsMatching
	}
	return nil
}

func (m *mockIngestRepo) UpdateImportSessionStats(_ context.Context, id int64, total, processed, matched, review, unmatched int, status SessionStatus, errMsg string) error {
	if s, ok := m.sessions[id]; ok {
		s.TotalRows = total
		s.ProcessedRows = processed
		s.MatchedRows = matched
		s.ReviewRows = review
		s.UnmatchedRows = unmatched
		s.Status = status
		s.ErrorMessage = errMsg
	}
	return nil
}

func (m *mockIngestRepo) UpdateImportRowMatch(_ context.Context, rowID int64, matchedProductID *int64, score float64, status string) error {
	r, ok := m.rows[rowID]
	if !ok {
		return apperr.NotFound("import_row")
	}
	r.MatchedProductID = matchedProductID
	r.SimilarityScore = &score
	r.Status = status
	return nil
}

func (m *mockIngestRepo) UpdateImportRowMatchDetailed(_ context.Context, rowID int64, matchedProductID *int64, score float64, confLevel ConfidenceLevel, reason string, candidates []CandidateMatch, isApproved bool, status string) error {
	r, ok := m.rows[rowID]
	if !ok {
		return apperr.NotFound("import_row")
	}
	r.MatchedProductID = matchedProductID
	r.SimilarityScore = &score
	r.ConfidenceLevel = confLevel
	r.MatchReason = reason
	r.CandidateMatches = candidates
	r.IsApproved = isApproved
	r.Status = status
	return nil
}

func (m *mockIngestRepo) UpdateImportRowApproval(_ context.Context, rowID int64, isApproved bool) error {
	if r, ok := m.rows[rowID]; ok {
		r.IsApproved = isApproved
	}
	return nil
}

func (m *mockIngestRepo) UpdateImportRowAction(_ context.Context, rowID int64, action, errorDetails string) error {
	if r, ok := m.rows[rowID]; ok {
		r.ImportAction = action
		r.ErrorDetails = errorDetails
	}
	return nil
}

func (m *mockIngestRepo) BatchUpdateImportRowMatches(_ context.Context, updates []RowMatchUpdate) error {
	for _, u := range updates {
		if r, ok := m.rows[u.RowID]; ok {
			r.MatchedProductID = u.MatchedProductID
			r.SimilarityScore = &u.Score
			r.ConfidenceLevel = u.ConfidenceLevel
			r.MatchReason = u.MatchReason
			r.CandidateMatches = u.Candidates
			r.IsApproved = u.IsApproved
			r.Status = u.Status
		}
	}
	return nil
}

func (m *mockIngestRepo) BatchUpdateImportRowActions(_ context.Context, updates []RowActionUpdate) error {
	for _, u := range updates {
		if r, ok := m.rows[u.RowID]; ok {
			r.ImportAction = u.ImportAction
			r.ErrorDetails = u.ErrorDetails
		}
	}
	return nil
}

func TestColumnDetection(t *testing.T) {
	rawHeaders := []string{
		"اسم الدواء",
		"سعر الجمهور",
		"الرصيد",
		"نسبة الخصم",
		"الباركود",
		"كود الصنف",
	}

	mapping := DetectColumns(rawHeaders)

	expected := map[string]string{
		FieldProductName: "اسم الدواء",
		FieldPrice:       "سعر الجمهور",
		FieldQuantity:    "الرصيد",
		FieldDiscount:    "نسبة الخصم",
		FieldBarcode:     "الباركود",
		FieldSKU:         "كود الصنف",
	}

	for expectedField, header := range expected {
		if mapping[expectedField] != header {
			t.Errorf("target field %q mapped to %q; want %q", expectedField, mapping[expectedField], header)
		}
	}
}

func TestDeterministicProductMatcherAndSessionLifecycle(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 10)
	repo := newMockIngestRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// 1. Register Upload
	upload, err := svc.RegisterUpload(ctx, &FileUpload{
		Filename:      "stock.xlsx",
		StorageKey:    "uploads/stock.xlsx",
		MimeType:      "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		FileSizeBytes: 1024,
	})
	if err != nil {
		t.Fatalf("RegisterUpload failed: %v", err)
	}

	// 2. Start Session
	headers := []string{"اسم الدواء", "سعر الجمهور", "الرصيد"}
	sess, err := svc.StartSession(ctx, upload.ID, headers, 0.85)
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// 3. Stage Rows
	rows := []map[string]any{
		{"name": "بانادول اكسترا اقراص", "price": "25.00", "qty": "100"},
		{"name": "فيتامين سي فوار", "price": "30.00", "qty": "50"},
	}
	err = svc.StageRows(ctx, sess.ID, "name", rows)
	if err != nil {
		t.Fatalf("StageRows failed: %v", err)
	}

	// 4. Match Rows
	candidates := []ProductCandidate{
		{ID: 1001, Name: "بنادول اكسترا اقراص"},
		{ID: 1002, Name: "كتافلام 50 مجم"},
		{ID: 1003, Name: "كونجستال اقراص"},
	}

	stagedRows, err := svc.ListImportRows(ctx, sess.ID, "", 10, 0)
	if err != nil || len(stagedRows) != 2 {
		t.Fatalf("ListImportRows failed: %v", err)
	}

	matched, matchID, score, err := svc.MatchRowDeterministic(ctx, stagedRows[0], candidates, 0.85)
	if err != nil || !matched || matchID == nil || *matchID != 1001 {
		t.Errorf("expected row 0 to match 1001, got matched=%v, matchID=%v, score=%f", matched, matchID, score)
	}

	matched2, _, _, _ := svc.MatchRowDeterministic(ctx, stagedRows[1], candidates, 0.85)
	if matched2 {
		t.Error("expected row 1 to be unmatched")
	}

	// 5. Update Mapping and Session
	newMapping := map[string]string{"اسم الصنف": FieldProductName}
	if err := svc.UpdateColumnMapping(ctx, sess.ID, newMapping); err != nil {
		t.Fatalf("UpdateColumnMapping failed: %v", err)
	}

	gotSess, err := svc.GetSessionProgress(ctx, sess.ID)
	if err != nil || gotSess.ID != sess.ID {
		t.Fatalf("GetSessionProgress failed: %v", err)
	}

	sessions, err := svc.ListSessions(ctx, 10, 10, 0)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("ListSessions failed: %v", err)
	}

	if err := svc.OverrideRowMatch(ctx, stagedRows[1].ID, 1002); err != nil {
		t.Fatalf("OverrideRowMatch failed: %v", err)
	}

	if err := svc.CommitSession(ctx, sess.ID); err != nil {
		t.Fatalf("CommitSession failed: %v", err)
	}
	if err := svc.CancelSession(ctx, sess.ID); err != nil {
		t.Fatalf("CancelSession failed: %v", err)
	}
}
