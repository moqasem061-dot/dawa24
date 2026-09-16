package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// VendorInvoiceCreateSubmit handles vendor creation of an invoice linked to an order with dynamic amounts.
func (h *UIHandler) VendorInvoiceCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/invoices", http.StatusSeeOther)
		return
	}

	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/invoices", "error", "غير مصرح لحساب الصيدلية بإصدار الفواتير - إصدار الفواتير وسندات التوريد مخصص للموردين فقط")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/invoices", "error", "تعذر قراءة بيانات النموذج")
		return
	}

	orderInput := strings.TrimSpace(r.PostFormValue("order_id"))
	if orderInput == "" {
		orderInput = strings.TrimSpace(r.PostFormValue("order_number"))
	}
	if orderInput == "" {
		h.redirectWithNotice(w, r, "/invoices", "error", "يرجى تحديد أو إدخال رقم الطلب لربط الفاتورة به")
		return
	}

	if h.commSvc == nil || h.billSvc == nil {
		h.redirectWithNotice(w, r, "/invoices", "error", "خدمة الفواتير غير متاحة حالياً")
		return
	}

	// Retrieve order
	var order *commerce.Order
	var errOrder error
	if orderID, err := strconv.ParseInt(orderInput, 10, 64); err == nil && orderID > 0 {
		order, errOrder = h.commSvc.GetOrder(ctx, orderID)
	}
	if order == nil {
		order, errOrder = h.commSvc.GetOrderByNumber(ctx, orderInput)
	}

	if errOrder != nil || order == nil {
		h.redirectWithNotice(w, r, "/invoices", "error", "لم يتم العثور على الطلب المحدد أو رقم الطلب غير صحيح")
		return
	}

	// Verify authorization: if not staff, vendor must own shipment or vendor branch in this order
	vendorOrgID := actor.OrganizationID
	if !actor.IsStaff && vendorOrgID > 0 {
		authorized := false
		for _, sh := range order.Shipments {
			if sh != nil && sh.OrganizationID == vendorOrgID {
				authorized = true
				break
			}
		}
		if !authorized && order.VendorBranchID != nil && h.orgSvc != nil {
			if b, err := h.orgSvc.GetBranch(ctx, *order.VendorBranchID); err == nil && b != nil && b.OrganizationID == vendorOrgID {
				authorized = true
			}
		}
		if !authorized {
			h.redirectWithNotice(w, r, "/invoices", "error", "ليس لديك صلاحية لإصدار فاتورة لهذا الطلب (الطلب لا يتبع لمنشأتك)")
			return
		}
	} else if vendorOrgID == 0 && len(order.Shipments) > 0 {
		vendorOrgID = order.Shipments[0].OrganizationID
	}

	// Check if invoice already exists for this order
	if existing, err := h.billSvc.GetInvoiceByOrderID(ctx, order.ID); err == nil && existing != nil {
		h.redirectWithNotice(w, r, "/invoices", "info", fmt.Sprintf("هذا الطلب مرتبط بالفعل بفاتورة سابقة رقم: %s", existing.InvoiceNumber))
		return
	}

	// Custom invoice number or auto-generate
	invNumber := strings.TrimSpace(r.PostFormValue("invoice_number"))
	if invNumber == "" {
		invNumber = fmt.Sprintf("INV-%d-%05d", time.Now().Year(), order.ID)
	}

	// Dynamic Amounts: default to order amounts, override if custom value entered
	totalAmount := order.TotalAmount
	if totalStr := strings.TrimSpace(r.PostFormValue("total_amount")); totalStr != "" {
		if parsedTotal, err := money.Parse(totalStr); err == nil && parsedTotal.Minor() > 0 {
			totalAmount = parsedTotal
		}
	}

	subtotal := order.Subtotal
	if subStr := strings.TrimSpace(r.PostFormValue("subtotal")); subStr != "" {
		if parsedSub, err := money.Parse(subStr); err == nil && parsedSub.Minor() >= 0 {
			subtotal = parsedSub
		}
	} else if totalAmount.Minor() != order.TotalAmount.Minor() {
		subtotal = totalAmount
	}

	taxAmount := order.TaxAmount
	if taxStr := strings.TrimSpace(r.PostFormValue("tax_amount")); taxStr != "" {
		if parsedTax, err := money.Parse(taxStr); err == nil && parsedTax.Minor() >= 0 {
			taxAmount = parsedTax
		}
	}

	// Dates
	issueDate := time.Now().UTC()
	if issueDateStr := strings.TrimSpace(r.PostFormValue("issue_date")); issueDateStr != "" {
		if t, err := time.Parse("2006-01-02", issueDateStr); err == nil {
			issueDate = t
		}
	}
	dueDate := issueDate.AddDate(0, 0, 30)
	if dueDateStr := strings.TrimSpace(r.PostFormValue("due_date")); dueDateStr != "" {
		if t, err := time.Parse("2006-01-02", dueDateStr); err == nil {
			dueDate = t
		}
	}

	statusStr := strings.TrimSpace(r.PostFormValue("status"))
	status := billing.InvoiceIssued
	if statusStr == "paid" {
		status = billing.InvoicePaid
	}

	notes := strings.TrimSpace(r.PostFormValue("notes"))
	if notes == "" {
		notes = order.Notes
	}

	paymentMethod := string(order.PaymentMethod)
	if pm := strings.TrimSpace(r.PostFormValue("payment_method")); pm != "" {
		paymentMethod = pm
	}

	newInv := &billing.Invoice{
		OrganizationID: vendorOrgID,
		CustomerOrgID:  order.OrganizationID,
		OrderID:        &order.ID,
		InvoiceNumber:  invNumber,
		IssueDate:      issueDate,
		DueDate:        dueDate,
		Subtotal:       subtotal,
		TaxAmount:      taxAmount,
		DiscountAmount: order.TotalDiscount,
		TotalAmount:    totalAmount,
		Status:         status,
		PaymentMethod:  paymentMethod,
		Notes:          notes,
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

	createdInv, err := h.billSvc.CreateInvoice(ctx, newInv)
	if err != nil {
		h.log.ErrorContext(ctx, "vendor create invoice failed", "error", err, "order_id", order.ID)
		h.redirectWithNotice(w, r, "/invoices", "error", h.safeMessage(err, lang))
		return
	}

	// If marked as paid, record full payment immediately so paid_amount matches total_amount
	if status == billing.InvoicePaid && createdInv != nil {
		payReq := billing.RecordInvoicePaymentRequest{
			InvoiceID:       createdInv.ID,
			OrganizationID:  vendorOrgID,
			UserID:          actor.UserID,
			Amount:          createdInv.TotalAmount,
			Method:          paymentMethod,
			ReferenceNumber: createdInv.InvoiceNumber,
			Notes:           "سداد مسجل تلقائياً عند إصدار الفاتورة",
		}
		if _, payErr := h.billSvc.RecordInvoicePayment(ctx, payReq); payErr != nil {
			h.log.WarnContext(ctx, "failed to record initial payment for paid invoice", "invoice_id", createdInv.ID, "error", payErr)
		}
	}

	h.redirectWithNotice(w, r, "/invoices", "success", fmt.Sprintf("تم إنشاء الفاتورة رقم %s بمبلغ %s ج.م وربطها بالطلب بنجاح", newInv.InvoiceNumber, newInv.TotalAmount.String()))
}
