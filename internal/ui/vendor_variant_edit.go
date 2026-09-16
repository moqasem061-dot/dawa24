package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
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

// Reading the variant edit form.
//
// Every field follows one rule: a key the form did not send leaves the stored
// value alone. The handler this replaces assigned unconditionally from
// r.FormValue, and because the dialog carries no SKU, barcode, English-name or
// unit input, every save wrote an empty string over all four.
//
// The rule is expressed once, in formField, rather than repeated as an if per
// column — which is how the previous version came to guard some fields and not
// others.

// formField returns a submitted value and whether the form carried it at all.
func formField(form url.Values, key string) (string, bool) {
	if form == nil || !form.Has(key) {
		return "", false
	}
	return strings.TrimSpace(form.Get(key)), true
}

// applyVariantEdit folds the submitted form onto a stored variant.
func applyVariantEdit(r *http.Request, v *catalog.ProductVariant, lang string) error {
	return applyVariantValues(r.PostForm, v, lang)
}

// applyVariantValues folds edit values onto a stored variant. The assistant
// passes the same keys the edit dialog posts.
func applyVariantValues(r url.Values, v *catalog.ProductVariant, lang string) error {
	// The name is bilingual and the dialog only carries Arabic. Writing
	// i18n.New(nameAr, "") erased the English name on every save.
	nameAr, hasAr := formField(r, "name_ar")
	nameEn, hasEn := formField(r, "name_en")
	if hasAr || hasEn {
		next := v.Name
		if next == nil {
			next = i18n.Text{}
		}
		if hasAr && nameAr != "" {
			next[i18n.AR] = nameAr
		}
		if hasEn {
			next[i18n.EN] = nameEn
		}
		v.Name = next
	}

	if sku, ok := formField(r, "sku"); ok {
		v.SKU = sku
	}
	if barcode, ok := formField(r, "barcode"); ok {
		v.Barcode = barcode
	}
	if unit, ok := formField(r, "unit"); ok && unit != "" {
		v.Unit = unit
	}
	if batch, ok := formField(r, "batch_number"); ok {
		v.BatchNumber = batch
	}

	if raw, ok := formField(r, "price"); ok && raw != "" {
		price, err := money.Parse(raw)
		if err != nil || price.IsNegative() {
			return fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_price"))
		}
		v.Price = price
	}
	if raw, ok := formField(r, "discount"); ok {
		if raw == "" {
			v.Discount = money.Zero
		} else {
			discount, err := money.Parse(raw)
			if err != nil || discount.IsNegative() || discount.Minor() > 10000 {
				return fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_discount"))
			}
			v.Discount = discount
		}
	}

	// Cost price and its discount are optional, and clearing them is a real
	// choice — but only when the form actually carried an empty field.
	if raw, ok := formField(r, "cost_price"); ok {
		if raw == "" {
			v.CostPrice = nil
		} else {
			cost, err := money.Parse(raw)
			if err != nil || cost.IsNegative() {
				return fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_price"))
			}
			v.CostPrice = &cost
		}
	}
	if raw, ok := formField(r, "cost_discount_percentage"); ok {
		if raw == "" {
			v.CostDiscountPercentage = 0
		} else {
			pct, err := strconv.ParseFloat(raw, 64)
			if err != nil || pct < 0 || pct > 100 {
				return fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_discount"))
			}
			v.CostDiscountPercentage = pct
		}
	}

	if raw, ok := formField(r, "min_order_qty"); ok && raw != "" {
		qty, err := strconv.Atoi(raw)
		if err != nil || qty <= 0 {
			return fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_min_qty"))
		}
		v.MinOrderQty = qty
	}

	// The per-branch quota. An empty box is a real choice — it is how the
	// supplier removes the cap — so, unlike min_order_qty above, the empty case
	// is handled rather than skipped. Everything else here follows the same
	// rule as the rest of the form: a key the dialog did not send leaves the
	// stored value alone.
	if raw, ok := formField(r, "quota_limit"); ok {
		limit, err := parseQuotaLimit(raw, lang)
		if err != nil {
			return err
		}
		v.QuotaLimit = limit
	}

	if raw, ok := formField(r, "expiry_date"); ok {
		if raw == "" {
			v.ExpiryDate = nil
		} else {
			t, err := time.Parse("2006-01-02", raw)
			if err != nil {
				return fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_expiry"))
			}
			v.ExpiryDate = &t
		}
	}

	if raw, ok := formField(r, "status"); ok && raw != "" {
		switch catalog.ProductStatus(raw) {
		case catalog.StatusActive, catalog.StatusInactive,
			catalog.StatusPending, catalog.StatusRejected:
			v.Status = catalog.ProductStatus(raw)
		default:
			return fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_status"))
		}
	}

	if raw, ok := formField(r, "is_negotiable"); ok {
		v.IsNegotiable = isTruthy(raw)
	}

	return nil
}

// parseQuotaLimit reads a per-branch quota out of a form field.
//
// Blank and zero both mean "no quota". They are folded together here rather
// than at the four surfaces that offer the field, because a supplier clearing
// the box and a supplier typing 0 mean the same thing and the column stores
// only NULL or a positive number.
func parseQuotaLimit(raw, lang string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_quota"))
	}
	if limit <= 0 {
		return nil, nil
	}
	if limit > catalog.MaxQuotaLimit {
		return nil, fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.quota_too_large"))
	}
	return &limit, nil
}

// quotaFailureRedirect picks where to send a supplier whose quota entry was
// rejected while creating an item: back to the form they were on.
//
// The Referer is reduced to its path and query before it is used. It is a
// header, which means it is whatever the caller chose to send, and handing it
// to a redirect unexamined turns this form into an open redirect to any host on
// the internet.
func quotaFailureRedirect(r *http.Request) string {
	const fallback = "/vendor/products"
	ref := strings.TrimSpace(r.Header.Get("Referer"))
	if ref == "" {
		return fallback
	}
	u, err := url.Parse(ref)
	if err != nil || !strings.HasPrefix(u.Path, "/vendor/") {
		return fallback
	}
	out := u.Path
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out
}

// VendorVariantUpdateSubmit handles editing an existing variant.
func (h *UIHandler) VendorVariantUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)
	ctx = database.WithAuditActorID(ctx, actor.UserID)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(lang, "vendor.catalog.invalid_variant_id"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(lang, "common.invalid_form_data"))
		return
	}
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(lang, "common.catalog_service_unavailable"))
		return
	}

	back := vendorProductsBackURL(r)

	existing, msg, err := h.saveVariantEdit(ctx, actor, lang, id, r.PostForm, r.PostFormValue("branch_id"))
	if msg != "" {
		if err != nil {
			h.log.ErrorContext(ctx, "update variant", "error", err, "variant_id", id)
		}
		h.redirectWithNotice(w, r, back, "error", msg)
		return
	}

	if stockStr := strings.TrimSpace(r.FormValue("stock_qty")); stockStr != "" {
		if stockQty, convErr := strconv.Atoi(stockStr); convErr == nil && stockQty >= 0 {
			if stockErr := h.recordInitialStock(ctx, actor.OrganizationID, existing, stockQty); stockErr != nil {
				h.log.ErrorContext(ctx, "record variant stock", "error", stockErr, "variant_id", id)
			}
		}
	}

	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "vendor.catalog.variant_updated_success"))
}

// vendorProductsBackURL rebuilds the listing the vendor was looking at.
func vendorProductsBackURL(r *http.Request) string {
	q := url.Values{}
	for formKey, queryKey := range map[string]string{
		"page":          "page",
		"limit":         "limit",
		"q":             "q",
		"status_filter": "status",
		"stock":         "stock",
		"sort":          "sort",
	} {
		if v := strings.TrimSpace(r.PostFormValue(formKey)); v != "" {
			q.Set(queryKey, v)
		}
	}
	if len(q) == 0 {
		return "/vendor/products"
	}
	return "/vendor/products?" + q.Encode()
}

// ownVariant loads one of this supplier's own listings.
func (h *UIHandler) ownVariant(ctx context.Context, actor authctx.Actor, lang string, id int64) (*catalog.ProductVariant, string) {
	v, err := h.catSvc.GetVariant(ctx, id)
	if err != nil || v == nil || v.OrganizationID != actor.OrganizationID {
		return nil, i18n.T(lang, "vendor.catalog.variant_not_found")
	}
	return v, ""
}

// saveVariantEdit applies edit values to one of this supplier's listings and
// saves it. It returns a user-facing message on failure, with the underlying
// error when there is one to log.
func (h *UIHandler) saveVariantEdit(
	ctx context.Context, actor authctx.Actor, lang string, id int64, form url.Values, branchField string,
) (*catalog.ProductVariant, string, error) {
	existing, msg := h.ownVariant(ctx, actor, lang, id)
	if msg != "" {
		return nil, msg, nil
	}
	if err := applyVariantValues(form, existing, lang); err != nil {
		return nil, err.Error(), nil
	}
	if bID := h.resolveTeamBranch(ctx, actor.OrganizationID, branchField); bID != nil {
		existing.BranchID = bID
	}
	if _, err := h.catSvc.UpdateVariant(ctx, id, existing); err != nil {
		return nil, i18n.T(lang, "vendor.catalog.update_variant_error_prefix") + h.safeMessage(err, lang), err
	}
	return existing, "", nil
}
