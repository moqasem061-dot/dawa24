package main

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	"github.com/moqasem061-dot/dawa24/internal/modules/inventory"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// ingestCatalogPort adapts the catalog service to the import pipeline. The
// two modules never import each other (AGENTS.md rule 5); this is the one
// place that knows both sets of types.
type ingestCatalogPort struct{ svc *catalog.Service }

func (p ingestCatalogPort) ListMatchProducts(ctx context.Context) ([]ingest.MatchProduct, error) {
	in, err := p.svc.ListMatchProducts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ingest.MatchProduct, len(in))
	for i, m := range in {
		out[i] = ingest.MatchProduct(m)
	}
	return out, nil
}

func (p ingestCatalogPort) ImportVocabulary(ctx context.Context, orgID int64) (ingest.Vocabulary, error) {
	v, err := p.svc.ImportVocabulary(ctx, orgID)
	if err != nil {
		return ingest.Vocabulary{}, err
	}
	out := ingest.Vocabulary{
		Brands:     make([]string, 0, len(v.Brands)),
		Categories: make([]string, 0, len(v.Categories)),
	}
	for _, b := range v.Brands {
		out.Brands = append(out.Brands, b.Name)
	}
	for _, c := range v.Categories {
		out.Categories = append(out.Categories, c.Name)
	}
	return out, nil
}

func (p ingestCatalogPort) ListVariantKeys(ctx context.Context, orgID int64) ([]ingest.VariantKey, error) {
	in, err := p.svc.ListVariantKeys(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]ingest.VariantKey, len(in))
	for i, k := range in {
		out[i] = ingest.VariantKey(k)
	}
	return out, nil
}

func (p ingestCatalogPort) BulkWriteVariants(ctx context.Context, orgID int64, rows []ingest.VariantWriteRow) (ingest.VariantWriteResult, error) {
	in := make([]catalog.VariantWriteRow, len(rows))
	for i, r := range rows {
		in[i] = catalog.VariantWriteRow{Ref: r.Ref, Variant: catalogVariant(r.Variant)}
	}
	res, err := p.svc.BulkWriteVariants(ctx, orgID, in)
	out := ingest.VariantWriteResult{
		Inserted:     res.Inserted,
		Updated:      res.Updated,
		IDs:          res.IDs,
		InsertedRefs: res.InsertedRefs,
	}
	for _, f := range res.Failures {
		out.Failures = append(out.Failures, ingest.WriteFailure(f))
	}
	return out, err
}

func (p ingestCatalogPort) RetireVariantsExcept(ctx context.Context, orgID, warehouseID int64, keep []int64) ([]ingest.RetiredVariant, error) {
	in, err := p.svc.RetireVariantsExcept(ctx, orgID, warehouseID, keep)
	out := make([]ingest.RetiredVariant, len(in))
	for i, r := range in {
		out[i] = ingest.RetiredVariant(r)
	}
	return out, err
}

func (p ingestCatalogPort) ProductRef(ctx context.Context, id int64) (*ingest.ProductRef, error) {
	prod, _, err := p.svc.GetProduct(ctx, id)
	if err != nil || prod == nil {
		return nil, err
	}
	return &ingest.ProductRef{
		ID:     prod.ID,
		NameAR: prod.Name.Get("ar"),
		NameEN: prod.Name.Get("en"),
		SKU:    prod.SKU,
	}, nil
}

// catalogVariant renders an import draft as the catalogue's variant.
func catalogVariant(d *ingest.VariantDraft) *catalog.ProductVariant {
	if d == nil {
		return nil
	}
	return &catalog.ProductVariant{
		ID:             d.ID,
		OrganizationID: d.OrganizationID,
		ProductID:      d.ProductID,
		Name:           d.Name,
		SKU:            d.SKU,
		Barcode:        d.Barcode,
		Unit:           d.Unit,
		BatchNumber:    d.BatchNumber,
		ExpiryDate:     d.ExpiryDate,
		BranchID:       d.BranchID,
		Price:          d.Price,
		Discount:       d.Discount,
		CostPrice:      d.CostPrice,
		// The column is NUMERIC(5,2) percent; basis points convert exactly.
		CostDiscountPercentage: float64(d.CostDiscountBps) / 100,
		MinOrderQty:            d.MinOrderQty,
		IsNegotiable:           d.IsNegotiable,
		Status:                 catalog.ProductStatus(d.Status),
		VariantType:            d.VariantType,
	}
}

// ingestInventoryPort adapts the inventory service to the import pipeline.
type ingestInventoryPort struct{ svc *inventory.Service }

func (p ingestInventoryPort) ListWarehouses(ctx context.Context) ([]ingest.Warehouse, error) {
	in, err := p.svc.ListWarehouses(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ingest.Warehouse, 0, len(in))
	for _, w := range in {
		if w != nil {
			out = append(out, ingest.Warehouse{ID: w.ID, Name: w.Name, Code: w.Code})
		}
	}
	return out, nil
}

func (p ingestInventoryPort) BulkWriteStocks(ctx context.Context, mode ingest.StockMode, rows []ingest.StockWriteRow) (ingest.StockWriteResult, error) {
	in := make([]inventory.StockWriteRow, len(rows))
	for i, r := range rows {
		in[i] = inventory.StockWriteRow{
			Ref:         r.Ref,
			HasQuantity: r.HasQuantity,
			Stock: &inventory.Stock{
				OrganizationID:   r.OrganizationID,
				WarehouseID:      r.WarehouseID,
				ProductID:        r.ProductID,
				ProductVariantID: r.VariantID,
				Quantity:         r.Quantity,
				MinThreshold:     r.MinThreshold,
			},
		}
	}
	res, err := p.svc.BulkWriteStocks(ctx, inventory.StockMode(mode), in)
	out := ingest.StockWriteResult{Written: res.Written}
	for _, f := range res.Failures {
		out.Failures = append(out.Failures, ingest.WriteFailure(f))
	}
	return out, err
}

func (p ingestInventoryPort) ClearWarehouseStocks(ctx context.Context, warehouseID int64) error {
	return p.svc.ClearWarehouseStocks(ctx, warehouseID)
}

func (p ingestInventoryPort) VariantIDsInWarehouse(ctx context.Context, warehouseID int64) (map[int64]bool, error) {
	return p.svc.VariantIDsInWarehouse(ctx, warehouseID)
}

// ingestMatcherPort adapts matching.Service to the ingest.Matcher interface.
type ingestMatcherPort struct {
	svc matching.Service
}

func (p ingestMatcherPort) Prepare(ctx context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error) {
	return p.svc.Prepare(ctx, req)
}

func (p ingestMatcherPort) MarkReviewable(ctx context.Context, runID int64) error {
	return p.svc.MarkReviewable(ctx, runID)
}

func (p ingestMatcherPort) Rematch(ctx context.Context, s productmatch.Scope, row productmatch.Row, rev int) (matchflow.RowDecision, error) {
	return p.svc.Rematch(ctx, s, row, rev)
}

func (p ingestMatcherPort) Revalidate(ctx context.Context, s productmatch.Scope, ref productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
	return p.svc.Revalidate(ctx, s, ref, d)
}
