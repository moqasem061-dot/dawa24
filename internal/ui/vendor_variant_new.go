package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorVariantNewPage renders the variant creation form with master product combobox and branches.
func (h *UIHandler) VendorVariantNewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/variants/new", http.StatusSeeOther)
		return
	}

	// No master-product list is loaded here. The picker is a combobox backed by
	// /vendor/catalog/search-json, so the page used to fetch two hundred of the
	// twenty thousand catalogue products, render none of them, and leave a
	// supplier unable to find anything outside that arbitrary window.
	var branches []*org.Branch
	if h.orgSvc != nil {
		branches, _ = h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	}

	selectedProdID, _ := strconv.ParseInt(r.URL.Query().Get("product_id"), 10, 64)
	form := pages.VendorVariantFormInput{
		ProductID:   selectedProdID,
		MinOrderQty: "1",
	}
	if selectedProdID > 0 && h.catSvc != nil {
		if mp, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), selectedProdID); err == nil && mp != nil {
			form.ProductName = mp.Name.Get(i18n.Lang(lang))
			form.NameAR = mp.Name.Get(i18n.AR)
			form.NameEN = mp.Name.Get(i18n.EN)
			form.Price = mp.Price.String()
			form.SKU = mp.SKU
			form.Barcode = mp.Barcode
			form.Unit = mp.Unit
		}
	}

	data := pages.VendorVariantEditorData{

		Branches:       branches,
		SelectedProdID: selectedProdID,
		Form:           form,
		Errors:         pages.VendorVariantFormErrors{Fields: map[string]string{}},
	}

	h.renderPage(ctx, w, "render new variant page", pages.VendorProductEditor(data, lang, dir))
}

// VendorVariantNewSubmit processes vendor variant creation with comprehensive validation.
func (h *UIHandler) VendorVariantNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	branchOptions, _ := h.vendorBranchOptions(ctx, actor.OrganizationID)

	form, errors, variant, stockQty := parseAndValidateVariantNew(ctx, h, r, actor.OrganizationID, lang, branchOptions)
	if errors.General != "" || len(errors.Fields) > 0 {
		h.respondVariantNewError(w, r, ctx, lang, dir, branchOptions, form, errors)
		return
	}

	if h.catSvc == nil {
		errors.General = i18n.T(lang, "common.catalog_service_unavailable")
		h.respondVariantNewError(w, r, ctx, lang, dir, branchOptions, form, errors)
		return
	}

	created, err := h.catSvc.CreateVariant(ctx, variant)
	if err != nil {
		h.log.ErrorContext(ctx, "create variant error", "error", err)
		errors.General = h.safeMessage(err, lang)
		h.respondVariantNewError(w, r, ctx, lang, dir, branchOptions, form, errors)
		return
	}

	// Record opening stock into the inventory warehouse strictly associated with the branch
	if stockQty > 0 && created != nil {
		if err := h.recordInitialStock(ctx, actor.OrganizationID, created, stockQty); err != nil {
			h.log.WarnContext(ctx, "variant created but initial stock failed", "error", err, "variant_id", created.ID)
		}
	}

	successMsg := i18n.T(lang, "vendor.variant.published_success")
	// redirectWithNotice already handles the HTMX case with HX-Redirect, and it
	// is the only place that knows which query parameters noticeFrom reads. The
	// hand-built URL this replaces put the message in `notice` and the kind in
	// `notice_type`, so noticeFrom took the kind from `notice_type` and found no
	// message at all: every successful publish showed a blank green banner.
	h.redirectWithNotice(w, r, "/vendor/products", "success", successMsg)
}

func (h *UIHandler) respondVariantNewError(
	w http.ResponseWriter, r *http.Request, ctx context.Context,
	lang, dir string, branchOptions []pages.VendorBranchOption,
	form pages.VendorVariantFormInput, errors pages.VendorVariantFormErrors,
) {
	w.WriteHeader(http.StatusUnprocessableEntity)
	if h.isHTMX(r) {
		modalData := pages.VendorCustomVariantModalData{
			Branches: branchOptions,
			Form:     form,
			Errors:   errors,
		}
		_ = pages.AddCustomVariantForm(modalData, lang).Render(ctx, w)
		return
	}

	var orgBranches []*org.Branch
	if h.orgSvc != nil {
		actor, _ := authctx.From(ctx)
		orgBranches, _ = h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	}
	editorData := pages.VendorVariantEditorData{
		Branches:       orgBranches,
		SelectedProdID: form.ProductID,
		Form:           form,
		Errors:         errors,
	}
	h.renderPage(ctx, w, "render new variant error page", pages.VendorProductEditor(editorData, lang, dir))
}

func parseAndValidateVariantNew(
	ctx context.Context, h *UIHandler, r *http.Request,
	orgID int64, lang string, branchOptions []pages.VendorBranchOption,
) (pages.VendorVariantFormInput, pages.VendorVariantFormErrors, *catalog.ProductVariant, int) {
	errors := pages.VendorVariantFormErrors{Fields: make(map[string]string)}

	prodID, _ := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("product_id")), 10, 64)
	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	sku := strings.TrimSpace(r.PostFormValue("sku"))
	barcode := strings.TrimSpace(r.PostFormValue("barcode"))
	unit := strings.TrimSpace(r.PostFormValue("unit"))
	priceStr := strings.TrimSpace(r.PostFormValue("price"))
	discountStr := strings.TrimSpace(r.PostFormValue("discount"))
	costStr := strings.TrimSpace(r.PostFormValue("cost_price"))
	costDiscStr := strings.TrimSpace(r.PostFormValue("cost_discount_percentage"))
	if costDiscStr == "" {
		costDiscStr = strings.TrimSpace(r.PostFormValue("cost_discount"))
	}
	stockQty, _ := strconv.Atoi(strings.TrimSpace(r.PostFormValue("stock_qty")))
	minQty, _ := strconv.Atoi(strings.TrimSpace(r.PostFormValue("min_order_qty")))
	if minQty <= 0 {
		minQty = 1
	}
	quotaLimitStr := strings.TrimSpace(r.PostFormValue("quota_limit"))
	branchIDVal, _ := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("branch_id")), 10, 64)
	batch := strings.TrimSpace(r.PostFormValue("batch_number"))
	expiryDateStr := strings.TrimSpace(r.PostFormValue("expiry_date"))
	isNegotiable := r.PostFormValue("is_negotiable") == "true" || r.PostFormValue("is_negotiable") == "1"

	form := pages.VendorVariantFormInput{
		ProductID:              prodID,
		NameAR:                 nameAr,
		NameEN:                 nameEn,
		SKU:                    sku,
		Barcode:                barcode,
		Unit:                   unit,
		Price:                  priceStr,
		Discount:               discountStr,
		CostPrice:              costStr,
		CostDiscountPercentage: costDiscStr,
		StockQty:               r.PostFormValue("stock_qty"),
		MinOrderQty:            r.PostFormValue("min_order_qty"),
		QuotaLimit:             quotaLimitStr,
		BranchID:               branchIDVal,
		BatchNumber:            batch,
		ExpiryDate:             expiryDateStr,
		IsNegotiable:           isNegotiable,
	}

	// 1. Master product validation
	if prodID <= 0 {
		errors.Fields["product_id"] = i18n.T(lang, "vendor.catalog.select_master_product_first")
	} else if h.catSvc != nil {
		mp, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), prodID)
		if err != nil || mp == nil {
			errors.Fields["product_id"] = i18n.T(lang, "vendor.variant.product_not_found")
		} else {
			form.ProductName = mp.Name.Get(i18n.Lang(lang))
			if form.NameAR == "" {
				form.NameAR = mp.Name.Get(i18n.AR)
			}
			if form.NameEN == "" {
				form.NameEN = mp.Name.Get(i18n.EN)
			}
			if form.Unit == "" {
				form.Unit = mp.Unit
			}
		}
	}

	// 2. Arabic name validation
	if form.NameAR == "" {
		errors.Fields["name_ar"] = i18n.T(lang, "vendor.variant.name_ar_required")
	}

	// 3. Branch validation (REQUIRED: supplier own branch)
	validBranch := false
	if branchIDVal > 0 {
		for _, b := range branchOptions {
			if b.ID == branchIDVal {
				validBranch = true
				break
			}
		}
	}
	if !validBranch {
		errors.Fields["branch_id"] = i18n.T(lang, "vendor.variant.branch_required")
	}

	// 4. Price validation
	price, priceErr := money.Parse(priceStr)
	if priceErr != nil || !price.IsPositive() {
		errors.Fields["price"] = i18n.T(lang, "vendor.variant.price_invalid")
	}

	// 5. Discount validation
	var discount money.Amount
	if discountStr != "" {
		d, dErr := money.Parse(discountStr)
		if dErr != nil || d.IsNegative() || d.Minor() > 10000 {
			errors.Fields["discount"] = i18n.T(lang, "vendor.variant.discount_invalid")
		} else {
			discount = d
		}
	}

	// 6. Cost discount percentage validation
	var costDiscount float64
	if costDiscStr != "" {
		cd, cdErr := strconv.ParseFloat(costDiscStr, 64)
		if cdErr != nil || cd < 0 || cd > 100 {
			errors.Fields["cost_discount_percentage"] = i18n.T(lang, "vendor.variant.cost_discount_invalid")
		} else {
			costDiscount = cd
		}
	}

	// 7. Quota limit validation
	quotaLimit, quotaErr := parseQuotaLimit(quotaLimitStr, lang)
	if quotaErr != nil {
		errors.Fields["quota_limit"] = quotaErr.Error()
	}

	// 8. Expiry date validation
	var expiryDate *time.Time
	if expiryDateStr != "" {
		if t, err := time.Parse("2006-01-02", expiryDateStr); err == nil {
			expiryDate = &t
		} else {
			errors.Fields["expiry_date"] = i18n.T(lang, "vendor.catalog.invalid_expiry")
		}
	}

	if len(errors.Fields) > 0 {
		errors.General = i18n.T(lang, "vendor.variant.form_errors_heading")
		return form, errors, nil, 0
	}

	var cost *money.Amount
	if costStr != "" {
		if c, err := money.Parse(costStr); err == nil && c.IsPositive() {
			cost = &c
		}
	}

	branchID := &branchIDVal
	variant := &catalog.ProductVariant{
		OrganizationID:         orgID,
		ProductID:              prodID,
		Name:                   i18n.New(form.NameAR, form.NameEN),
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
		Unit:                   form.Unit,
		IsNegotiable:           isNegotiable,
		Status:                 catalog.StatusActive,
	}

	return form, errors, variant, stockQty
}
