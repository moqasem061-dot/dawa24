package ui

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminOrdersPage renders the cross-tenant order search and procurement tabs.
func (h *UIHandler) AdminOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab == "" {
		tab = "all"
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	paymentStatus := strings.TrimSpace(r.URL.Query().Get("payment_status"))
	customerOrgID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("customer_org_id")), 10, 64)
	vendorOrgID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("vendor_org_id")), 10, 64)
	buyerType := strings.TrimSpace(r.URL.Query().Get("buyer_type"))
	dateFrom := strings.TrimSpace(r.URL.Query().Get("date_from"))
	dateTo := strings.TrimSpace(r.URL.Query().Get("date_to"))

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var orders []*commerce.Order
	var totalCount int
	var kpi commerce.AdminOrderKPIs
	var orgs []*org.Organization

	if h.orgSvc != nil {
		if list, err := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 1000, 0); err == nil {
			orgs = list
		}
	}

	// The buyer and seller selects come from the orders themselves. Filling
	// them from every organisation and narrowing by type in the template is
	// what hid every supplier that bought from another supplier.
	var parties commerce.AdminOrderParties
	if h.commSvc != nil {
		if p, err := h.commSvc.AdminOrderParties(sysCtx); err == nil {
			parties = p
		} else {
			h.log.WarnContext(ctx, "load admin order parties", "error", err)
		}
	}

	if h.commSvc != nil {
		var err error
		kpi, err = h.commSvc.AdminOrderKPIs(ctx)
		if err != nil {
			all, direct, neg, _ := h.commSvc.AdminOrderStats(ctx)
			kpi.TotalOrders = all
			kpi.DirectOrders = direct
			kpi.NegotiationOrders = neg
		}
		orders, totalCount, _ = h.commSvc.AdminSearchOrdersFiltered(ctx, commerce.AdminOrderFilter{
			Query:         query,
			Tab:           tab,
			Status:        status,
			PaymentStatus: paymentStatus,
			CustomerOrgID: customerOrgID,
			VendorOrgID:   vendorOrgID,
			BuyerType:     buyerType,
			DateFrom:      dateFrom,
			DateTo:        dateTo,
			Limit:         limit,
			Offset:        offset,
		})
	}

	data := pages.AdminOrdersData{
		ActiveTab:        tab,
		Query:            query,
		Status:           status,
		PaymentStatus:    paymentStatus,
		CustomerOrgID:    customerOrgID,
		BuyerType:        buyerType,
		Parties:          parties,
		VendorOrgID:      vendorOrgID,
		DateFrom:         dateFrom,
		DateTo:           dateTo,
		Orders:           orders,
		KPIs:             kpi,
		Organizations:    orgs,
		Page:             page,
		PerPage:          limit,
		TotalCount:       totalCount,
		AllCount:         kpi.TotalOrders,
		DirectCount:      kpi.DirectOrders,
		NegotiationCount: kpi.NegotiationOrders,
	}

	h.renderPage(ctx, w, "render admin orders", pages.AdminOrdersHub(data, lang, dir))
}

// AdminOffersPage renders the special supplier bundle offers moderation list.
func (h *UIHandler) AdminOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	orgID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("org_id")), 10, 64)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var offers []*promo.SpecialOffer
	var totalCount int
	if h.promoSvc != nil {
		if orgID > 0 {
			all, _ := h.promoSvc.ListSpecialOffersByOrg(ctx, orgID)
			var filtered []*promo.SpecialOffer
			for _, o := range all {
				if o == nil {
					continue
				}
				if statusFilter != "" && statusFilter != "all" {
					if statusFilter == "pending" && o.AdminStatus != "pending" && o.AdminStatus != "" {
						continue
					} else if statusFilter == "active" && (o.AdminStatus != "approved" || o.Status != "active") {
						continue
					} else if statusFilter == "rejected" && o.AdminStatus != "rejected" {
						continue
					} else if statusFilter == "draft" && o.Status != "draft" && o.Status != "inactive" {
						continue
					} else if statusFilter == "changes_requested" && o.AdminStatus != "changes_requested" {
						continue
					}
				}
				filtered = append(filtered, o)
			}
			totalCount = len(filtered)
			start := offset
			if start > totalCount {
				start = totalCount
			}
			end := start + limit
			if end > totalCount {
				end = totalCount
			}
			offers = filtered[start:end]
		} else {
			offers, totalCount, _ = h.promoSvc.ListAllSpecialOffersWithTotal(ctx, statusFilter, limit, offset)
		}
	}

	var orgs []*org.Organization
	if h.orgSvc != nil {
		orgs, _ = h.orgSvc.ListOrganizations(database.AsSystem(ctx), nil, nil, 500, 0)
	}

	data := pages.AdminOffersData{
		Offers:        offers,
		FilterStatus:  statusFilter,
		Organizations: orgs,
		SelectedOrgID: orgID,
		Page:          page,
		PerPage:       limit,
		TotalCount:    totalCount,
	}

	h.renderPage(ctx, w, "render admin offers", pages.AdminOffers(data, lang, dir))
}

// AdminOfferApproveSubmit approves a supplier special offer for marketplace publishing.
func (h *UIHandler) AdminOfferApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/offers", "error", i18n.T(lang, "admin.commerce.invalid_offer_id"))
		return
	}

	actor, _ := authctx.From(ctx)
	if h.promoSvc != nil {
		off, _ := h.promoSvc.GetSpecialOffer(ctx, id)
		if err := h.promoSvc.UpdateSpecialOfferAdminStatus(ctx, id, "approved", i18n.T(lang, "admin.commerce.approved_by_admin"), actor.UserID); err != nil {
			h.redirectWithNotice(w, r, "/admin/offers", "error", h.safeMessage(err, lang))
			return
		}
		_ = h.promoSvc.ToggleSpecialOfferStatus(ctx, id, true)
		if off != nil && off.OrganizationID > 0 {
			offTitle := off.Title.Get(i18n.Lang(lang))
			if offTitle == "" {
				offTitle = off.Title.Get(i18n.AR)
			}
			h.safeGo("notify-special-offer-approved", func() {
				h.notifySpecialOfferStatus(context.Background(), off.OrganizationID, offTitle, true, "")
			})
		}
	}

	h.redirectWithNotice(w, r, "/admin/offers", "success", i18n.T(lang, "admin.commerce.offer_approved_success"))
}

// AdminOfferRejectSubmit rejects a supplier special offer with optional reason notes.
func (h *UIHandler) AdminOfferRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/offers", "error", i18n.T(lang, "admin.commerce.invalid_offer_id"))
		return
	}

	actor, _ := authctx.From(ctx)
	notes := strings.TrimSpace(r.PostFormValue("notes"))
	if notes == "" {
		notes = i18n.T(lang, "admin.commerce.rejected_by_admin")
	}

	if h.promoSvc != nil {
		off, _ := h.promoSvc.GetSpecialOffer(ctx, id)
		if err := h.promoSvc.UpdateSpecialOfferAdminStatus(ctx, id, "rejected", notes, actor.UserID); err != nil {
			h.redirectWithNotice(w, r, "/admin/offers", "error", h.safeMessage(err, lang))
			return
		}
		_ = h.promoSvc.ToggleSpecialOfferStatus(ctx, id, false)
		if off != nil && off.OrganizationID > 0 {
			offTitle := off.Title.Get(i18n.Lang(lang))
			if offTitle == "" {
				offTitle = off.Title.Get(i18n.AR)
			}
			h.safeGo("notify-special-offer-rejected", func() {
				h.notifySpecialOfferStatus(context.Background(), off.OrganizationID, offTitle, false, notes)
			})
		}
	}

	h.redirectWithNotice(w, r, "/admin/offers", "success", i18n.T(lang, "admin.commerce.offer_rejected_success"))
}

// AdminOfferRequestChangesSubmit requests changes on a supplier special offer.
func (h *UIHandler) AdminOfferRequestChangesSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/offers", "error", i18n.T(lang, "admin.commerce.invalid_offer_id"))
		return
	}

	actor, _ := authctx.From(ctx)
	notes := strings.TrimSpace(r.PostFormValue("notes"))
	if notes == "" {
		notes = "يرجى مراجعة وتعديل بيانات العرض بناء على تعليمات الإدارة"
	}

	if h.promoSvc != nil {
		off, _ := h.promoSvc.GetSpecialOffer(ctx, id)
		if err := h.promoSvc.UpdateSpecialOfferAdminStatus(ctx, id, "changes_requested", notes, actor.UserID); err != nil {
			h.redirectWithNotice(w, r, "/admin/offers", "error", h.safeMessage(err, lang))
			return
		}
		_ = h.promoSvc.ToggleSpecialOfferStatus(ctx, id, false)
		if off != nil && off.OrganizationID > 0 {
			offTitle := off.Title.Get(i18n.Lang(lang))
			if offTitle == "" {
				offTitle = off.Title.Get(i18n.AR)
			}
			h.safeGo("notify-special-offer-changes-requested", func() {
				h.notifySpecialOfferStatus(context.Background(), off.OrganizationID, offTitle, false, notes)
			})
		}
	}

	h.redirectWithNotice(w, r, "/admin/offers", "success", "تم إرسال طلب التعديل إلى المورد مع الملاحظات بنجاح.")
}

// AdminOfferStatusSubmit activates or deactivates an offer.
func (h *UIHandler) AdminOfferStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.promoSvc != nil {
		isActive := r.PostFormValue("active") == "true"
		_ = h.promoSvc.ToggleSpecialOfferStatus(ctx, id, isActive)
	}
	h.redirectWithNotice(w, r, "/admin/offers", "success", i18n.T(langOf(r), "admin.commerce.offer_status_toggled_success"))
}

// AdminPolicyCreateSubmit creates a new draft version of a legal policy document.
func (h *UIHandler) AdminPolicyCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", i18n.T(lang, "admin.commerce.policy_service_unavailable"))
		return
	}

	p := &platformadmin.Policy{
		PolicyKey:   r.PostFormValue("policy_key"),
		Version:     r.PostFormValue("version"),
		Title:       i18n.New(r.PostFormValue("title_ar"), r.PostFormValue("title_en")),
		Content:     i18n.New(r.PostFormValue("content_ar"), r.PostFormValue("content_en")),
		Summary:     i18n.New(r.PostFormValue("summary_ar"), r.PostFormValue("summary_en")),
		IsPublished: r.PostFormValue("is_published") == "1",
		CreatedBy:   &actor.UserID,
	}

	if err := h.adminSvc.CreatePolicyVersion(ctx, p); err != nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/settings?tab=policies&key="+p.PolicyKey, "success", i18n.T(lang, "admin.commerce.policy_saved_success"))
}

// AdminPolicyPublishSubmit activates a specific policy version.
func (h *UIHandler) AdminPolicyPublishSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", i18n.T(lang, "admin.commerce.invalid_policy_id"))
		return
	}

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", i18n.T(lang, "admin.commerce.policy_service_unavailable"))
		return
	}

	if err := h.adminSvc.PublishPolicyVersion(ctx, id); err != nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "success", i18n.T(lang, "admin.commerce.policy_published_success"))
}
