package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

const vendorCatalogSKUCreateRetries = 3

// VendorProductDetailJSON returns detailed master product data by ID as JSON.
func (h *UIHandler) VendorProductDetailJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, `{"error":"invalid product id"}`, http.StatusBadRequest)
		return
	}

	if h.catSvc == nil {
		http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	p, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), id)
	if err != nil || p == nil {
		http.Error(w, `{"error":"product not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":                      p.ID,
		"name_ar":                 p.Name.Get(i18n.AR),
		"name_en":                 p.Name.Get(i18n.EN),
		"scientific_name":         p.ScientificName,
		"dosage_form":             p.DosageForm,
		"concentration":           p.Concentration,
		"unit":                    p.Unit,
		"sku":                     p.SKU,
		"barcode":                 p.Barcode,
		"price":                   p.Price.String(),
		"manufacturing_companies": p.ManufacturingCompanies,
		"category_id":             p.CategoryID,
	})
}

// VendorProductAddFromCatalogSubmit adds a master product as a vendor variant with customized fields.
func (h *UIHandler) VendorProductAddFromCatalogSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	productID, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	if productID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "vendor.catalog.select_master_product_first"))
		return
	}
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "common.catalog_service_unavailable"))
		return
	}

	masterProduct, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), productID)
	if err != nil || masterProduct == nil {
		if err != nil {
			h.logLoadErr(ctx, "catSvc.GetProduct", err)
		}
		if err == nil || apperr.KindOf(err) == apperr.KindNotFound {
			h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "vendor.catalog.select_master_product_first"))
		} else {
			h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "common.catalog_service_unavailable"))
		}
		return
	}
	nameAr := masterProduct.Name.Get(i18n.AR)
	nameEn := masterProduct.Name.Get(i18n.EN)
	if nameAr == "" && nameEn == "" {
		nameAr = i18n.T("ar", "vendor.catalog.default_variant_name")
		nameEn = i18n.T("en", "vendor.catalog.default_variant_name")
	} else if nameAr == "" {
		nameAr = nameEn
	} else if nameEn == "" {
		nameEn = nameAr
	}

	batch := strings.TrimSpace(r.FormValue("batch_number"))
	sku := strings.TrimSpace(masterProduct.SKU)
	barcode := strings.TrimSpace(masterProduct.Barcode)
	unit := strings.TrimSpace(masterProduct.Unit)
	if unit == "" {
		unit = "item"
	}

	priceStr := strings.TrimSpace(r.FormValue("price"))
	costStr := strings.TrimSpace(r.FormValue("cost_price"))
	costDiscStr := strings.TrimSpace(r.FormValue("cost_discount_percentage"))
	if costDiscStr == "" {
		costDiscStr = strings.TrimSpace(r.FormValue("cost_discount"))
	}
	stockQty, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("stock_qty")))
	minQty, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("min_order_qty")))
	if minQty <= 0 {
		minQty = 1
	}

	// The supplier's per-branch quota. A blank box means no quota at all, which
	// is what an item without one has always had, so a parse failure here is a
	// message rather than a silent zero.
	quotaLimit, quotaErr := parseQuotaLimit(r.PostFormValue("quota_limit"), langOf(r))
	if quotaErr != nil {
		h.redirectWithNotice(w, r, quotaFailureRedirect(r), "error", quotaErr.Error())
		return
	}

	branchIDVal, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("branch_id")), 10, 64)
	if branchIDVal <= 0 {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "vendor.variant.branch_required"))
		return
	}
	branchID := &branchIDVal

	var expiryDate *time.Time
	if expStr := strings.TrimSpace(r.FormValue("expiry_date")); expStr != "" {
		if t, err := time.Parse("2006-01-02", expStr); err == nil { // nolint:errswallow: optional input; an unparseable value leaves the default
			expiryDate = &t
		}
	}

	price, _ := money.Parse(priceStr)
	var cost *money.Amount
	if costStr != "" {
		if c, err := money.Parse(costStr); err == nil && c.IsPositive() {
			cost = &c
		}
	}
	costDiscount, _ := strconv.ParseFloat(costDiscStr, 64)
	if costDiscount < 0 {
		costDiscount = 0
	} else if costDiscount > 100 {
		costDiscount = 100
	}

	discount, discountErr := vendorInputDiscount(r.PostForm, price)
	if discountErr != nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "vendor.pricing.invalid"))
		return
	}
	isNegotiable := r.FormValue("is_negotiable") == "true" || r.FormValue("is_negotiable") == "1"

	variant := &catalog.ProductVariant{
		OrganizationID:         actor.OrganizationID,
		ProductID:              productID,
		Name:                   i18n.New(nameAr, nameEn),
		BatchNumber:            batch,
		ExpiryDate:             expiryDate,
		Price:                  price,
		CostPrice:              cost,
		CostDiscountPercentage: costDiscount,
		Discount:               discount,
		StockQty:               stockQty,
		MinOrderQty:            minQty,
		QuotaLimit:             quotaLimit,
		BranchID:               branchID,
		SKU:                    sku,
		Barcode:                barcode,
		Unit:                   unit,
		IsNegotiable:           isNegotiable,
		Status:                 catalog.StatusActive,
	}

	if existing, lookupErr := h.catSvc.GetVariantByProductAndOrg(ctx, actor.OrganizationID, productID); lookupErr == nil && existing != nil {
		h.redirectWithNotice(w, r, "/vendor/products", "success", i18n.T(langOf(r), "vendor.catalog.variant_already_added"))
		return
	} else if lookupErr != nil && apperr.KindOf(lookupErr) != apperr.KindNotFound {
		h.logLoadErr(ctx, "catSvc.GetVariantByProductAndOrg", lookupErr)
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "common.catalog_service_unavailable"))
		return
	}

	if sku != "" {
		if existing, lookupErr := h.catSvc.GetVariantBySKUOrBarcode(ctx, actor.OrganizationID, sku, ""); lookupErr == nil && existing != nil {
			if existing.ProductID == productID {
				h.redirectWithNotice(w, r, "/vendor/products", "success", i18n.T(langOf(r), "vendor.catalog.variant_already_added"))
				return
			}
			sku, lookupErr = h.uniqueVendorCatalogSKU(ctx, actor.OrganizationID, productID)
			if lookupErr != nil {
				h.logLoadErr(ctx, "resolve catalog product sku collision", lookupErr)
				h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "common.catalog_service_unavailable"))
				return
			}
			variant.SKU = sku
		} else if lookupErr != nil && apperr.KindOf(lookupErr) != apperr.KindNotFound {
			h.logLoadErr(ctx, "catSvc.GetVariantBySKUOrBarcode", lookupErr)
			h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "common.catalog_service_unavailable"))
			return
		}
	}

	created, alreadyAdded, err := h.createVendorCatalogVariant(ctx, actor.OrganizationID, variant)
	if alreadyAdded {
		h.redirectWithNotice(w, r, "/vendor/products", "success", i18n.T(langOf(r), "vendor.catalog.variant_already_added"))
		return
	}
	if err != nil {
		h.log.ErrorContext(ctx, "add variant from catalog error", "error", err)
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "vendor.catalog.add_variant_error_prefix")+h.safeMessage(err, langOf(r)))
		return
	}

	if stockQty > 0 && created != nil {
		_ = h.recordInitialStock(ctx, actor.OrganizationID, created, stockQty)
	}

	h.redirectWithNotice(w, r, "/vendor/products", "success", i18n.T(langOf(r), "vendor.catalog.variant_added_success"))
}

func (h *UIHandler) createVendorCatalogVariant(
	ctx context.Context, organizationID int64, variant *catalog.ProductVariant,
) (*catalog.ProductVariant, bool, error) {
	for attempt := 0; attempt < vendorCatalogSKUCreateRetries; attempt++ {
		created, err := h.catSvc.CreateVariant(ctx, variant)
		if err == nil || !isDuplicateVendorCatalogSKU(err) {
			return created, false, err
		}

		sku, existing, err := h.nextVendorCatalogSKU(ctx, organizationID, variant.ProductID)
		if err != nil {
			return nil, false, err
		}
		if existing != nil {
			return existing, true, nil
		}
		variant.SKU = sku
	}
	return nil, false, apperr.Conflict("vendor.catalog.duplicate_sku", i18n.TDefault("vendor.catalog.duplicate_sku"))
}

func (h *UIHandler) nextVendorCatalogSKU(
	ctx context.Context, organizationID, productID int64,
) (string, *catalog.ProductVariant, error) {
	existing, err := h.catSvc.GetVariantByProductAndOrg(ctx, organizationID, productID)
	if err == nil && existing != nil {
		return "", existing, nil
	}
	if err != nil && apperr.KindOf(err) != apperr.KindNotFound {
		return "", nil, err
	}
	sku, err := h.uniqueVendorCatalogSKU(ctx, organizationID, productID)
	return sku, nil, err
}

func isDuplicateVendorCatalogSKU(err error) bool {
	appErr, ok := apperr.As(err)
	return ok && appErr.Code == "vendor.catalog.duplicate_sku"
}

func (h *UIHandler) uniqueVendorCatalogSKU(ctx context.Context, organizationID, productID int64) (string, error) {
	base := fmt.Sprintf("D24-%d", productID)
	for suffix := 1; suffix <= 100; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = fmt.Sprintf("%s-%d", base, suffix)
		}
		existing, err := h.catSvc.GetVariantBySKUOrBarcode(ctx, organizationID, candidate, "")
		if err != nil {
			if apperr.KindOf(err) == apperr.KindNotFound {
				return candidate, nil
			}
			return "", err
		}
		if existing != nil && existing.ProductID == productID {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("catalog product %d has no available vendor SKU", productID)
}

// VendorCatalogSelectPage permanently redirects legacy route to /vendor/products.
func (h *UIHandler) VendorCatalogSelectPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/vendor/products", http.StatusMovedPermanently)
}

// VendorCatalogSelectSubmit redirects legacy form submission to /vendor/products.
func (h *UIHandler) VendorCatalogSelectSubmit(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/vendor/products", http.StatusMovedPermanently)
}

// VendorProductsDeleteAllSubmit removes all products/variants of the current vendor.
func (h *UIHandler) VendorProductsDeleteAllSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)
	ctx = database.WithAuditActorID(ctx, actor.UserID)
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "common.catalog_service_unavailable"))
		return
	}
	count, err := h.catSvc.DeleteAllVariantsByOrg(ctx, actor.OrganizationID)
	if err != nil {
		h.log.ErrorContext(ctx, "delete all vendor variants error", "error", err)
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "vendor.catalog.delete_variants_error_prefix")+h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/products", "success", fmt.Sprintf(i18n.T(langOf(r), "vendor.catalog.deleted_all_success"), count))
}

// VendorProductsActivateAllSubmit activates and publishes all variants belonging to the current vendor.
func (h *UIHandler) VendorProductsActivateAllSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)
	ctx = database.WithAuditActorID(ctx, actor.UserID)
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(lang, "common.catalog_service_unavailable"))
		return
	}

	count, err := h.catSvc.ActivateAllVariantsByOrg(ctx, actor.OrganizationID)
	if err != nil {
		h.log.ErrorContext(ctx, "activate all vendor variants error", "error", err)
		h.redirectWithNotice(w, r, "/vendor/products", "error", "تعذر تفعيل الأصناف: "+h.safeMessage(err, lang))
		return
	}

	msg := fmt.Sprintf("تم تفعيل ونشر %d صنف بنجاح ليصبح متاحاً للطلب في الكتالوج", count)
	if count == 0 {
		msg = "جميع الأصناف مفعلة ونشطة بالفعل"
	}
	h.redirectWithNotice(w, r, "/vendor/products", "success", msg)
}
