package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// purchaseBody is the body of POST /orders/purchase. Quantity and unitPrice stay
// raw so a fraction, a string quantity or a number unitPrice is judged here, not
// silently coerced.
type purchaseBody struct {
	ExternalReference string             `json:"externalReference"`
	Notes             string             `json:"notes"`
	Lines             []purchaseLineBody `json:"lines"`
}

// purchaseLineBody keeps variantId, code and its aliases raw: the variant id may
// be a JSON string or an integer, and the item code a string or a number.
// lineCodeRaw (purchase_lines.go) picks the first of code, itemId and itemCode.
type purchaseLineBody struct {
	VariantID json.RawMessage `json:"variantId"`
	Quantity  json.RawMessage `json:"quantity"`
	UnitPrice json.RawMessage `json:"unitPrice"`
	Code      json.RawMessage `json:"code"`
	ItemID    json.RawMessage `json:"itemId"`
	ItemCode  json.RawMessage `json:"itemCode"`
}

// request turns the body into the service request. A quantity that is not a whole
// JSON number becomes 0, which the service refuses as ERP_PO_QUANTITY_INVALID with
// the line number. An unitPrice that is neither a string nor a number is refused here.
func (b purchaseBody) request() (erp.CreatePurchaseRequest, *erp.Error) {
	req := erp.CreatePurchaseRequest{ExternalReference: b.ExternalReference, Notes: b.Notes}
	for i, line := range b.Lines {
		qty, err := strconv.Atoi(string(line.Quantity))
		if err != nil {
			qty = 0
		}
		variantID, ok := variantIDText(line.VariantID)
		if !ok {
			return erp.CreatePurchaseRequest{}, erp.NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"variantId"}, "line": i + 1})
		}
		item := erp.PurchaseLine{VariantID: variantID, Quantity: qty}
		if price := strings.Trim(string(line.UnitPrice), `"`); len(line.UnitPrice) > 0 && string(line.UnitPrice) != "null" {
			parsed, err := money.Parse(price)
			if err != nil {
				return erp.CreatePurchaseRequest{}, erp.NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"unitPrice"}, "line": i + 1})
			}
			item.UnitPrice = &parsed
		}
		code, present, ok := lineCode(line)
		if present && !ok {
			return erp.CreatePurchaseRequest{}, erp.NewError("ERP_VALIDATION", 422, map[string]any{"field": "lines[" + strconv.Itoa(i+1) + "].code"})
		}
		item.Code = code
		req.Lines = append(req.Lines, item)
	}
	return req, nil
}

// getOffers lists the supplier products with the barcode, cheapest first.
func (h *Handler) getOffers(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, r, erp.NewError("ERP_AUTH_MISSING", 401, nil))
		return
	}
	offers, err := h.svc.Offers(r.Context(), principal, r.URL.Query().Get("barcode"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, http.StatusOK, offersJSON(offers))
}

// postPurchase places the order, or returns the stored order for a reference
// the ERP already sent: 201 for a new order, 200 for a replay.
func (h *Handler) postPurchase(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, r, erp.NewError("ERP_AUTH_MISSING", 401, nil))
		return
	}
	raw, err := readBody(w, r)
	if err != nil {
		return // readBody already wrote the envelope
	}
	var body purchaseBody
	if err := json.Unmarshal(raw, &body); err != nil {
		writeError(w, r, erp.NewError("ERP_BAD_JSON", 400, map[string]any{"position": 0}))
		return
	}
	req, bad := body.request()
	if bad != nil {
		writeError(w, r, bad)
		return
	}
	res, err := h.svc.CreatePurchaseOrder(r.Context(), principal, req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if res.Replay {
		status = http.StatusOK
	}
	writeOK(w, r, status, purchaseJSON(res.Row))
}

// offersJSON is the GET /offers item shape of the contract (OfferList).
func offersJSON(offers []erp.Offer) []map[string]any {
	out := make([]map[string]any, 0, len(offers))
	for _, o := range offers {
		out = append(out, map[string]any{
			"variantId":       o.VariantID,
			"supplier":        map[string]any{"name": o.SupplierName, "linked": o.SupplierLinked},
			"name":            o.Name,
			"barcode":         o.Barcode,
			"publicPrice":     o.PublicPrice,
			"discountPercent": o.DiscountPercent,
			"unitPrice":       o.UnitPrice,
			"inStock":         o.InStock,
		})
	}
	return out
}
