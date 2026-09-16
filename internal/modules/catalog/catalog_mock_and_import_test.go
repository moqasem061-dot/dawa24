package catalog_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func (mockCatalogRepoStub) ListAllSavingProductsAdmin(context.Context, *int64, *int64, string, string, int, int) ([]*catalog.SavingProductAdminView, *catalog.SavingProductAdminStats, error) {
	return nil, nil, nil
}
func (mockCatalogRepoStub) GetSavingProductByID(context.Context, int64) (*catalog.SavingProduct, error) {
	return nil, nil
}
func (mockCatalogRepoStub) DeleteSavingProduct(context.Context, int64, int64) error { return nil }
func (mockCatalogRepoStub) DeleteAllSavingProducts(context.Context, int64) error    { return nil }
func (mockCatalogRepoStub) GetProductProviders(context.Context, int64) ([]*catalog.ProductProviderInfo, error) {
	return nil, nil
}
func (mockCatalogRepoStub) BatchUpsertSavingProducts(context.Context, int64, *int64, []*catalog.SavingProduct) (int, int, error) {
	return 0, 0, nil
}
func (mockCatalogRepoStub) ListAllMasterProductsForMatching(context.Context) ([]*catalog.CatalogMatchSource, error) {
	return nil, nil
}
func (mockCatalogRepoStub) DeleteAllVariantsByOrg(context.Context, int64) (int64, error) {
	return 0, nil
}
func (mockCatalogRepoStub) ActivateAllVariantsByOrg(context.Context, int64) (int64, error) {
	return 0, nil
}
func (mockCatalogRepoStub) DeleteAllProducts(context.Context) (int64, error) {
	return 0, nil
}
func (mockCatalogRepoStub) GetProductBySKU(context.Context, string) (*catalog.Product, error) {
	return nil, nil
}
func (mockCatalogRepoStub) GetProductByBarcode(context.Context, string) (*catalog.Product, error) {
	return nil, nil
}
func (mockCatalogRepoStub) UpdateProductImageBySKU(context.Context, string, string, string) (*catalog.Product, error) {
	return nil, nil
}
func (mockCatalogRepoStub) UpdateProductImageByID(context.Context, int64, string, string) (*catalog.Product, error) {
	return nil, nil
}
func (mockCatalogRepoStub) ListDosageForms(context.Context) ([]string, error) {
	return []string{"tablet", "capsule", "syrup"}, nil
}
func (mockCatalogRepoStub) ListMatchDecisions(context.Context, string, int, int) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (mockCatalogRepoStub) ListMatchDecisionsFiltered(context.Context, catalog.DecisionMemoryFilter) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (mockCatalogRepoStub) ListMatchDecisionsForOrgWithPlatform(context.Context, int64, string, int, int) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (mockCatalogRepoStub) GetDecisionMemoryPreference(context.Context, int64) (bool, error) {
	return true, nil
}
func (mockCatalogRepoStub) SetDecisionMemoryPreference(context.Context, int64, bool, int64) error {
	return nil
}
func (mockCatalogRepoStub) PromoteMatchDecision(context.Context, int64, int64) error {
	return nil
}
func (mockCatalogRepoStub) DemoteMatchDecision(context.Context, int64) error {
	return nil
}
func (mockCatalogRepoStub) BulkPromoteMatchDecisions(context.Context, []int64, int64) (int64, error) {
	return 0, nil
}
func (mockCatalogRepoStub) BulkDeleteMatchDecisions(context.Context, []int64) (int64, error) {
	return 0, nil
}
func (mockCatalogRepoStub) RelinkMatchDecision(context.Context, int64, *int64, int64) error {
	return nil
}
func (mockCatalogRepoStub) RelinkMatchDecisionForOrg(context.Context, int64, int64, *int64, int64) error {
	return nil
}
func (mockCatalogRepoStub) DeleteMatchDecision(context.Context, int64) error {
	return nil
}
func (mockCatalogRepoStub) ClearMatchDecisions(context.Context) error {
	return nil
}
func (mockCatalogRepoStub) ListMatchDecisionsForOrg(context.Context, int64, string, int, int) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (mockCatalogRepoStub) DeleteMatchDecisionForOrg(context.Context, int64, int64) error {
	return nil
}
func (mockCatalogRepoStub) ClearMatchDecisionsForOrg(context.Context, int64) error {
	return nil
}
func (mockCatalogRepoStub) SaveManualDecision(context.Context, int64, int64, string, int64, string) error {
	return nil
}
func (mockCatalogRepoStub) IsDecisionMemoryEnabled(context.Context) bool {
	return true
}
func (mockCatalogRepoStub) SetDecisionMemoryEnabled(context.Context, bool) error {
	return nil
}
func (mockCatalogRepoStub) ListCustomerMappings(context.Context, int64, string, int, int) ([]*catalog.CustomerMappingView, int, error) {
	return nil, 0, nil
}
func (mockCatalogRepoStub) DeleteCustomerMapping(context.Context, int64, int64) error {
	return nil
}
func (mockCatalogRepoStub) ClearCustomerMappings(context.Context, int64) error {
	return nil
}
func (mockCatalogRepoStub) ListBuyerOffers(context.Context, catalog.BuyerOfferQuery) ([]*catalog.BuyerOffer, int, error) {
	return nil, 0, nil
}

// stagingRepo is the minimal catalog.Repository the commit path needs.
func (*stagingRepo) GetVariantsByIDs(context.Context, []int64) (map[int64]*catalog.ProductVariant, error) {
	return map[int64]*catalog.ProductVariant{}, nil
}

type stagingRepo struct {
	mockCatalogRepoStub
	written []*catalog.Product
}

func (r *stagingRepo) BulkUpsertProducts(
	_ context.Context, prods []*catalog.Product, _ catalog.BulkWriteOptions,
) (catalog.BulkWriteResult, error) {
	r.written = append(r.written, prods...)
	res := catalog.BulkWriteResult{Matches: map[int]catalog.MatchReason{}}
	for _, p := range prods {
		if p.ID > 0 {
			res.Updated++
			continue
		}
		res.Inserted++
	}
	return res, nil
}

func newImportService(t *testing.T, store *memoryImportStore) (*catalog.Service, *stagingRepo) {
	t.Helper()
	repo := &stagingRepo{}
	svc := catalog.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.SetImportStore(store)
	return svc, repo
}

const serviceFixture = "اسم الصنف,كود الصنف,سعر البيع\n" +
	"بانادول اكسترا,SVC-1,55.00\n" +
	"كتافلام أقراص,SVC-2,42.00\n" +
	"اوجمنتين حقن,SVC-3,115.00\n"

// Analysing and preparing must leave the catalogue untouched. That property is
// the entire justification for the review step.
func TestPrepareImportWritesNothingToTheCatalogue(t *testing.T) {
	store := newMemoryStore()
	svc, repo := newImportService(t, store)
	ctx := context.Background()

	session, structure, err := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 7)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}
	// Analysis describes the file; it deliberately does not parse it into
	// products. What it must get right is the shape: three columns, a header
	// row, and a name column bound to the first of them.
	if len(structure.Columns) != 3 {
		t.Fatalf("described %d columns, want 3", len(structure.Columns))
	}
	if structure.HeaderRow != 1 {
		t.Fatalf("header row = %d, want 1", structure.HeaderRow)
	}
	if missing := structure.MissingCritical(); len(missing) != 0 {
		t.Fatalf("critical fields unbound: %v", missing)
	}

	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if len(store.written) != 0 {
		t.Fatalf("preparing wrote %d products to the catalogue; it must write none", len(store.written))
	}
	if len(repo.written) != 0 {
		t.Fatalf("preparing wrote %d products via the catalogue repo; it must write none", len(repo.written))
	}
	if len(store.rows) != 3 {
		t.Fatalf("staged %d rows, want 3", len(store.rows))
	}
	if store.session.Status != catalog.SessionReady {
		t.Errorf("status = %q, want ready", store.session.Status)
	}
}

func TestPrepareImportAssignsActionsPerMode(t *testing.T) {
	tests := []struct {
		mode                 catalog.ImportMode
		insert, update, skip int
	}{
		{catalog.ModeUpdateAndAdd, 2, 1, 0},
		{catalog.ModeAddNewOnly, 2, 0, 1},
		{catalog.ModeUpdateExistingOnly, 0, 1, 2},
		{catalog.ModeClearAndAdd, 3, 0, 0},
	}

	for _, tc := range tests {
		t.Run(string(tc.mode), func(t *testing.T) {
			store := newMemoryStore()
			// One of the three already exists in the catalogue.
			store.existing["svc-1"] = 4812
			svc, _ := newImportService(t, store)
			ctx := context.Background()

			session, _, err := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
			if err != nil {
				t.Fatalf("analyse failed: %v", err)
			}
			prepared, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
				Mode: tc.mode, Options: catalog.DefaultImportOptions(),
			})
			if err != nil {
				t.Fatalf("prepare failed: %v", err)
			}

			if prepared.InsertRows != tc.insert || prepared.UpdateRows != tc.update || prepared.SkipRows != tc.skip {
				t.Errorf("insert=%d update=%d skip=%d; want %d, %d and %d",
					prepared.InsertRows, prepared.UpdateRows, prepared.SkipRows,
					tc.insert, tc.update, tc.skip)
			}
		})
	}
}

func TestCommitImportArchivesFirstOnlyForClearAndAdd(t *testing.T) {
	for _, mode := range []catalog.ImportMode{catalog.ModeUpdateAndAdd, catalog.ModeClearAndAdd} {
		t.Run(string(mode), func(t *testing.T) {
			store := newMemoryStore()
			svc, _ := newImportService(t, store)
			ctx := context.Background()

			session, _, _ := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
			if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
				Mode: mode, Options: catalog.DefaultImportOptions(),
			}); err != nil {
				t.Fatalf("prepare failed: %v", err)
			}

			committed, result, err := svc.CommitImport(ctx, session.PublicID)
			if err != nil {
				t.Fatalf("commit failed: %v", err)
			}
			if committed.Status != catalog.SessionCommitted {
				t.Errorf("status = %q, want committed", committed.Status)
			}
			if result.Total() != 3 {
				t.Errorf("wrote %d products, want 3", result.Total())
			}
			if len(store.written) != 3 {
				t.Errorf("repository received %d products, want 3", len(store.written))
			}

			wantArchive := int64(0)
			if mode == catalog.ModeClearAndAdd {
				wantArchive = 1
			}
			if store.archived != wantArchive {
				t.Errorf("archived %d times, want %d for mode %q", store.archived, wantArchive, mode)
			}
		})
	}
}

func TestCommitImportSkipsDeselectedRows(t *testing.T) {
	store := newMemoryStore()
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	session, _, _ := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if err := svc.SetRowIncluded(ctx, session.PublicID, store.rows[1].ID, false); err != nil {
		t.Fatalf("deselect failed: %v", err)
	}

	if _, result, err := svc.CommitImport(ctx, session.PublicID); err != nil {
		t.Fatalf("commit failed: %v", err)
	} else if result.Total() != 2 {
		t.Errorf("wrote %d products, want 2", result.Total())
	}
	if len(store.written) != 2 {
		t.Errorf("repository received %d products, want 2", len(store.written))
	}
}

func TestCommitImportRefusesASecondTime(t *testing.T) {
	store := newMemoryStore()
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	session, _, _ := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if _, _, err := svc.CommitImport(ctx, session.PublicID); err != nil {
		t.Fatalf("first commit failed: %v", err)
	}

	// A double submit must not import the same file twice.
	if _, _, err := svc.CommitImport(ctx, session.PublicID); err == nil {
		t.Fatal("a committed session was committed again")
	}
	if len(store.written) != 3 {
		t.Errorf("repository received %d products, want 3 — the second commit must write nothing", len(store.written))
	}
}

func TestCancelImportPreventsCommit(t *testing.T) {
	store := newMemoryStore()
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	session, _, _ := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if err := svc.CancelImport(ctx, session.PublicID); err != nil {
		t.Fatalf("cancel failed: %v", err)
	}

	if _, _, err := svc.CommitImport(ctx, session.PublicID); err == nil {
		t.Fatal("a cancelled session was committed")
	}
	if len(store.written) != 0 {
		t.Errorf("repository received %d products after a cancel, want 0", len(store.written))
	}
}

// AI is an enhancement, never a dependency. A Gateway that is down must leave
// the deterministic values in place and let the import finish.
func TestPrepareImportSurvivesAnUnavailableMapper(t *testing.T) {
	store := newMemoryStore()
	store.vocab = testVocabulary()
	svc, _ := newImportService(t, store)
	svc.SetAIMapper(&stubMapper{available: true, fail: errGatewayDown})
	ctx := context.Background()

	session, _, _ := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
	prepared, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd,
		Options: catalog.ImportOptions{
			UseAI: true, AssignCategory: true, AssignDosageForm: true,
		},
	})
	if err != nil {
		t.Fatalf("prepare failed when the Gateway was down: %v", err)
	}

	if prepared.Status != catalog.SessionReady {
		t.Errorf("status = %q, want ready — an unavailable model must not fail the import", prepared.Status)
	}
	if prepared.ParsedRows != 3 {
		t.Errorf("parsed %d rows, want 3", prepared.ParsedRows)
	}
	if !prepared.AIFallback {
		t.Error("the session does not record that it fell back")
	}
	// The deterministic pass still ran: every row carries a form, inferred from
	// the name where it could be and the placeholder where it could not.
	for _, row := range store.rows {
		if row.Product.DosageForm == "" {
			t.Errorf("row %d has no dosage form; the name-based rules did not run",
				row.SourceRow)
		}
	}
}
