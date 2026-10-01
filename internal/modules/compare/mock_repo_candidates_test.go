package compare_test

import (
	"context"
	"sort"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func (m *mockCompareRepo) FindCandidateProducts(ctx context.Context, orgID *int64, query, sku string, limit int) ([]*compare.CandidateProduct, error) {
	var candidates []*compare.CandidateProduct
	cleanQ := strings.ToLower(strings.TrimSpace(query))
	// Mock master catalog product
	if cleanQ == "" || strings.Contains("panadol extra", cleanQ) || strings.Contains("بانادول اكسترا", cleanQ) {
		candidates = append(candidates, &compare.CandidateProduct{
			ID:             45,
			SKU:            "1001",
			NameAr:         "بانادول اكسترا 24 قرص",
			NameEn:         "Panadol Extra 24 Tab",
			ScientificName: "Paracetamol + Caffeine",
		})
	}
	if cleanQ == "" || strings.Contains("congestal", cleanQ) || strings.Contains("كونجستال", cleanQ) {
		candidates = append(candidates, &compare.CandidateProduct{
			ID:             88,
			SKU:            "1003",
			NameAr:         "كونجستال 20 قرص",
			NameEn:         "Congestal 20 Tab",
			ScientificName: "Paracetamol + Pseudoephedrine",
		})
	}
	return candidates, nil
}

func (m *mockCompareRepo) ListCatalogMatchProducts(ctx context.Context, limit int) ([]*compare.CandidateProduct, error) {
	return m.FindCandidateProducts(ctx, nil, "", "", limit)
}

func (m *mockCompareRepo) SearchFileRows(ctx context.Context, userID int64, orgID *int64, query string, limit int) ([]*compare.CompareFileRowWithSupplier, error) {
	var results []*compare.CompareFileRowWithSupplier
	cleanQ := strings.ToLower(strings.TrimSpace(query))

	for fileID, rows := range m.fileRows {
		file, ok := m.files[fileID]
		if !ok || file.DeletedAt != nil || file.Status != compare.FileReady {
			continue
		}
		for _, r := range rows {
			if cleanQ == "" || strings.Contains(strings.ToLower(r.RawName), cleanQ) || strings.Contains(strings.ToLower(r.NormalizedName), cleanQ) || strings.Contains(strings.ToLower(r.SKU), cleanQ) || strings.Contains(strings.ToLower(file.SupplierName), cleanQ) {
				results = append(results, &compare.CompareFileRowWithSupplier{
					CompareFileRow: *r,
					SupplierName:   file.SupplierName,
				})
			}
		}
	}
	return results, nil
}

func (m *mockCompareRepo) ListDistinctSuppliers(ctx context.Context) ([]string, error) {
	var suppliers []string
	seen := make(map[string]bool)
	for _, f := range m.files {
		if f.DeletedAt == nil && f.Status == compare.FileReady && f.IsTempWarehouse && !seen[f.SupplierName] {
			seen[f.SupplierName] = true
			suppliers = append(suppliers, f.SupplierName)
		}
	}
	sort.Strings(suppliers)
	return suppliers, nil
}

func (m *mockCompareRepo) LoadPackRelationships(ctx context.Context, productIDs []int64) ([]productmatch.PackRelationship, error) {
	if len(m.packRels) > 0 {
		return m.packRels, nil
	}
	return nil, nil
}
