package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// InvoicesPage renders the vendor's invoice list with status badges and actions.
func (h *UIHandler) InvoicesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/invoices", http.StatusSeeOther)
		return
	}

	// If pharmacy customer visits /invoices, redirect them to /orders since invoices are printed from order details
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/orders", "info", i18n.T(lang, "invoice.customer_redirect_info"))
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	dateFrom := strings.TrimSpace(r.URL.Query().Get("date_from"))
	dateTo := strings.TrimSpace(r.URL.Query().Get("date_to"))
	sortBy := strings.TrimSpace(r.URL.Query().Get("sort"))
	sortOrder := strings.TrimSpace(r.URL.Query().Get("order"))
	branchIDStr := strings.TrimSpace(r.URL.Query().Get("branch_id"))
	var branchID *int64
	var selectedBranchID int64
	if bID, err := strconv.ParseInt(branchIDStr, 10, 64); err == nil && bID > 0 {
		branchID = &bID
		selectedBranchID = bID
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var orgID *int64
	if !actor.IsStaff && actor.OrganizationID > 0 {
		orgID = &actor.OrganizationID
	}

	var vendorBranches []*org.Branch
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		if bList, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
			vendorBranches = bList
		}
	}

	var detailedInvoices []*billing.AdminInvoiceView
	var totalCount int
	if h.billSvc != nil {
		invoices, total, err := h.billSvc.AdminListDetailedInvoices(ctx, billing.InvoiceFilter{
			Search:         q,
			Status:         status,
			OrganizationID: orgID,
			BranchID:       branchID,
			DateFrom:       dateFrom,
			DateTo:         dateTo,
			SortBy:         sortBy,
			SortOrder:      sortOrder,
			Limit:          limit,
			Offset:         offset,
		})
		if err != nil {
			h.log.WarnContext(ctx, "account: list detailed invoices", "error", err)
		} else {
			detailedInvoices = invoices
			totalCount = total
		}
	}

	// Fetch recent orders for vendor to populate the "Link Invoice to Order" modal
	var vendorOrders []pages.VendorOrderOption
	if h.commSvc != nil && actor.OrganizationID > 0 {
		if shipments, _, err := h.commSvc.ListVendorShipmentsWithTotal(ctx, actor.OrganizationID, "", 50, 0); err == nil {
			seenOrders := make(map[int64]bool)
			for _, s := range shipments {
				if s != nil && s.OrderID > 0 && !seenOrders[s.OrderID] {
					seenOrders[s.OrderID] = true
					custName := ""
					var ord *commerce.Order
					if o, _ := h.commSvc.GetOrder(ctx, s.OrderID); o != nil {
						ord = o
						if h.orgSvc != nil && ord.OrganizationID != nil && *ord.OrganizationID > 0 {
							if cOrg, _ := h.orgSvc.GetOrganization(ctx, *ord.OrganizationID); cOrg != nil {
								custName = cOrg.LegalName
								if custName == "" {
									custName = cOrg.TradeName.Get("ar")
								}
							}
						}
					}
					orderNum := s.ShipmentNumber
					var subtotal, taxAmount money.Amount
					if ord != nil {
						if ord.OrderNumber != "" {
							orderNum = ord.OrderNumber
						}
						subtotal = ord.Subtotal
						taxAmount = ord.TaxAmount
					} else {
						subtotal = s.TotalAmount
					}
					vendorOrders = append(vendorOrders, pages.VendorOrderOption{
						ID:           s.OrderID,
						OrderNumber:  orderNum,
						CustomerName: custName,
						TotalAmount:  s.TotalAmount,
						Subtotal:     subtotal,
						TaxAmount:    taxAmount,
					})
				}
			}
		}
	}

	data := pages.InvoicesData{
		Invoices:         detailedInvoices,
		Search:           q,
		StatusFilter:     status,
		DateFrom:         dateFrom,
		DateTo:           dateTo,
		SortBy:           sortBy,
		SortOrder:        sortOrder,
		Branches:         vendorBranches,
		SelectedBranchID: selectedBranchID,
		VendorOrders:     vendorOrders,
		IsVendor:         true,
		Page:             page,
		PerPage:          limit,
		TotalCount:       totalCount,
	}

	h.renderPage(ctx, w, "render invoices page", pages.InvoicesPage(lang, dir, data))
}
