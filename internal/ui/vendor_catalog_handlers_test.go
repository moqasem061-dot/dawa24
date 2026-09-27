package ui

import (
	"context"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

type vendorCatalogAddRepository struct {
	catalog.Repository
	bySKU            map[string]*catalog.ProductVariant
	byProduct        map[int64]*catalog.ProductVariant
	firstCreateError error
	onFirstCreate    func(*catalog.ProductVariant)
	createCalls      int
}

func (r *vendorCatalogAddRepository) CreateVariant(_ context.Context, variant *catalog.ProductVariant) error {
	r.createCalls++
	if r.createCalls == 1 && r.firstCreateError != nil {
		if r.onFirstCreate != nil {
			r.onFirstCreate(variant)
		}
		return r.firstCreateError
	}
	r.bySKU[variant.SKU] = variant
	r.byProduct[variant.ProductID] = variant
	return nil
}

func (r *vendorCatalogAddRepository) GetVariantBySKUOrBarcode(_ context.Context, _ int64, sku, barcode string) (*catalog.ProductVariant, error) {
	if variant := r.bySKU[sku]; sku != "" && variant != nil {
		return variant, nil
	}
	for _, variant := range r.bySKU {
		if barcode != "" && variant.Barcode == barcode {
			return variant, nil
		}
	}
	return nil, apperr.NotFound("product_variant")
}

func (r *vendorCatalogAddRepository) GetVariantByProductAndOrg(_ context.Context, _ int64, productID int64) (*catalog.ProductVariant, error) {
	if variant := r.byProduct[productID]; variant != nil {
		return variant, nil
	}
	return nil, apperr.NotFound("product_variant")
}

func TestCreateVendorCatalogVariantRetriesUniqueSKURace(t *testing.T) {
	otherProduct := &catalog.ProductVariant{ProductID: 1, SKU: "MASTER-SKU"}
	repo := newVendorCatalogAddRepository()
	repo.bySKU[otherProduct.SKU] = otherProduct
	repo.firstCreateError = apperr.Conflict("vendor.catalog.duplicate_sku", "duplicate SKU")
	service := catalog.NewService(repo, slog.Default())
	handler := &UIHandler{catSvc: service}
	variant := &catalog.ProductVariant{OrganizationID: 7, ProductID: 2, Name: i18n.New("اسم", "Name"), SKU: "MASTER-SKU"}

	ctx := database.WithTenant(context.Background(), 7)
	created, alreadyAdded, err := handler.createVendorCatalogVariant(ctx, 7, variant)
	if err != nil {
		t.Fatalf("createVendorCatalogVariant() error = %v", err)
	}
	if alreadyAdded {
		t.Fatal("createVendorCatalogVariant() reported an existing product after adding a different one")
	}
	if created == nil || created.SKU != "D24-2" || repo.createCalls != 2 {
		t.Fatalf("created = %#v, create calls = %d; want SKU D24-2 after one retry", created, repo.createCalls)
	}
}

func TestCreateVendorCatalogVariantTreatsConcurrentSameProductAsAlreadyAdded(t *testing.T) {
	existing := &catalog.ProductVariant{OrganizationID: 7, ProductID: 2, SKU: "MASTER-SKU"}
	repo := newVendorCatalogAddRepository()
	repo.firstCreateError = apperr.Conflict("vendor.catalog.duplicate_sku", "duplicate SKU")
	repo.onFirstCreate = func(*catalog.ProductVariant) { repo.byProduct[2] = existing }
	handler := &UIHandler{catSvc: catalog.NewService(repo, slog.Default())}
	ctx := database.WithTenant(context.Background(), 7)

	created, alreadyAdded, err := handler.createVendorCatalogVariant(ctx, 7, &catalog.ProductVariant{
		OrganizationID: 7, ProductID: 2, Name: i18n.New("اسم", "Name"), SKU: "MASTER-SKU",
	})
	if err != nil {
		t.Fatalf("createVendorCatalogVariant() error = %v", err)
	}
	if !alreadyAdded || created != existing || repo.createCalls != 1 {
		t.Fatalf("created = %#v, alreadyAdded = %t, create calls = %d; want existing variant without retry", created, alreadyAdded, repo.createCalls)
	}
}

func newVendorCatalogAddRepository() *vendorCatalogAddRepository {
	return &vendorCatalogAddRepository{
		bySKU:     make(map[string]*catalog.ProductVariant),
		byProduct: make(map[int64]*catalog.ProductVariant),
	}
}
