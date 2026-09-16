package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
)

// resolveInvoiceDataByParam fetches printable data by URL ID parameter.
func (h *UIHandler) resolveInvoiceDataByParam(r *http.Request, isOrder bool) (*billing.PrintableInvoiceData, error) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("invalid id")
	}

	var invoice *billing.Invoice
	var order *commerce.Order

	if isOrder {
		if h.commSvc != nil {
			order, _ = h.commSvc.GetOrder(ctx, id)
		}
		if order == nil {
			return nil, fmt.Errorf("order not found")
		}
	} else {
		if h.billSvc != nil {
			inv, errGet := h.billSvc.GetInvoice(ctx, id)
			if errGet == nil && inv != nil {
				invoice = inv
			}
		}
		if invoice == nil {
			return nil, fmt.Errorf("invoice not found")
		}
	}

	return h.buildPrintableInvoiceData(ctx, invoice, order, lang)
}

// InvoiceExportExcel streams an invoice as an Excel file.
func (h *UIHandler) InvoiceExportExcel(w http.ResponseWriter, r *http.Request) {
	data, err := h.resolveInvoiceDataByParam(r, false)
	if err != nil || data == nil {
		http.NotFound(w, r)
		return
	}

	filename := fmt.Sprintf("invoice_%s_%s.xlsx", data.InvoiceNumber, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	if err := GenerateInvoiceExcel(data, w); err != nil {
		h.log.ErrorContext(r.Context(), "generate invoice excel", "error", err)
	}
}

// InvoiceExportWord streams an invoice as a Word .docx document.
func (h *UIHandler) InvoiceExportWord(w http.ResponseWriter, r *http.Request) {
	data, err := h.resolveInvoiceDataByParam(r, false)
	if err != nil || data == nil {
		http.NotFound(w, r)
		return
	}

	filename := fmt.Sprintf("invoice_%s_%s.docx", data.InvoiceNumber, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	if err := GenerateInvoiceWord(data, w); err != nil {
		h.log.ErrorContext(r.Context(), "generate invoice word", "error", err)
	}
}

// OrderInvoiceExportExcel streams an order invoice as an Excel file.
func (h *UIHandler) OrderInvoiceExportExcel(w http.ResponseWriter, r *http.Request) {
	data, err := h.resolveInvoiceDataByParam(r, true)
	if err != nil || data == nil {
		http.NotFound(w, r)
		return
	}

	filename := fmt.Sprintf("order_invoice_%s_%s.xlsx", data.InvoiceNumber, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	if err := GenerateInvoiceExcel(data, w); err != nil {
		h.log.ErrorContext(r.Context(), "generate order invoice excel", "error", err)
	}
}

// OrderInvoiceExportWord streams an order invoice as a Word .docx document.
func (h *UIHandler) OrderInvoiceExportWord(w http.ResponseWriter, r *http.Request) {
	data, err := h.resolveInvoiceDataByParam(r, true)
	if err != nil || data == nil {
		http.NotFound(w, r)
		return
	}

	filename := fmt.Sprintf("order_invoice_%s_%s.docx", data.InvoiceNumber, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	if err := GenerateInvoiceWord(data, w); err != nil {
		h.log.ErrorContext(r.Context(), "generate order invoice word", "error", err)
	}
}
