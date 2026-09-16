package catalog_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// memoryImportStore is an in-memory ImportSessionStore.
//
// It exists so the three-step flow — analyse, prepare, commit — can be driven
// without a database, and because implementing the port twice is the cheapest
// check that it is actually an abstraction and not a description of one
// PostgreSQL schema.
type memoryImportStore struct {
	session  *catalog.ImportSession
	file     []byte
	rows     []*catalog.StagingRow
	existing map[string]int64 // folded name or SKU -> product id
	// catalogue backs the similarity tier. It is empty unless a test populates
	// it, so tests written before that tier existed still exercise exactly the
	// exact-identifier path they were written for.
	catalogue []catalog.MatchProduct
	vocab     catalog.EnrichVocabulary
	archived  int64
	written   []*catalog.Product
	nextID    int64
}

func newMemoryStore() *memoryImportStore {
	return &memoryImportStore{
		existing: map[string]int64{},
		nextID:   1,
	}
}

func (m *memoryImportStore) CreateImportSession(_ context.Context, s *catalog.ImportSession, file []byte) error {
	s.ID, s.PublicID = 1, "11111111-1111-1111-1111-111111111111"
	m.session, m.file = s, file
	return nil
}

func (m *memoryImportStore) GetImportSession(_ context.Context, publicID string) (*catalog.ImportSession, error) {
	if m.session == nil || m.session.PublicID != publicID {
		return nil, fmt.Errorf("no such session %q", publicID)
	}
	return m.session, nil
}

func (m *memoryImportStore) UpdateImportSession(
	_ context.Context, s *catalog.ImportSession, _ ...catalog.SessionStatus,
) error {
	m.session = s
	return nil
}

func (m *memoryImportStore) SaveImportProgress(
	_ context.Context, _ string, p catalog.ImportProgress,
) error {
	if m.session != nil {
		m.session.Progress = p
	}
	return nil
}

func (m *memoryImportStore) ClaimImportSessionForCommit(
	_ context.Context, publicID string,
) (*catalog.ImportSession, error) {
	if m.session == nil || m.session.PublicID != publicID {
		return nil, fmt.Errorf("no such session %q", publicID)
	}
	if !m.session.IsReviewable() {
		return nil, fmt.Errorf("session %s is not reviewable (status %s)", publicID, m.session.Status)
	}
	m.session.Status = catalog.SessionCommitting
	return m.session, nil
}

func (m *memoryImportStore) ImportSourceFile(context.Context, int64) ([]byte, error) {
	return m.file, nil
}

func (m *memoryImportStore) ReleaseImportSourceFile(context.Context, int64) error {
	m.file = nil
	return nil
}

func (m *memoryImportStore) ListRecentImportSessions(context.Context, int64, int) ([]*catalog.ImportSession, error) {
	return nil, nil
}

func (m *memoryImportStore) ReplaceStagingRows(_ context.Context, _ int64, rows []*catalog.StagingRow) error {
	m.rows = rows
	for _, row := range rows {
		row.ID = m.nextID
		m.nextID++
	}
	return nil
}

func (m *memoryImportStore) ClearStagingRows(context.Context, int64) error {
	m.rows = nil
	return nil
}

func (m *memoryImportStore) ListStagingRows(
	context.Context, int64, catalog.StagingFilter,
) ([]*catalog.StagingRow, int, error) {
	return m.rows, len(m.rows), nil
}

func (m *memoryImportStore) LoadCommittableRows(context.Context, int64) ([]*catalog.StagingRow, error) {
	var out []*catalog.StagingRow
	for _, row := range m.rows {
		if row.Included && row.Action != catalog.ActionSkip && !row.HasErrors() {
			out = append(out, row)
		}
	}
	return out, nil
}

func (m *memoryImportStore) GetStagingRow(_ context.Context, _, rowID int64) (*catalog.StagingRow, error) {
	for _, row := range m.rows {
		if row.ID == rowID {
			return row, nil
		}
	}
	return nil, fmt.Errorf("row not found: %d", rowID)
}

func (m *memoryImportStore) SetRowIncluded(_ context.Context, _, rowID int64, included bool) error {
	for _, row := range m.rows {
		if row.ID == rowID {
			row.Included = included
		}
	}
	return nil
}

func (m *memoryImportStore) SetRowsIncludedByAction(
	_ context.Context, _ int64, action catalog.RowAction, included bool,
) (int64, error) {
	var n int64
	for _, row := range m.rows {
		if action == "" || row.Action == action {
			row.Included = included
			n++
		}
	}
	return n, nil
}

func (m *memoryImportStore) CountStagingActions(context.Context, int64) (catalog.StagingCounts, error) {
	var counts catalog.StagingCounts
	for _, row := range m.rows {
		counts.Total++
		switch {
		case !row.Included || row.Action == catalog.ActionSkip:
			counts.Skip++
		case row.Action == catalog.ActionInsert:
			counts.Insert++
		case row.Action == catalog.ActionUpdate:
			counts.Update++
		}
		if len(row.AIChanges) > 0 {
			counts.AIChanged++
		}
	}
	return counts, nil
}

func (m *memoryImportStore) DefaultCatalogOrg(context.Context) (int64, error) { return 50, nil }

func (m *memoryImportStore) MatchExistingProducts(
	_ context.Context, prods []*catalog.Product,
) (map[int]catalog.ExistingMatch, error) {
	out := map[int]catalog.ExistingMatch{}
	for i, p := range prods {
		if id, ok := m.existing[strings.ToLower(p.SKU)]; ok && p.SKU != "" {
			out[i] = catalog.ExistingMatch{ProductID: id, Reason: catalog.MatchSKU}
			continue
		}
		if id, ok := m.existing[catalog.NormalizeName(p.Name.Get(i18n.AR))]; ok {
			out[i] = catalog.ExistingMatch{ProductID: id, Reason: catalog.MatchName}
		}
	}
	return out, nil
}

// ListMatchProducts backs the similarity tier. The double returns the same
// products the exact matcher knows about, so a test that expects a row to match
// exactly still does, and one that expects no similarity match gets none.
func (m *memoryImportStore) ListMatchProducts(context.Context) ([]catalog.MatchProduct, error) {
	out := make([]catalog.MatchProduct, 0, len(m.catalogue))
	for _, p := range m.catalogue {
		out = append(out, p)
	}
	return out, nil
}

func (m *memoryImportStore) ImportVocabulary(context.Context, int64) (catalog.EnrichVocabulary, error) {
	return m.vocab, nil
}

func (m *memoryImportStore) BulkCommitProducts(
	_ context.Context, prods []*catalog.Product, _ catalog.BulkWriteOptions, archiveOrg int64,
) (int64, catalog.BulkWriteResult, error) {
	archived := int64(0)
	if archiveOrg > 0 {
		m.archived++
		archived = 7
	}
	m.written = append(m.written, prods...)
	res := catalog.BulkWriteResult{Matches: map[int]catalog.MatchReason{}}
	for _, p := range prods {
		if p.ID > 0 {
			res.Updated++
			continue
		}
		res.Inserted++
	}
	return archived, res, nil
}

// mockCatalogRepoStub implements dummy methods for catalog.Repository.
type mockCatalogRepoStub struct{}

func (mockCatalogRepoStub) CreateProduct(context.Context, *catalog.Product) error { return nil }
func (mockCatalogRepoStub) BulkUpsertProducts(context.Context, []*catalog.Product) (catalog.BulkWriteResult, error) {
	return catalog.BulkWriteResult{}, nil
}
func (mockCatalogRepoStub) GetProductByID(context.Context, int64) (*catalog.Product, error) {
	return nil, nil
}
func (mockCatalogRepoStub) UpdateProduct(context.Context, *catalog.Product) error { return nil }
func (mockCatalogRepoStub) DeleteProduct(context.Context, int64) error            { return nil }
func (mockCatalogRepoStub) SearchProducts(context.Context, catalog.SearchParams) ([]*catalog.Product, error) {
	return nil, nil
}
func (mockCatalogRepoStub) CountProducts(context.Context, catalog.SearchParams) (int, error) {
	return 0, nil
}
func (mockCatalogRepoStub) ListProducts(context.Context, string, int, int) ([]*catalog.Product, error) {
	return nil, nil
}
func (mockCatalogRepoStub) SetProductsStatus(context.Context, []int64, catalog.ProductStatus) (int64, error) {
	return 0, nil
}
func (mockCatalogRepoStub) CreateVariant(context.Context, *catalog.ProductVariant) error { return nil }
func (mockCatalogRepoStub) GetVariantByID(context.Context, int64) (*catalog.ProductVariant, error) {
	return nil, nil
}
func (mockCatalogRepoStub) GetVariantBySKUOrBarcode(context.Context, int64, string, string) (*catalog.ProductVariant, error) {
	return nil, nil
}
func (mockCatalogRepoStub) GetVariantByProductAndOrg(context.Context, int64, int64) (*catalog.ProductVariant, error) {
	return nil, nil
}
func (mockCatalogRepoStub) ListVariantsByProduct(context.Context, int64) ([]*catalog.ProductVariant, error) {
	return nil, nil
}
func (mockCatalogRepoStub) ListVariantsByProducts(context.Context, []int64) ([]*catalog.ProductVariant, error) {
	return nil, nil
}
func (mockCatalogRepoStub) ListVariantsByOrganization(context.Context, int64, catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	return nil, 0, nil
}
func (mockCatalogRepoStub) ListAllVariants(context.Context, catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	return nil, 0, nil
}
func (mockCatalogRepoStub) UpdateVariant(context.Context, *catalog.ProductVariant) error { return nil }
func (mockCatalogRepoStub) DeleteVariant(context.Context, int64) error                   { return nil }
func (mockCatalogRepoStub) SearchVariants(context.Context, catalog.VariantSearchParams) ([]*catalog.ProductVariant, error) {
	return nil, nil
}
func (mockCatalogRepoStub) SetVariantsStatus(context.Context, []int64, catalog.ProductStatus) (int64, error) {
	return 0, nil
}
func (mockCatalogRepoStub) ToggleVariantStatus(context.Context, int64, int64) (catalog.ProductStatus, error) {
	return catalog.StatusActive, nil
}
func (mockCatalogRepoStub) CreateCategory(context.Context, *catalog.Category) error { return nil }
func (mockCatalogRepoStub) GetCategoryByID(context.Context, int64) (*catalog.Category, error) {
	return nil, nil
}
func (mockCatalogRepoStub) ListCategories(context.Context) ([]*catalog.Category, error) {
	return nil, nil
}
func (mockCatalogRepoStub) ListCategoriesWithProductCount(context.Context, string, string, int, int) ([]*catalog.CategoryWithCount, int, error) {
	return nil, 0, nil
}
func (mockCatalogRepoStub) UpdateCategory(context.Context, *catalog.Category) error { return nil }
func (mockCatalogRepoStub) DeleteCategory(context.Context, int64) error             { return nil }
func (mockCatalogRepoStub) CountProductsByOrg(context.Context, int64, string) (int, error) {
	return 0, nil
}
func (mockCatalogRepoStub) CountProductsInCategory(context.Context, int64) (int, error) {
	return 0, nil
}
func (mockCatalogRepoStub) CreateBrand(context.Context, *catalog.Brand) error { return nil }
func (mockCatalogRepoStub) GetBrandByID(context.Context, int64) (*catalog.Brand, error) {
	return nil, nil
}
func (mockCatalogRepoStub) ListBrands(context.Context) ([]*catalog.Brand, error) { return nil, nil }
func (mockCatalogRepoStub) ListBrandsWithProductCount(context.Context, string, string, int, int) ([]*catalog.BrandWithCount, int, error) {
	return nil, 0, nil
}
func (mockCatalogRepoStub) UpdateBrand(context.Context, *catalog.Brand) error { return nil }
func (mockCatalogRepoStub) DeleteBrand(context.Context, int64) error          { return nil }
func (mockCatalogRepoStub) ListBrandsByCategory(context.Context, int64) ([]*catalog.Brand, error) {
	return nil, nil
}
func (mockCatalogRepoStub) BrandInCategory(context.Context, int64, int64) (bool, error) {
	return true, nil
}
func (mockCatalogRepoStub) SetBrandCategories(context.Context, int64, []int64) error { return nil }
func (mockCatalogRepoStub) CountProductsInBrand(context.Context, int64) (int, error) { return 0, nil }
func (mockCatalogRepoStub) SetCustomerPricing(context.Context, *catalog.CustomerProductMapping) error {
	return nil
}
func (mockCatalogRepoStub) GetCustomerPricing(context.Context, int64, int64, int64) (*catalog.CustomerProductMapping, error) {
	return nil, nil
}
func (mockCatalogRepoStub) CreateProductAlert(context.Context, *catalog.ProductAlert) error {
	return nil
}
func (mockCatalogRepoStub) ListProductAlertsByUser(context.Context, int64) ([]*catalog.ProductAlert, error) {
	return nil, nil
}
func (mockCatalogRepoStub) UpsertProductIndex(context.Context, *catalog.ProductIndexItem) error {
	return nil
}
func (mockCatalogRepoStub) DeleteProductIndex(context.Context, string) error         { return nil }
func (mockCatalogRepoStub) DeleteProductIndexByProduct(context.Context, int64) error { return nil }
func (mockCatalogRepoStub) SearchProductIndex(context.Context, catalog.SearchParams) ([]*catalog.ProductIndexItem, error) {
	return nil, nil
}
func (mockCatalogRepoStub) RebuildProductIndex(context.Context) (int64, error) { return 0, nil }
func (mockCatalogRepoStub) CreateSavingProduct(context.Context, *catalog.SavingProduct) error {
	return nil
}
func (mockCatalogRepoStub) UpdateSavingProduct(context.Context, *catalog.SavingProduct) error {
	return nil
}
func (mockCatalogRepoStub) ListSavingProductsByOrg(context.Context, int64, int, int) ([]*catalog.SavingProduct, error) {
	return nil, nil
}
func (mockCatalogRepoStub) ListSavingProductsEnriched(context.Context, int64, string, string, int, int) ([]*catalog.SavingProductEnriched, *catalog.SavingProductStats, error) {
	return nil, nil, nil
}
