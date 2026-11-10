package http

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// The Orgasoft order feed (2026-11-10). Same scopes, filters and paging as the
// order feeds, but the body is a bare JSON array in the shape Orgasoft asked
// for, and the next page is in the X-Next-Cursor header. Field order and number
// text are part of that contract: money is written as the raw decimal the
// database holds, so 265.00 stays 265.00.

// feedNumberPattern matches the values Orgasoft reads as numbers: 0, or a
// number of up to 18 digits with no leading zero.
var feedNumberPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})$`)

// feedOrder is one entry of the feed. Field order is the wire order.
type feedOrder struct {
	OrderID         json.RawMessage `json:"orderId"`
	Status          string          `json:"status"`
	Total           json.RawMessage `json:"total"`
	PharmacyID      int64           `json:"pharmacyId"`
	PharmacyName    string          `json:"pharmacyName"`
	PharmacyCode    json.RawMessage `json:"pharmacyCode"`
	PharmacyNumber  string          `json:"pharmacyNumber"`
	PharmacyCity    string          `json:"pharmacyCity"`
	PharmacyAddress string          `json:"pharmacyAddress"`
	Items           []feedItem      `json:"items"`
}

// feedItem is one line of an entry. Price is the public (list) unit price
// when there is one; Discount is a percentage.
type feedItem struct {
	Code     json.RawMessage `json:"code"`
	Name     string          `json:"name"`
	Qty      int             `json:"qty"`
	Price    json.RawMessage `json:"price"`
	Discount json.RawMessage `json:"discount"`
}

// feedScalar writes an identifier as a JSON number when it is a plain
// number, as a JSON string otherwise, and null when it is empty.
func feedScalar(s string) json.RawMessage {
	if s == "" {
		return json.RawMessage("null")
	}
	if feedNumberPattern.MatchString(s) {
		return json.RawMessage(s)
	}
	raw, _ := json.Marshal(s) // a string always marshals
	return raw
}

// rawMoney writes a money value as its decimal text, never through a float.
func rawMoney(a money.Amount) json.RawMessage { return json.RawMessage(a.String()) }

// feedName is the Arabic name, else the English one.
func feedName(name map[string]string) string {
	if ar := name["ar"]; ar != "" {
		return ar
	}
	return name["en"]
}

// feedLine renders one order line. The price is the list price when there is
// one, else the unit price, and the discount is the percentage between them.
func feedLine(l erp.OrderLine) feedItem {
	price := l.UnitPrice
	if l.ListPrice.IsPositive() {
		price = l.ListPrice
	}
	return feedItem{
		Code:     feedScalar(l.ExternalProductCode),
		Name:     feedName(l.Name),
		Qty:      l.Quantity,
		Price:    rawMoney(price),
		Discount: rawMoney(erp.DiscountPercent(price, l.UnitPrice)),
	}
}

// feedSalesOrder renders one supplier shipment: the buyer's pharmacy and
// the shipment's total (subtotal plus shipping).
func feedSalesOrder(row erp.SalesRow) feedOrder {
	total, err := row.Subtotal.Add(row.ShippingFee)
	if err != nil {
		total = row.Total
	}
	code := json.RawMessage("null")
	if row.Customer.Mapped {
		code = feedScalar(row.Customer.ErpCode)
	}
	items := make([]feedItem, 0, len(row.Lines))
	for _, l := range row.Lines {
		items = append(items, feedLine(l))
	}
	return feedOrder{
		OrderID:         feedScalar(row.OrderNumber),
		Status:          strings.ToUpper(row.Status),
		Total:           rawMoney(total),
		PharmacyID:      row.BuyerOrgID,
		PharmacyName:    feedName(row.Customer.Name),
		PharmacyCode:    code,
		PharmacyNumber:  row.Customer.Ordering.Phone,
		PharmacyCity:    row.Customer.Ordering.City,
		PharmacyAddress: row.Customer.Ordering.Address,
		Items:           items,
	}
}

// feedPurchaseOrder renders one of the buyer's orders: the buyer's own
// organization and ordering branch, with the lines of every shipment.
func feedPurchaseOrder(row erp.PurchaseRow, buyerOrgID int64) feedOrder {
	items := []feedItem{}
	for _, sh := range row.Shipments {
		for _, l := range sh.Lines {
			items = append(items, feedLine(l))
		}
	}
	return feedOrder{
		OrderID:         feedScalar(row.OrderNumber),
		Status:          strings.ToUpper(row.Status),
		Total:           rawMoney(row.Total),
		PharmacyID:      buyerOrgID,
		PharmacyName:    feedName(row.BuyerName),
		PharmacyCode:    json.RawMessage("null"),
		PharmacyNumber:  row.Ordering.Phone,
		PharmacyCity:    row.Ordering.City,
		PharmacyAddress: row.Ordering.Address,
		Items:           items,
	}
}

// writeOrgasoftFeed writes the bare array. X-Next-Cursor is set only when a next
// page exists.
func writeOrgasoftFeed(w http.ResponseWriter, r *http.Request, rows []feedOrder, nextAfter int64) {
	requestID(w, r)
	w.Header().Set("Content-Type", "application/json")
	if nextAfter > 0 {
		w.Header().Set("X-Next-Cursor", EncodeCursor(nextAfter))
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(rows)
}

// listSalesOrgasoft is GET /<provider>/orders/sales: the same read as /orders/sales.
func (h *Handler) listSalesOrgasoft(w http.ResponseWriter, r *http.Request) {
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
	rows := make([]feedOrder, 0, len(page.Rows))
	for _, row := range page.Rows {
		rows = append(rows, feedSalesOrder(row))
	}
	writeOrgasoftFeed(w, r, rows, page.NextAfter)
}

// listPurchaseOrgasoft is GET /<provider>/orders/purchase: the same read as /orders/purchase.
func (h *Handler) listPurchaseOrgasoft(w http.ResponseWriter, r *http.Request) {
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
	rows := make([]feedOrder, 0, len(page.Rows))
	for _, row := range page.Rows {
		rows = append(rows, feedPurchaseOrder(row, principal.OrganizationID))
	}
	writeOrgasoftFeed(w, r, rows, page.NextAfter)
}
