package http

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// The order shape Orgasoft asked for (2026-11-10), used by every order read and
// by purchase creation: a list is a bare JSON array with the next page in the
// X-Next-Cursor header; one order is a bare object. Field order and number
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

// feedTotal is Orgasoft's total: what the items cost after discount (the sum of
// the line totals) plus shipping. The stored subtotal is the price before
// discount and the order header total is too, so neither is used.
func feedTotal(lines []erp.OrderLine, shipping money.Amount) money.Amount {
	total := shipping
	for _, l := range lines {
		if sum, err := total.Add(l.TotalPrice); err == nil {
			total = sum
		}
	}
	return total
}

// feedSalesOrder renders one supplier shipment for the buyer's pharmacy.
func feedSalesOrder(row erp.SalesRow) feedOrder {
	total := feedTotal(row.Lines, row.ShippingFee)
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
	total := money.Zero
	for _, sh := range row.Shipments {
		for _, l := range sh.Lines {
			items = append(items, feedLine(l))
		}
		if sum, err := total.Add(feedTotal(sh.Lines, sh.ShippingFee)); err == nil {
			total = sum
		}
	}
	return feedOrder{
		OrderID:         feedScalar(row.OrderNumber),
		Status:          strings.ToUpper(row.Status),
		Total:           rawMoney(total),
		PharmacyID:      buyerOrgID,
		PharmacyName:    feedName(row.BuyerName),
		PharmacyCode:    json.RawMessage("null"),
		PharmacyNumber:  row.Ordering.Phone,
		PharmacyCity:    row.Ordering.City,
		PharmacyAddress: row.Ordering.Address,
		Items:           items,
	}
}

// writeOrderFeed writes a list of orders as a bare array. X-Next-Cursor is set
// only when a next page exists.
func writeOrderFeed(w http.ResponseWriter, r *http.Request, rows []feedOrder, nextAfter int64) {
	requestID(w, r)
	w.Header().Set("Content-Type", "application/json")
	if nextAfter > 0 {
		w.Header().Set("X-Next-Cursor", EncodeCursor(nextAfter))
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(rows)
}

// writeOrder writes one order as a bare object.
func writeOrder(w http.ResponseWriter, r *http.Request, status int, order feedOrder) {
	requestID(w, r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(order)
}
