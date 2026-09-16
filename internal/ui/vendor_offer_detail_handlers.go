package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) loadVendorOfferItemOptions(ctx context.Context, orgID int64) ([]*org.Branch, []*catalog.ProductVariant, []pages.VendorOfferItemOption) {
	var branches []*org.Branch
	if h.orgSvc != nil && orgID > 0 {
		bList, err := h.orgSvc.ListBranches(ctx, orgID)
		if err != nil {
			h.log.WarnContext(ctx, "vendor offer: list branches", "error", err)
		} else {
			branches = bList
		}
	}

	var variants []*catalog.ProductVariant
	var itemOptions []pages.VendorOfferItemOption

	var warehouses []*inventory.Warehouse
	var stocks []*inventory.Stock
	whNameMap := make(map[int64]string)
	stockMap := make(map[int64]*inventory.Stock)

	if h.invSvc != nil && orgID > 0 {
		allWhs, _ := h.invSvc.ListWarehouses(ctx)
		stocks, _ = h.invSvc.ListStocksByOrg(ctx, orgID)
		for _, wh := range allWhs {
			if wh != nil && wh.OrganizationID == orgID {
				warehouses = append(warehouses, wh)
				whNameMap[wh.ID] = wh.Name
			}
		}
		for _, s := range stocks {
			if s != nil {
				stockMap[s.ProductVariantID] = s
			}
		}
	}

	if h.catSvc != nil {
		vars, _, err := h.catSvc.ListVariantsByOrganization(ctx, orgID, catalog.VariantSearchParams{Limit: 500})
		if err == nil {
			variants = vars
		}
		if len(variants) == 0 {
			products, err := h.catSvc.Search(ctx, catalog.SearchParams{Limit: 100})
			if err == nil {
				for _, p := range products {
					pVars, _ := h.catSvc.ListVariantsByProduct(ctx, p.ID)
					for _, v := range pVars {
						if v.OrganizationID == orgID || v.OrganizationID == 0 {
							variants = append(variants, v)
						}
					}
				}
			}
		}

		for _, v := range variants {
			if v == nil {
				continue
			}
			whName := ""
			stockQty := v.StockQty
			if s, ok := stockMap[v.ID]; ok && s != nil {
				stockQty = s.Quantity
				if n, exists := whNameMap[s.WarehouseID]; exists {
					whName = n
				}
			}
			expStr := ""
			if v.ExpiryDate != nil {
				expStr = v.ExpiryDate.Format("2006-01-02")
			}
			itemOptions = append(itemOptions, pages.VendorOfferItemOption{
				VariantID:      v.ID,
				NameAr:         v.Name["ar"],
				NameEn:         v.Name["en"],
				SKU:            v.SKU,
				BatchNumber:    v.BatchNumber,
				ExpiryDate:     expStr,
				Price:          v.Price.String(),
				PriceFloat:     float64(v.Price.Minor()) / 100.0,
				WarehouseName:  whName,
				AvailableStock: stockQty,
			})
		}
	}

	return branches, variants, itemOptions
}

// VendorOfferNewPage renders the special offer creation form.
func (h *UIHandler) VendorOfferNewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers/new", http.StatusSeeOther)
		return
	}

	branches, variants, itemOptions := h.loadVendorOfferItemOptions(ctx, actor.OrganizationID)

	data := pages.VendorOfferFormData{
		Branches:    branches,
		Variants:    variants,
		ItemOptions: itemOptions,
		IsEdit:      false,
	}

	h.renderPage(ctx, w, "render vendor offer new page", pages.VendorOfferFormPage(data, lang, dir))
}

// VendorOfferEditPage renders the special offer edit form.
func (h *UIHandler) VendorOfferEditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 || h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", "العرض غير موجود")
		return
	}

	offer, err := h.promoSvc.GetSpecialOffer(ctx, id)
	if err != nil || offer == nil || offer.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", "العرض غير موجود أو ليس لديك صلاحية لتعديله")
		return
	}

	branches, variants, itemOptions := h.loadVendorOfferItemOptions(ctx, actor.OrganizationID)

	data := pages.VendorOfferFormData{
		Offer:       offer,
		Branches:    branches,
		Variants:    variants,
		ItemOptions: itemOptions,
		IsEdit:      true,
	}

	h.renderPage(ctx, w, "render vendor offer edit page", pages.VendorOfferFormPage(data, lang, dir))
}

// VendorOfferNewSubmit handles special offer creation with file uploads.
func (h *UIHandler) VendorOfferNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}
	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	if err := r.ParseMultipartForm(uploadMemoryBudget); err != nil {
		if perr := r.ParseForm(); perr != nil {
			h.redirectWithNotice(w, r, "/vendor/offers/new", "error", i18n.T(lang, "common.invalid_form_data"))
			return
		}
	}

	in, err := readOfferForm(r, lang)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/offers/new", "error", err.Error())
		return
	}
	if !h.offerBranchBelongsToOrg(ctx, actor.OrganizationID, in.BranchID) {
		h.redirectWithNotice(w, r, "/vendor/offers/new", "error", i18n.T(lang, "vendor.offer.branch_forbidden"))
		return
	}

	image := strings.TrimSpace(r.PostFormValue("image"))
	if uploaded, upErr := saveUploadedFile(r, "image_file", "offers"); upErr == nil && uploaded != "" {
		image = uploaded
	}

	o := &promo.SpecialOffer{OrganizationID: actor.OrganizationID, Image: image}
	in.applyTo(o)

	created, err := h.promoSvc.CreateSpecialOffer(ctx, o)
	if err != nil {
		h.log.ErrorContext(ctx, "create special offer",
			"error", err, "organization_id", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/offers/new", "error", h.safeMessage(err, lang))
		return
	}

	h.safeGo("dispatch-offer-created-notif", func() {
		h.dispatchInAppNotification(context.Background(), actor.UserID, &actor.OrganizationID, "vendor.offer.view",
			i18n.T(lang, "vendor.offer.created_notification_title"),
			fmt.Sprintf(i18n.T(lang, "vendor.offer.created_notification_body"), in.TitleAr))
	})

	h.log.InfoContext(ctx, "special offer created",
		"offer_id", created.ID, "organization_id", actor.OrganizationID, "products", len(in.Products))
	h.redirectWithNotice(w, r, "/vendor/offers", "success", i18n.T(lang, "vendor.offer.created_review_success"))
}

// VendorOfferEditSubmit processes special offer updates with file uploads.
func (h *UIHandler) VendorOfferEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 || h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "vendor.offer.not_found"))
		return
	}

	existing, err := h.promoSvc.GetSpecialOffer(ctx, id)
	if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "vendor.offer.forbidden"))
		return
	}

	if err := r.ParseMultipartForm(uploadMemoryBudget); err != nil {
		if perr := r.ParseForm(); perr != nil {
			h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "common.invalid_form_data"))
			return
		}
	}

	editURL := fmt.Sprintf("/vendor/offers/%d/edit", existing.ID)

	in, err := readOfferForm(r, lang)
	if err != nil {
		h.redirectWithNotice(w, r, editURL, "error", err.Error())
		return
	}
	if !h.offerBranchBelongsToOrg(ctx, actor.OrganizationID, in.BranchID) {
		h.redirectWithNotice(w, r, editURL, "error", i18n.T(lang, "vendor.offer.branch_forbidden"))
		return
	}

	// The stored image survives unless the vendor uploads a new one. A form
	// that omits the field must not blank the offer's artwork.
	image := existing.Image
	if formImg := strings.TrimSpace(r.PostFormValue("image")); formImg != "" {
		image = formImg
	}
	if uploaded, upErr := saveUploadedFile(r, "image_file", "offers"); upErr == nil && uploaded != "" {
		image = uploaded
	}
	existing.Image = image

	in.applyTo(existing)
	for _, p := range existing.Products {
		p.OfferID = existing.ID
	}

	if err := h.promoSvc.UpdateSpecialOffer(ctx, existing); err != nil {
		h.log.ErrorContext(ctx, "update special offer", "error", err, "offer_id", existing.ID)
		h.redirectWithNotice(w, r, editURL, "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/offers", "success", i18n.T(lang, "vendor.offer.updated_success"))
}

// offerBranchBelongsToOrg refuses a branch the acting company does not own.
// The branch decides which warehouse fulfils the offer, so accepting one from
// another tenant would route a pharmacy's order to a stranger.
func (h *UIHandler) offerBranchBelongsToOrg(ctx context.Context, orgID int64, branchID *int64) bool {
	if branchID == nil || *branchID <= 0 {
		return true
	}
	if h.orgSvc == nil {
		return false
	}
	branches, err := h.orgSvc.ListBranches(ctx, orgID)
	if err != nil {
		h.log.ErrorContext(ctx, "offer branch check: list branches", "error", err, "organization_id", orgID)
		return false
	}
	for _, b := range branches {
		if b != nil && b.ID == *branchID {
			return true
		}
	}
	return false
}
