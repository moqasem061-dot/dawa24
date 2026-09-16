package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// InvoicePrintPage renders the Egyptian-standard printable invoice for a specific invoice ID.
func (h *UIHandler) InvoicePrintPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}

	var invoice *billing.Invoice
	if h.billSvc != nil {
		inv, errGet := h.billSvc.GetInvoice(ctx, id)
		if errGet == nil && inv != nil {
			invoice = inv
		}
	}

	if invoice == nil {
		http.NotFound(w, r)
		return
	}

	printableData, errBuild := h.buildPrintableInvoiceData(ctx, invoice, nil, lang)
	if errBuild != nil {
		h.log.ErrorContext(ctx, "failed to build printable invoice data", "error", errBuild)
		http.Error(w, i18n.T(lang, "invoice.prepare_failed"), http.StatusInternalServerError)
		return
	}

	h.renderPage(ctx, w, "render invoice print page", pages.InvoicePrintablePage(*printableData, lang, dir))
}

// OrderInvoicePrintPage renders the Egyptian-standard printable invoice for a specific order ID.
func (h *UIHandler) OrderInvoicePrintPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}

	var order *commerce.Order
	if h.commSvc != nil {
		ord, errGet := h.commSvc.GetOrder(ctx, id)
		if errGet == nil && ord != nil {
			order = ord
		}
	}

	if order == nil {
		http.NotFound(w, r)
		return
	}

	if order.Status == commerce.StatusPending {
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", i18n.T(lang, "invoice.print_pending_error"))
		return
	}

	printableData, errBuild := h.buildPrintableInvoiceData(ctx, nil, order, lang)
	if errBuild != nil {
		h.log.ErrorContext(ctx, "failed to build printable invoice data for order", "error", errBuild)
		http.Error(w, i18n.T(lang, "invoice.prepare_failed"), http.StatusInternalServerError)
		return
	}

	h.renderPage(ctx, w, "render order invoice print page", pages.InvoicePrintablePage(*printableData, lang, dir))
}

func (h *UIHandler) synthesizeInvoiceFromOrder(ctx context.Context, order *commerce.Order) *billing.Invoice {
	vendorOrgID := int64(1)
	if len(order.Shipments) > 0 && order.Shipments[0].OrganizationID > 0 {
		vendorOrgID = order.Shipments[0].OrganizationID
	} else if order.VendorBranchID != nil {
		vendorOrgID = *order.VendorBranchID
	}
	newInv := &billing.Invoice{
		OrganizationID: vendorOrgID,
		CustomerOrgID:  order.OrganizationID,
		OrderID:        &order.ID,
		InvoiceNumber:  fmt.Sprintf("INV-%d-%05d", time.Now().Year(), order.ID),
		IssueDate:      order.CreatedAt,
		DueDate:        order.CreatedAt.AddDate(0, 0, 30),
		Subtotal:       order.Subtotal,
		TaxAmount:      order.TaxAmount,
		DiscountAmount: order.TotalDiscount,
		TotalAmount:    order.TotalAmount,
		Status:         billing.InvoicePaid,
		PaymentMethod:  string(order.PaymentMethod),
		Notes:          order.Notes,
	}
	if order.PaymentStatus != commerce.PaymentPaid {
		newInv.Status = billing.InvoiceIssued
	}
	for _, l := range order.Lines {
		newInv.Lines = append(newInv.Lines, billing.InvoiceLine{
			ProductID:   l.ProductID,
			Description: l.ProductName.Get("ar"),
			Quantity:    l.Quantity,
			UnitPrice:   l.UnitPrice,
			TotalPrice:  l.TotalPrice,
		})
	}
	savedInv, errCreate := h.billSvc.CreateInvoice(ctx, newInv)
	if errCreate == nil && savedInv != nil {
		return savedInv
	}
	return newInv
}

func (h *UIHandler) buildPrintableVendorInfo(ctx context.Context, invoice *billing.Invoice, lang any) billing.PrintableOrgInfo {
	vendorInfo := billing.PrintableOrgInfo{
		DisplayName:        i18n.T(lang, "invoice.default_vendor_display_name"),
		LegalName:          i18n.T(lang, "invoice.default_vendor_legal_name"),
		TaxNumber:          "100-245-890",
		CommercialRegister: "108920",
		Phone:              "0100002424",
		Address:            i18n.T(lang, "invoice.default_vendor_address"),
		City:               i18n.T(lang, "invoice.default_vendor_city"),
	}
	if invoice != nil && invoice.OrganizationID > 0 && h.orgSvc != nil {
		if vOrg, _ := h.orgSvc.GetOrganization(ctx, invoice.OrganizationID); vOrg != nil {
			vendorInfo.OrganizationID = vOrg.ID
			if vOrg.LegalName != "" {
				vendorInfo.LegalName = vOrg.LegalName
				vendorInfo.DisplayName = vOrg.LegalName
			}
			if vOrg.TradeName.Get("ar") != "" {
				vendorInfo.DisplayName = vOrg.TradeName.Get("ar")
			}
			if vOrg.TaxNumber != "" {
				vendorInfo.TaxNumber = vOrg.TaxNumber
			}
			if vOrg.CommercialRegister != "" {
				vendorInfo.CommercialRegister = vOrg.CommercialRegister
			}
		}
		if supp, err := h.orgSvc.GetSupplierProfile(ctx, invoice.OrganizationID); err == nil && supp != nil && supp.Image != "" {
			vendorInfo.LogoURL = supp.Image
		}
	}
	return vendorInfo
}

func (h *UIHandler) buildPrintableCustomerInfo(ctx context.Context, invoice *billing.Invoice, order *commerce.Order, lang any) billing.PrintableOrgInfo {
	custInfo := billing.PrintableOrgInfo{
		DisplayName:        i18n.T(lang, "invoice.default_customer_display_name"),
		LegalName:          i18n.T(lang, "invoice.default_customer_legal_name"),
		TaxNumber:          "400-123-789",
		CommercialRegister: "98201",
		PharmacistLicense:  "PH-2026/884",
		Phone:              "0110002424",
		Address:            i18n.T(lang, "invoice.default_customer_address"),
		City:               i18n.T(lang, "invoice.default_customer_city"),
	}
	var custOrgID *int64
	if invoice != nil && invoice.CustomerOrgID != nil {
		custOrgID = invoice.CustomerOrgID
	} else if order != nil && order.OrganizationID != nil {
		custOrgID = order.OrganizationID
	}
	if custOrgID != nil && *custOrgID > 0 && h.orgSvc != nil {
		if cOrg, _ := h.orgSvc.GetOrganization(ctx, *custOrgID); cOrg != nil {
			custInfo.OrganizationID = cOrg.ID
			if cOrg.OrganizationNumber != "" {
				custInfo.OrganizationNumber = cOrg.OrganizationNumber
			}
			if cOrg.LegalName != "" {
				custInfo.LegalName = cOrg.LegalName
				custInfo.DisplayName = cOrg.LegalName
			}
			if cOrg.TradeName.Get("ar") != "" {
				custInfo.DisplayName = cOrg.TradeName.Get("ar")
			}
			if cOrg.TaxNumber != "" {
				custInfo.TaxNumber = cOrg.TaxNumber
			}
			if cOrg.CommercialRegister != "" {
				custInfo.CommercialRegister = cOrg.CommercialRegister
			}
			if cOrg.PharmacistLicense != "" {
				custInfo.PharmacistLicense = cOrg.PharmacistLicense
			}
		}
		if invoice != nil && invoice.OrganizationID > 0 {
			if links, err := h.orgSvc.ListUserOrganizationsByVendor(ctx, invoice.OrganizationID, "approved"); err == nil {
				for _, link := range links {
					if link != nil && link.CustomerOrgID != nil && *link.CustomerOrgID == *custOrgID && link.OrganizationNumber != "" {
						custInfo.OrganizationNumber = link.OrganizationNumber
						break
					}
				}
			}
		}
	}

	if order != nil && len(order.Shipments) > 0 {
		sh := order.Shipments[0]
		if sh.CustomerBranchName.Get("ar") != "" {
			custInfo.DisplayName = sh.CustomerBranchName.Get("ar")
		}
		if sh.CustomerBranchAddress != "" {
			custInfo.Address = sh.CustomerBranchAddress
		}
		if sh.CustomerBranchPhone != "" {
			custInfo.Phone = sh.CustomerBranchPhone
		}
	}
	return custInfo
}
