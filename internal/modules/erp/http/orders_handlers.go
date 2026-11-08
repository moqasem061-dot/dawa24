package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
)

// The order feeds (S3): sales orders are the supplier's own shipments, purchase
// orders are the buyer's own orders. Every handler takes the organization from
// the principal. A successful sales read is flagged as sensitive, so the call
// log shows who read customer data.

func (h *Handler) listSales(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, r, erp.NewError("ERP_AUTH_MISSING", 401, nil))
		return
	}
	filter, err := orderFilterFromQuery(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	page, err := h.svc.ListSales(r.Context(), principal, filter)
	if err != nil {
		writeError(w, r, err)
		return
	}
	markSensitiveRead(r.Context())
	rows := make([]map[string]any, 0, len(page.Rows))
	for _, row := range page.Rows {
		rows = append(rows, salesJSON(row))
	}
	writePage(w, r, rows, pageJSON(page.Limit, page.NextAfter))
}

func (h *Handler) getSale(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, r, erp.NewError("ERP_AUTH_MISSING", 401, nil))
		return
	}
	row, err := h.svc.GetSales(r.Context(), principal, chi.URLParam(r, "orderNumber"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	markSensitiveRead(r.Context())
	writeOK(w, r, http.StatusOK, salesJSON(row))
}

func (h *Handler) listPurchase(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, r, erp.NewError("ERP_AUTH_MISSING", 401, nil))
		return
	}
	filter, err := orderFilterFromQuery(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	page, err := h.svc.ListPurchase(r.Context(), principal, filter)
	if err != nil {
		writeError(w, r, err)
		return
	}
	rows := make([]map[string]any, 0, len(page.Rows))
	for _, row := range page.Rows {
		rows = append(rows, purchaseJSON(row))
	}
	writePage(w, r, rows, pageJSON(page.Limit, page.NextAfter))
}

func (h *Handler) getPurchase(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, r, erp.NewError("ERP_AUTH_MISSING", 401, nil))
		return
	}
	row, err := h.svc.GetPurchase(r.Context(), principal, chi.URLParam(r, "orderNumber"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, http.StatusOK, purchaseJSON(row))
}

func (h *Handler) ackSales(w http.ResponseWriter, r *http.Request) { h.ack(w, r, "sales") }

func (h *Handler) ackPurchase(w http.ResponseWriter, r *http.Request) { h.ack(w, r, "purchase") }

// ack decodes the body through the key's provider (the canonical body and the
// provider's own shapes both reach the service) and reports one result per order.
func (h *Handler) ack(w http.ResponseWriter, r *http.Request, direction string) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, r, erp.NewError("ERP_AUTH_MISSING", 401, nil))
		return
	}
	body, err := readBody(w, r)
	if err != nil {
		return // readBody already wrote the envelope
	}
	prov, ok := provider.Get(principal.Provider)
	if !ok {
		writeError(w, r, erp.NewError("ERP_INTERNAL", 500, nil))
		return
	}
	entries, err := prov.DecodeAck(body)
	if err != nil {
		writeError(w, r, erp.NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"orders"}}))
		return
	}
	results, err := h.svc.Ack(r.Context(), principal, direction, entries)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(results))
	for _, res := range results {
		out = append(out, map[string]any{
			"orderNumber":       res.OrderNumber,
			"result":            res.Result,
			"code":              nullable(res.Code),
			"existingReference": nullable(res.ExistingReference),
		})
	}
	writeOK(w, r, http.StatusOK, out)
}

// orderFilterFromQuery reads cursor, limit, state, from and to. Range checks
// that depend on policy (limit bounds, state names) belong to the service.
func orderFilterFromQuery(r *http.Request) (erp.OrderFilter, error) {
	var f erp.OrderFilter
	after, err := DecodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		return f, err
	}
	f.After = after
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return f, erp.NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"limit"}})
		}
		f.Limit = limit
	}
	f.State = r.URL.Query().Get("state")
	if f.From, err = parseTimeParam(r, "from"); err != nil {
		return f, err
	}
	if f.To, err = parseTimeParam(r, "to"); err != nil {
		return f, err
	}
	return f, nil
}

func parseTimeParam(r *http.Request, name string) (*time.Time, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, erp.NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{name}})
	}
	return &t, nil
}

// writePage is the orders envelope: data and a page object beside it.
func writePage(w http.ResponseWriter, r *http.Request, data any, page map[string]any) {
	id := requestID(w, r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "requestId": id, "data": data, "page": page})
}

func pageJSON(limit int, nextAfter int64) map[string]any {
	var next any
	if nextAfter > 0 {
		next = EncodeCursor(nextAfter)
	}
	return map[string]any{"nextCursor": next, "limit": limit}
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func erpJSON(reference string, ackedAt *time.Time) map[string]any {
	return map[string]any{"reference": nullable(reference), "acknowledgedAt": timeOrNil(ackedAt)}
}

func customerJSON(c erp.CustomerInfo) map[string]any {
	return map[string]any{
		"erpCode":        nullable(c.ErpCode),
		"mapped":         c.Mapped,
		"unmappedReason": nullable(c.UnmappedReason),
		"name":           c.Name,
		"branchName":     c.BranchName,
		"address":        c.Address,
		"phone":          c.Phone,
	}
}

func linesJSON(lines []erp.OrderLine) []map[string]any {
	out := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		out = append(out, map[string]any{
			"lineId":              l.LineID,
			"variantId":           nullable(l.VariantPublicID),
			"externalProductCode": nullable(l.ExternalProductCode),
			"name":                l.Name,
			"barcode":             l.Barcode,
			"quantity":            l.Quantity,
			"unitPrice":           l.UnitPrice,
			"discountAmount":      l.DiscountAmount,
			"totalPrice":          l.TotalPrice,
		})
	}
	return out
}

func salesJSON(row erp.SalesRow) map[string]any {
	return map[string]any{
		"orderNumber":    row.OrderNumber,
		"shipmentNumber": row.ShipmentNumber,
		"status":         row.Status,
		"importState":    row.ImportState,
		"createdAt":      row.CreatedAt,
		"updatedAt":      row.UpdatedAt,
		"paymentMethod":  row.PaymentMethod,
		"paymentStatus":  row.PaymentStatus,
		"notes":          row.Notes,
		"subtotal":       row.Subtotal,
		"shippingFee":    row.ShippingFee,
		"total":          row.Total,
		"customer":       customerJSON(row.Customer),
		"erp":            erpJSON(row.ErpReference, row.AckedAt),
		"lines":          linesJSON(row.Lines),
	}
}

func purchaseJSON(row erp.PurchaseRow) map[string]any {
	shipments := make([]map[string]any, 0, len(row.Shipments))
	for _, sh := range row.Shipments {
		shipments = append(shipments, map[string]any{
			"shipmentNumber": sh.ShipmentNumber,
			"status":         sh.Status,
			"supplier":       map[string]any{"name": sh.SupplierName},
			"subtotal":       sh.Subtotal,
			"shippingFee":    sh.ShippingFee,
			"lines":          linesJSON(sh.Lines),
		})
	}
	return map[string]any{
		"orderNumber":       row.OrderNumber,
		"status":            row.Status,
		"importState":       row.ImportState,
		"createdAt":         row.CreatedAt,
		"updatedAt":         row.UpdatedAt,
		"externalReference": nullable(row.ExternalReference),
		"total":             row.Total,
		"shipments":         shipments,
		"erp":               erpJSON(row.ErpReference, row.AckedAt),
	}
}
