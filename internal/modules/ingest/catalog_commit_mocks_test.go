package ingest

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
)

// mockImportStore provides an in-memory fake for ImportStore during commit testing.
type mockCommitImportStore struct {
	session        *Session
	stagedRows     []*RowOutcome
	committedRows  []RowOutcome
	finishedCalled bool
	// pendingRows is how many included rows the vendor never confirmed.
	pendingRows int
	// mentions is what every included row of the file refers to, confirmed or
	// not, which is what the replace mode protects from retirement.
	mentions []RowMention
}

func (m *mockCommitImportStore) MentionedRows(_ context.Context, _ int64) ([]RowMention, error) {
	return m.mentions, nil
}

func (m *mockCommitImportStore) Create(_ context.Context, _ *Session, _ []byte) error { return nil }
func (m *mockCommitImportStore) Get(_ context.Context, _ string) (*Session, error) {
	if m.session != nil {
		return m.session, nil
	}
	return nil, ErrImportStoreUnavailable
}
func (m *mockCommitImportStore) File(_ context.Context, _ int64) ([]byte, error) { return nil, nil }
func (m *mockCommitImportStore) SaveDraft(_ context.Context, s *Session) error {
	m.session = s
	return nil
}
func (m *mockCommitImportStore) Begin(_ context.Context, _ int64) error       { return nil }
func (m *mockCommitImportStore) BeginCommit(_ context.Context, _ int64) error { return nil }
func (m *mockCommitImportStore) Progress(_ context.Context, _ int64, _ int, _ string) error {
	return nil
}
func (m *mockCommitImportStore) FinishStaging(_ context.Context, s *Session) error {
	m.session = s
	return nil
}
func (m *mockCommitImportStore) RecoverStaleRuns(_ context.Context) (int, error) { return 0, nil }
func (m *mockCommitImportStore) Finish(_ context.Context, s *Session) error {
	m.session = s
	m.finishedCalled = true
	return nil
}
func (m *mockCommitImportStore) Fail(_ context.Context, _ int64, _ string) error { return nil }
func (m *mockCommitImportStore) Cancel(_ context.Context, _ int64) error         { return nil }
func (m *mockCommitImportStore) List(_ context.Context, _ int64, _ int) ([]*Session, error) {
	return nil, nil
}
func (m *mockCommitImportStore) AppendRows(_ context.Context, _, _ int64, _ []RowOutcome) error {
	return nil
}
func (m *mockCommitImportStore) ClearRows(_ context.Context, _ int64) error { return nil }
func (m *mockCommitImportStore) Rows(_ context.Context, _ int64, _ RowFilter) ([]*RowOutcome, int, error) {
	return m.stagedRows, len(m.stagedRows), nil
}
func (m *mockCommitImportStore) RowCounts(_ context.Context, _ int64) (map[string]int, error) {
	return map[string]int{}, nil
}
func (m *mockCommitImportStore) ApplyAIMatches(_ context.Context, _ int64, _ []AIMatch) error {
	return nil
}
func (m *mockCommitImportStore) UpdateRow(_ context.Context, _, _ int64, _ string, _ string, _, _, _, _ *float64, _ *int, _ *bool) error {
	return nil
}
func (m *mockCommitImportStore) SetBatchQuantity(_ context.Context, _ int64, _ int) error { return nil }
func (m *mockCommitImportStore) AssignRowMatch(_ context.Context, _, _, _ int64, _, _ string) error {
	return nil
}
func (m *mockCommitImportStore) ToggleRowExclude(_ context.Context, _, _ int64) (bool, error) {
	return false, nil
}
func (m *mockCommitImportStore) ConfirmRowMatches(_ context.Context, _ int64, _ []int64) (int, error) {
	return 0, nil
}
func (m *mockCommitImportStore) ClearRowMatches(_ context.Context, _ int64, _ []int64) (int, error) {
	return 0, nil
}
func (m *mockCommitImportStore) SetRowsExcluded(_ context.Context, _ int64, _ []int64, _ bool) (int, error) {
	return 0, nil
}
func (m *mockCommitImportStore) PendingRowIDs(_ context.Context, _ int64, _ int) ([]int64, int, error) {
	return nil, m.pendingRows, nil
}
func (m *mockCommitImportStore) RowIDsForFilter(_ context.Context, _ int64, _ RowFilter, _ int) ([]int64, error) {
	return nil, nil
}
func (m *mockCommitImportStore) StagedRowsForCommit(_ context.Context, _ int64) ([]*RowOutcome, error) {
	return m.stagedRows, nil
}
func (m *mockCommitImportStore) UpdateCommittedRows(_ context.Context, _ int64, rows []RowOutcome) error {
	m.committedRows = append(m.committedRows, rows...)
	return nil
}
func (m *mockCommitImportStore) Sweep(_ context.Context) error { return nil }

// mockCommitCatalogPort mocks CatalogPort for testing.
type mockCommitCatalogPort struct {
	keys              []catalog.VariantKey
	writtenVariants   []catalog.VariantWriteRow
	deactivatedExcept []int64
	retired           []catalog.RetiredVariant
	nextVariantID     int64
}

func (m *mockCommitCatalogPort) ListMatchProducts(_ context.Context) ([]catalog.MatchProduct, error) {
	return nil, nil
}
func (m *mockCommitCatalogPort) ImportVocabulary(_ context.Context, _ int64) (catalog.EnrichVocabulary, error) {
	return catalog.EnrichVocabulary{}, nil
}
func (m *mockCommitCatalogPort) ListVariantKeys(_ context.Context, _ int64) ([]catalog.VariantKey, error) {
	return m.keys, nil
}
func (m *mockCommitCatalogPort) BulkWriteVariants(_ context.Context, _ int64, rows []catalog.VariantWriteRow) (catalog.VariantWriteResult, error) {
	res := catalog.VariantWriteResult{
		IDs:          make(map[int]int64, len(rows)),
		InsertedRefs: make(map[int]bool, len(rows)),
	}
	for _, r := range rows {
		m.writtenVariants = append(m.writtenVariants, r)
		if r.Variant.ID > 0 {
			res.IDs[r.Ref] = r.Variant.ID
			res.Updated++
		} else {
			m.nextVariantID++
			res.IDs[r.Ref] = m.nextVariantID
			res.InsertedRefs[r.Ref] = true
			res.Inserted++
		}
	}
	return res, nil
}
func (m *mockCommitCatalogPort) RetireVariantsExcept(
	_ context.Context, _ int64, _ int64, keep []int64,
) ([]catalog.RetiredVariant, error) {
	m.deactivatedExcept = keep
	kept := make(map[int64]bool, len(keep))
	for _, id := range keep {
		kept[id] = true
	}
	var out []catalog.RetiredVariant
	for _, k := range m.keys {
		if !kept[k.ID] {
			out = append(out, catalog.RetiredVariant{ID: k.ID, ProductID: k.ProductID})
		}
	}
	m.retired = out
	return out, nil
}
func (m *mockCommitCatalogPort) GetProduct(_ context.Context, _ int64) (*catalog.Product, []*catalog.ProductVariant, error) {
	return nil, nil, nil
}
func (m *mockCommitCatalogPort) Search(_ context.Context, _ catalog.SearchParams) ([]*catalog.Product, error) {
	return nil, nil
}

// mockCommitInventoryPort mocks InventoryPort for testing.
type mockCommitInventoryPort struct {
	warehouses  []*inventory.Warehouse
	writtenRows []inventory.StockWriteRow
	lastMode    inventory.StockMode
	// inWarehouse is the set of variants already holding a balance in the
	// warehouse an import is writing to.
	inWarehouse map[int64]bool
}

func (m *mockCommitInventoryPort) ListWarehouses(_ context.Context) ([]*inventory.Warehouse, error) {
	return m.warehouses, nil
}
func (m *mockCommitInventoryPort) BulkWriteStocks(_ context.Context, mode inventory.StockMode, rows []inventory.StockWriteRow) (inventory.StockWriteResult, error) {
	m.lastMode = mode
	m.writtenRows = append(m.writtenRows, rows...)
	return inventory.StockWriteResult{Written: len(rows)}, nil
}
func (m *mockCommitInventoryPort) ClearWarehouseStocks(_ context.Context, _ int64) error {
	return nil
}
func (m *mockCommitInventoryPort) VariantIDsInWarehouse(_ context.Context, _ int64) (map[int64]bool, error) {
	if m.inWarehouse == nil {
		return map[int64]bool{}, nil
	}
	return m.inWarehouse, nil
}
