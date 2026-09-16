package ui

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// parseDateParam keeps only valid YYYY-MM-DD values so a malformed query can
// neither break the SQL layer nor silently filter everything out.
func parseDateParam(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if _, err := time.Parse("2006-01-02", v); err != nil {
		return ""
	}
	return v
}

// VendorPaymentsPage renders the payments received by the vendor.
func (h *UIHandler) VendorPaymentsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/payments", http.StatusSeeOther)
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	customer := strings.TrimSpace(r.URL.Query().Get("customer"))
	method := strings.TrimSpace(r.URL.Query().Get("method"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	dateFrom := parseDateParam(r.URL.Query().Get("from"))
	dateTo := parseDateParam(r.URL.Query().Get("to"))

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var payments []*billing.AdminPaymentView
	var total int
	var stats *billing.VendorPaymentStats
	var openInvoices []*billing.AdminInvoiceView
	var customerOrgs []*billing.CustomerOrgSummary

	if h.billSvc != nil {
		s, err := h.billSvc.GetVendorPaymentStats(ctx, actor.OrganizationID)
		if err == nil {
			stats = s
		}

		if cOrgs, err := h.billSvc.ListVendorCustomerOrgs(ctx, actor.OrganizationID); err == nil {
			customerOrgs = cOrgs
		}

		filter := billing.PaymentFilter{
			OrganizationID: &actor.OrganizationID,
			Search:         search,
			CustomerSearch: customer,
			Method:         method,
			Status:         status,
			DateFrom:       dateFrom,
			DateTo:         dateTo,
			Limit:          limit,
			Offset:         offset,
		}
		pList, count, err := h.billSvc.ListDetailedPayments(ctx, filter)
		if err == nil {
			payments = pList
			total = count
		}

		invList, _, _ := h.billSvc.ListDetailedInvoices(ctx, billing.InvoiceFilter{
			OrganizationID: &actor.OrganizationID,
			Limit:          100,
		})
		for _, inv := range invList {
			if inv.Status != billing.InvoicePaid && inv.Status != billing.InvoiceCancelled {
				openInvoices = append(openInvoices, inv)
			}
		}
	}

	if stats == nil {
		stats = &billing.VendorPaymentStats{}
	}

	data := pages.VendorPaymentsPageData{
		Payments:     payments,
		Invoices:     openInvoices,
		Stats:        stats,
		Search:       search,
		Customer:     customer,
		CustomerOrgs: customerOrgs,
		Method:       method,
		Status:       status,
		DateFrom:     dateFrom,
		DateTo:       dateTo,
		Page:         page,
		PerPage:      limit,
		TotalCount:   total,
		Lang:         lang,
		Dir:          dir,
	}

	h.renderPage(ctx, w, "render vendor payments", pages.VendorPaymentsPage(data))
}

// VendorRecordPaymentSubmit records a payment against a vendor invoice.
func (h *UIHandler) VendorRecordPaymentSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	dest := r.PostFormValue("redirect_to")
	if dest == "" {
		dest = "/vendor/payments"
	}

	actor, ok := authctx.From(ctx)
	if !ok || (actor.OrganizationID <= 0 && !actor.IsStaff) {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(dest), http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "vendor_finance.flash.form_parse_error"))
		return
	}

	invoiceID, err := strconv.ParseInt(r.PostFormValue("invoice_id"), 10, 64)
	if err != nil || invoiceID <= 0 {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "vendor_finance.flash.invalid_invoice"))
		return
	}

	invoiceOrgID := actor.OrganizationID
	if actor.IsStaff || invoiceOrgID <= 0 {
		if h.billSvc != nil {
			if inv, errGet := h.billSvc.GetInvoice(ctx, invoiceID); errGet == nil && inv != nil {
				invoiceOrgID = inv.OrganizationID
			}
		}
	}

	amountStr := strings.TrimSpace(r.PostFormValue("amount"))
	amt, err := money.Parse(amountStr)
	if err != nil || amt.Minor() <= 0 {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "vendor_finance.flash.invalid_amount"))
		return
	}

	method := strings.TrimSpace(r.PostFormValue("method"))
	if method == "" {
		method = "bank_transfer"
	}

	refNum := strings.TrimSpace(r.PostFormValue("reference_number"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	req := billing.RecordInvoicePaymentRequest{
		InvoiceID:       invoiceID,
		OrganizationID:  invoiceOrgID,
		UserID:          actor.UserID,
		Amount:          amt,
		Method:          method,
		ReferenceNumber: refNum,
		Notes:           notes,
	}

	_, err = h.billSvc.RecordInvoicePayment(ctx, req)
	if err != nil {
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "vendor_finance.flash.record_success"))
}

// VendorEarningsOrderPage renders orders revenue and comprehensive net profit report for the vendor.
func (h *UIHandler) VendorEarningsOrderPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/earnings/order", http.StatusSeeOther)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}

	dateFrom := parseDateParam(r.URL.Query().Get("from"))
	if dateFrom == "" {
		dateFrom = parseDateParam(r.URL.Query().Get("date_from"))
	}
	dateTo := parseDateParam(r.URL.Query().Get("to"))
	if dateTo == "" {
		dateTo = parseDateParam(r.URL.Query().Get("date_to"))
	}
	customerOrgID, _ := strconv.ParseInt(r.URL.Query().Get("customer_org_id"), 10, 64)
	if customerOrgID <= 0 {
		customerOrgID, _ = strconv.ParseInt(r.URL.Query().Get("org_id"), 10, 64)
	}

	finFilter := commerce.VendorFinancialFilter{
		DateFrom:      dateFrom,
		DateTo:        dateTo,
		CustomerOrgID: customerOrgID,
	}

	var summary *commerce.VendorFinancialSummary
	if h.commSvc != nil {
		summary, _ = h.commSvc.GetVendorFinancialSummary(ctx, actor.OrganizationID, period, finFilter)
	}
	if summary == nil {
		summary = &commerce.VendorFinancialSummary{
			Period:        period,
			DateFrom:      dateFrom,
			DateTo:        dateTo,
			CustomerOrgID: customerOrgID,
		}
	}

	var customerOrgs []*billing.CustomerOrgSummary
	if h.billSvc != nil {
		if orgs, err := h.billSvc.ListVendorCustomerOrgs(ctx, actor.OrganizationID); err == nil {
			customerOrgs = orgs
		}
	}

	pageOrders := parseIntDefault(r.URL.Query().Get("page_orders"), 1)
	limitOrders := parseIntDefault(r.URL.Query().Get("limit_orders"), 15)
	if limitOrders <= 0 {
		limitOrders = 15
	}
	totalOrders := len(summary.Shipments)
	var pagedShipments []*commerce.VendorShipmentProfit
	if totalOrders > 0 {
		start := (pageOrders - 1) * limitOrders
		if start < 0 {
			start = 0
		}
		if start < totalOrders {
			end := start + limitOrders
			if end > totalOrders {
				end = totalOrders
			}
			pagedShipments = summary.Shipments[start:end]
		}
	}

	ordersPagination := components.PaginationProps{
		CurrentPage:     pageOrders,
		PageSize:        limitOrders,
		TotalCount:      totalOrders,
		BaseURL:         "/vendor/earnings/order",
		QueryValues:     r.URL.Query(),
		PageParam:       "page_orders",
		SizeParam:       "limit_orders",
		PageSizeOptions: []int{10, 15, 25, 50, 100},
	}

	pageProducts := parseIntDefault(r.URL.Query().Get("page_products"), 1)
	limitProducts := parseIntDefault(r.URL.Query().Get("limit_products"), 15)
	if limitProducts <= 0 {
		limitProducts = 15
	}
	totalProducts := len(summary.TopProducts)
	var pagedProducts []*commerce.VendorProductProfit
	if totalProducts > 0 {
		start := (pageProducts - 1) * limitProducts
		if start < 0 {
			start = 0
		}
		if start < totalProducts {
			end := start + limitProducts
			if end > totalProducts {
				end = totalProducts
			}
			pagedProducts = summary.TopProducts[start:end]
		}
	}

	productsPagination := components.PaginationProps{
		CurrentPage:     pageProducts,
		PageSize:        limitProducts,
		TotalCount:      totalProducts,
		BaseURL:         "/vendor/earnings/order",
		QueryValues:     r.URL.Query(),
		PageParam:       "page_products",
		SizeParam:       "limit_products",
		PageSizeOptions: []int{10, 15, 25, 50, 100},
	}

	pageData := pages.VendorEarningsOrderPageData{
		Summary:            summary,
		PagedShipments:     pagedShipments,
		OrdersPagination:   ordersPagination,
		PagedProducts:      pagedProducts,
		ProductsPagination: productsPagination,
		CustomerOrgs:       customerOrgs,
		DateFrom:           dateFrom,
		DateTo:             dateTo,
		CustomerOrgID:      customerOrgID,
		Lang:               lang,
		Dir:                dir,
	}

	h.renderPage(ctx, w, "render vendor earnings order", pages.VendorEarningsOrderPage(pageData))
}

func parseIntDefault(v string, def int) int {
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// VendorEarningsOffersPage renders offers revenue and commissions for the vendor.
func (h *UIHandler) VendorEarningsOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/earnings/offers", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render vendor earnings offers", pages.VendorEarningsOffersPage(money.Zero, lang, dir))
}

// VendorOfferOrdersPage renders offer-based orders fulfilled by the vendor.
func (h *UIHandler) VendorOfferOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders/offers", http.StatusSeeOther)
		return
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var shipments []*commerce.OrderShipment
	var total int
	if h.commSvc != nil {
		shipments, total, _ = h.commSvc.ListVendorShipmentsWithTotal(ctx, actor.OrganizationID, "", limit, offset)
	}

	h.renderPage(ctx, w, "render vendor offer orders", pages.VendorOfferOrdersPage(shipments, lang, dir, page, limit, total))
}

// VendorOfferOrderDetailPage renders single offer order details.
func (h *UIHandler) VendorOfferOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	shipID, _ := strconv.ParseInt(idStr, 10, 64)

	h.renderPage(ctx, w, "render vendor offer order detail", pages.VendorOfferOrderDetailPage(shipID, lang, dir))
}
