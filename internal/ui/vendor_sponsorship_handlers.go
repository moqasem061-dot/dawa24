package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorSponsorshipRequestsPage renders the vendor's sponsorship requests list
// and the package purchase form.
func (h *UIHandler) VendorSponsorshipRequestsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/sponsorship-requests", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var packages []*promo.OfferPackage
	var purchases []*promo.SponsorshipPurchase
	var requests []*promo.SponsorshipRequest
	var totalRequests int
	var activePurchases []*promo.SponsorshipPurchase
	var activeOffers []*promo.Offer

	if h.promoSvc != nil {
		packages, _ = h.promoSvc.ListPackages(ctx)
		purchases, _ = h.promoSvc.ListSponsorshipPurchases(ctx)
		requests, totalRequests, _ = h.promoSvc.ListSponsorshipRequestsByOrgWithTotal(ctx, limit, offset)
		activePurchases, _ = h.promoSvc.ListActiveSponsorshipPurchases(ctx)
		activeOffers, _ = h.promoSvc.ListActiveOffers(ctx, 100, 0)
	}

	totalCredits := 0
	for _, p := range activePurchases {
		if p != nil {
			totalCredits += p.CreditsRemainingInt()
		}
	}

	var walletBal money.Amount
	if h.billSvc != nil {
		sysCtx := database.AsSystem(ctx)
		if w, err := h.billSvc.GetWallet(sysCtx, actor.UserID, "EGP"); err == nil && w != nil {
			walletBal = w.Available()
		}
	}

	itemOptions := h.loadVendorInStockItems(ctx, actor.OrganizationID)

	data := pages.SponsorshipRequestsData{
		Packages:        packages,
		Purchases:       purchases,
		ActivePurchases: activePurchases,
		Requests:        requests,
		OrgID:           actor.OrganizationID,
		ItemOptions:     itemOptions,
		ActiveOffers:    activeOffers,
		TotalCredits:    totalCredits,
		WalletBalance:   walletBal,
		NoticeType:      r.URL.Query().Get("notice_type"),
		NoticeMsg:       r.URL.Query().Get("notice"),
		Page:            page,
		PerPage:         limit,
		TotalCount:      totalRequests,
	}

	h.renderPage(ctx, w, "render vendor sponsorship requests", pages.VendorSponsorshipRequestsPage(lang, dir, data))
}

// VendorSponsorshipRequestSubmit handles batch or single sponsorship request submission.
func (h *UIHandler) VendorSponsorshipRequestSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	_ = r.ParseForm()

	itemType := strings.TrimSpace(r.PostFormValue("item_type"))
	if itemType != "product" && itemType != "offer" {
		itemType = "product"
	}

	packageID, err := strconv.ParseInt(r.PostFormValue("package_id"), 10, 64)
	if err != nil || packageID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", "يرجى اختيار باقة رعاية تحتوي على رصيد كافٍ.")
		return
	}

	// Extract item IDs (supports multiple item_ids inputs or single item_id)
	var itemIDs []int64
	for _, raw := range r.PostForm["item_ids"] {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if id, parseErr := strconv.ParseInt(part, 10, 64); parseErr == nil && id > 0 {
				itemIDs = append(itemIDs, id)
			}
		}
	}
	if len(itemIDs) == 0 {
		if singleID, parseErr := strconv.ParseInt(r.PostFormValue("item_id"), 10, 64); parseErr == nil && singleID > 0 {
			itemIDs = append(itemIDs, singleID)
		}
	}

	if len(itemIDs) == 0 {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", "يرجى اختيار عنصر واحد على الأقل للرعاية.")
		return
	}

	created, err := h.promoSvc.SubmitBatchSponsorshipRequests(ctx, promo.SponsorshipItemType(itemType), itemIDs, packageID)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "success",
		"تم تقديم "+strconv.Itoa(len(created))+" طلب رعاية بنجاح وخصم "+strconv.Itoa(len(created))+" رصيد من باقتك، وهي قيد المراجعة.")
}

// VendorSponsorshipRequestCancelSubmit cancels a pending sponsorship request.
func (h *UIHandler) VendorSponsorshipRequestCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := h.localeAndDirLang(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "vendor.sponsorship.invalid_request_id"))
		return
	}

	if err := h.promoSvc.CancelSponsorshipRequest(ctx, id); err != nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "success", i18n.T(lang, "vendor.sponsorship.request_cancelled_success"))
}

// VendorSponsorshipPackagePurchaseSubmit handles the purchase of a sponsorship package.
func (h *UIHandler) VendorSponsorshipPackagePurchaseSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := h.localeAndDirLang(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)

	redirectURL := r.PostFormValue("redirect_url")
	if redirectURL == "" {
		redirectURL = "/vendor/sponsorship-requests"
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	packageID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || packageID <= 0 {
		h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "vendor.sponsorship.invalid_package_id"))
		return
	}

	autoRenew := r.PostFormValue("auto_renew") == "true" || r.PostFormValue("auto_renew") == "on"
	billingCycle := r.PostFormValue("billing_cycle")
	if billingCycle == "" {
		billingCycle = "monthly"
	}

	_, err = h.promoSvc.PurchaseSponsorshipPackage(ctx, packageID, autoRenew, billingCycle)
	if err != nil {
		h.redirectWithNotice(w, r, redirectURL, "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, redirectURL, "success", i18n.T(lang, "vendor.sponsorship.package_purchased_success"))
}

func (h *UIHandler) localeAndDirLang(r *http.Request) string {
	lang, _ := h.localeAndDir(r)
	return lang
}
