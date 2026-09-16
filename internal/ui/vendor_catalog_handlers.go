package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// VendorCatalogSearchJSON returns JSON autocomplete list of general catalog products.
func (h *UIHandler) VendorCatalogSearchJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}

	type catalogResult struct {
		ID             string `json:"id"`
		Label          string `json:"label"`
		Hint           string `json:"hint,omitempty"`
		Badge          string `json:"badge,omitempty"`
		RawID          int64  `json:"raw_id"`
		NameAR         string `json:"name_ar"`
		NameEN         string `json:"name_en"`
		ScientificName string `json:"scientific_name"`
		DosageForm     string `json:"dosage_form"`
		Concentration  string `json:"concentration"`
		Unit           string `json:"unit"`
		SKU            string `json:"sku"`
		Barcode        string `json:"barcode"`
		Price          string `json:"price"`
		Manufacturer   string `json:"manufacturer"`
	}

	var results []catalogResult
	if h.catSvc != nil {
		products, err := h.catSvc.Search(ctx, catalog.SearchParams{
			Query: query,
			Limit: 20,
		})
		if err == nil {
			for _, p := range products {
				label := p.Name.Get(i18n.AR)
				if label == "" {
					label = p.Name.Get(i18n.EN)
				}
				if p.DosageForm != "" {
					label += " — " + p.DosageForm
				}
				hint := p.ScientificName
				if p.SKU != "" {
					if hint != "" {
						hint += " · "
					}
					hint += p.SKU
				}
				badge := p.Price.String() + " ج.م"
				results = append(results, catalogResult{
					ID:             fmt.Sprintf("%d", p.ID),
					Label:          label,
					Hint:           hint,
					Badge:          badge,
					RawID:          p.ID,
					NameAR:         p.Name.Get(i18n.AR),
					NameEN:         p.Name.Get(i18n.EN),
					ScientificName: p.ScientificName,
					DosageForm:     p.DosageForm,
					Concentration:  p.Concentration,
					Unit:           p.Unit,
					SKU:            p.SKU,
					Barcode:        p.Barcode,
					Price:          p.Price.String(),
					Manufacturer:   p.ManufacturingCompanies,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(results)
}

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

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	productID, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	if productID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "vendor.catalog.select_master_product_first"))
		return
	}

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		if h.catSvc != nil {
			if mp, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), productID); err == nil && mp != nil {
				nameAr = mp.Name.Get(i18n.AR)
				nameEn = mp.Name.Get(i18n.EN)
			}
		}
	}
	if nameAr == "" && nameEn == "" {
		nameAr = i18n.T("ar", "vendor.catalog.default_variant_name")
		nameEn = i18n.T("en", "vendor.catalog.default_variant_name")
	}

	batch := strings.TrimSpace(r.FormValue("batch_number"))
	sku := strings.TrimSpace(r.FormValue("sku"))
	barcode := strings.TrimSpace(r.FormValue("barcode"))
	unit := strings.TrimSpace(r.FormValue("unit"))
	if unit == "" {
		unit = "item"
	}

	priceStr := strings.TrimSpace(r.FormValue("price"))
	costStr := strings.TrimSpace(r.FormValue("cost_price"))
	costDiscStr := strings.TrimSpace(r.FormValue("cost_discount_percentage"))
	if costDiscStr == "" {
		costDiscStr = strings.TrimSpace(r.FormValue("cost_discount"))
	}
	discountStr := strings.TrimSpace(r.FormValue("discount"))
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
		if t, err := time.Parse("2006-01-02", expStr); err == nil {
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

	discount, _ := money.Parse(discountStr)
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

	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "common.catalog_service_unavailable"))
		return
	}

	created, err := h.catSvc.CreateVariant(ctx, variant)
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
