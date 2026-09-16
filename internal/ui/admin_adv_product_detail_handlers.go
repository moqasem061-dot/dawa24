package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminAdvProductDetailPage renders the full detail and audit view for a single product sponsorship.
func (h *UIHandler) AdminAdvProductDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", i18n.T(lang, "promo.sponsorship.invalid_id"))
		return
	}

	row, err := h.promoSvc.GetAdminSponsorshipRow(sysCtx, id)
	if err != nil || row == nil {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", i18n.T(lang, "promo.sponsorship.not_found"))
		return
	}

	var purchaseID int64
	if row.PurchaseID != nil {
		purchaseID = *row.PurchaseID
	}

	var purchase *promo.SponsorshipPurchase
	if purchaseID > 0 {
		if p, err := h.promoSvc.GetSponsorshipPurchaseByID(sysCtx, purchaseID); err == nil && p != nil {
			purchase = p
		}
	}

	var creditEntries []*promo.CreditEntry
	if entries, err := h.promoSvc.GetAdminSponsorshipCreditEntries(sysCtx, purchaseID, row.ID); err == nil {
		creditEntries = entries
	}

	var events []*promo.AdminSponsorshipEvent
	itemID := row.ItemID
	if itemID <= 0 {
		itemID = row.ProductID
	}
	if itemID > 0 {
		if evs, err := h.promoSvc.GetAdminSponsorshipEvents(sysCtx, itemID); err == nil {
			events = evs
		}
	}

	noticeType := r.URL.Query().Get("notice")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	noticeMsg := r.URL.Query().Get("msg")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("message")
	}

	data := pages.AdminAdvProductDetailData{
		Row:           row,
		Purchase:      purchase,
		CreditEntries: creditEntries,
		Events:        events,
		NoticeType:    noticeType,
		NoticeMsg:     noticeMsg,
	}

	h.renderPage(ctx, w, "render admin adv-product detail page", pages.AdminAdvProductDetailPage(lang, dir, data))
}
