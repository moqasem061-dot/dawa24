package integration_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/aicapabilities"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

type blackholeMockIngestRepo struct {
	rows map[int64]*ingest.ImportRow
}

func (m *blackholeMockIngestRepo) CreateFileUpload(_ context.Context, f *ingest.FileUpload) error {
	return nil
}
func (m *blackholeMockIngestRepo) GetFileUploadByID(_ context.Context, id int64) (*ingest.FileUpload, error) {
	return nil, apperr.NotFound("file_upload")
}
func (m *blackholeMockIngestRepo) CreateImportSession(_ context.Context, s *ingest.ImportSession) error {
	return nil
}
func (m *blackholeMockIngestRepo) GetImportSessionByID(_ context.Context, id int64) (*ingest.ImportSession, error) {
	return nil, apperr.NotFound("import_session")
}
func (m *blackholeMockIngestRepo) ListImportSessions(_ context.Context, orgID int64, limit, offset int) ([]*ingest.ImportSession, error) {
	return nil, nil
}
func (m *blackholeMockIngestRepo) UpdateImportSessionProgress(_ context.Context, id int64, processed, matched int, status ingest.SessionStatus, errMsg string) error {
	return nil
}
func (m *blackholeMockIngestRepo) UpdateColumnMapping(_ context.Context, id int64, mapping map[string]string) error {
	return nil
}
func (m *blackholeMockIngestRepo) UpdateSessionStatus(_ context.Context, id int64, status ingest.SessionStatus) error {
	return nil
}
func (m *blackholeMockIngestRepo) InsertImportRows(_ context.Context, rows []*ingest.ImportRow) error {
	return nil
}
func (m *blackholeMockIngestRepo) ListImportRows(_ context.Context, sessionID int64, status string, limit, offset int) ([]*ingest.ImportRow, error) {
	return nil, nil
}
func (m *blackholeMockIngestRepo) GetImportRowByID(_ context.Context, id int64) (*ingest.ImportRow, error) {
	return nil, apperr.NotFound("import_row")
}
func (m *blackholeMockIngestRepo) UpdateImportRowMatch(_ context.Context, rowID int64, matchedProductID *int64, score float64, status string) error {
	if r, ok := m.rows[rowID]; ok {
		r.MatchedProductID = matchedProductID
		r.SimilarityScore = &score
		r.Status = status
	}
	return nil
}
func (m *blackholeMockIngestRepo) UpdateImportSessionConfig(_ context.Context, id int64, warehouseID *int64, mode ingest.ImportMode, aiMatching, savingsMatching bool) error {
	return nil
}
func (m *blackholeMockIngestRepo) UpdateImportSessionStats(_ context.Context, id int64, total, processed, matched, review, unmatched int, status ingest.SessionStatus, errMsg string) error {
	return nil
}
func (m *blackholeMockIngestRepo) UpdateImportRowMatchDetailed(_ context.Context, rowID int64, matchedProductID *int64, score float64, confLevel ingest.ConfidenceLevel, reason string, candidates []ingest.CandidateMatch, isApproved bool, status string) error {
	return nil
}
func (m *blackholeMockIngestRepo) UpdateImportRowApproval(_ context.Context, rowID int64, isApproved bool) error {
	return nil
}
func (m *blackholeMockIngestRepo) UpdateImportRowAction(_ context.Context, rowID int64, action, errorDetails string) error {
	return nil
}
func (m *blackholeMockIngestRepo) BatchUpdateImportRowMatches(_ context.Context, _ []ingest.RowMatchUpdate) error {
	return nil
}
func (m *blackholeMockIngestRepo) BatchUpdateImportRowActions(_ context.Context, _ []ingest.RowActionUpdate) error {
	return nil
}

// A black-holed Gateway must never stop a pharmacy importing its catalogue.
//
// The client is pointed at RFC 5737 TEST-NET-1, which is unroutable: nothing
// answers and nothing refuses, which is the failure mode that hangs a system
// rather than erroring it. Two properties are asserted, and they are the two
// halves of AGENTS.md R3.
//
// This test used to install a per-row AI matcher on the ingest service and
// assert the deterministic path still matched. That matcher is gone — row
// matching is deterministic, and what it cannot settle goes to batched
// adjudication — so the test now exercises the tier that actually dials the
// Gateway. The guarantee it protects is unchanged.
func TestGatewayBlackHoleVerifiesDeterministicFallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	gwCfg := config.Gateway{
		BaseURL:    "http://192.0.2.1:8181",
		VirtualKey: "vk-test-blackhole-key",
		ClientApp:  "dawa24-store-test",
		Timeout:    100 * time.Millisecond,
		Enabled:    true,
	}
	gwClient := gateway.New(gwCfg, logger)

	// One: deterministic matching does not consult the Gateway at all, so an
	// unreachable one cannot affect it.
	row := &ingest.ImportRow{
		ID:             101,
		SessionID:      1,
		OrganizationID: 42,
		RowNumber:      1,
		NormalizedName: "بنادول اكسترا 500 مجم",
		Status:         "pending",
	}
	repo := &blackholeMockIngestRepo{rows: map[int64]*ingest.ImportRow{101: row}}
	ingSvc := ingest.NewService(repo, logger)

	candidates := []ingest.ProductCandidate{
		{ID: 1001, Name: "بنادول اكسترا 500 مجم اقراص"},
		{ID: 1002, Name: "كونجستال اقراص"},
		{ID: 1003, Name: "اوغمنتين 1 جم"},
	}

	matched, matchID, score, err := ingSvc.MatchRowDeterministic(ctx, row, candidates, 0.75)
	if err != nil {
		t.Fatalf("unexpected error during deterministic match: %v", err)
	}
	if !matched || matchID == nil || *matchID != 1001 {
		t.Fatalf("deterministic match failed: matched=%v id=%v", matched, matchID)
	}
	if score < 0.75 {
		t.Fatalf("expected similarity score >= 0.75, got %f", score)
	}

	// Two: the adjudication tier gives up quickly and returns an error the
	// caller can fall back on. Hanging here would hold an import open for as
	// long as the run's own timeout, which is the failure this guards.
	started := time.Now()
	_, aiErr := gateway_test_adjudicate(ctx, gwClient, logger)
	elapsed := time.Since(started)

	if aiErr == nil {
		t.Fatal("a black-holed gateway reported success")
	}
	if elapsed > 4*time.Second {
		t.Fatalf("adjudication took %v against an unreachable gateway; it must fail fast", elapsed)
	}
}

// gateway_test_adjudicate runs one adjudication through the real capability.
//
// It used to go through catalog/gateway's own adjudicator. That prompt is gone —
// the administrator's import asks the same question as the other three, through
// aicapabilities — so this now guards the path that actually runs, which is the
// point of the test rather than a change to it.
func gateway_test_adjudicate(
	ctx context.Context, client gateway.Client, logger *slog.Logger,
) ([]matchflow.Decision, error) {
	caps := aicapabilities.NewService(client, logger)
	return caps.EnhanceMatches(ctx, matchflow.Batch{
		Catalog: []matchflow.CatalogEntry{
			{ProductID: 1001, NameAR: "بنادول اكسترا 500 مجم اقراص"},
		},
		Items: []matchflow.Item{{
			Ref:     0,
			Text:    "بنادول اكسترا 500 مجم",
			Options: []int64{1001},
		}},
	})
}
