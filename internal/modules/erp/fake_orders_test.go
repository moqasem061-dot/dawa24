package erp

import (
	"context"
	"fmt"
	"time"
)

// fakeOrders is the in-memory OrderRepository the service tests use. It keeps
// the same rules as the SQL: a sales row is visible only to its supplier, a
// purchase row only to its buyer, and the import state follows the stored
// acknowledgement.
type fakeOrders struct {
	sales           []fakeSale
	purchase        []fakePurchase
	imports         map[string]fakeImport
	lastSalesFilter OrderFilter
	nextID          int64
}

type fakeSale struct {
	org     int64
	created time.Time
	row     SalesRow
}

type fakePurchase struct {
	org       int64
	created   time.Time
	row       PurchaseRow
	shipments []PurchaseShipment
}

type fakeImport struct {
	ref, status string
	count       int
}

var _ OrderRepository = (*fakeOrders)(nil)

func newFakeOrders() *fakeOrders {
	return &fakeOrders{imports: map[string]fakeImport{}}
}

func (f *fakeOrders) nextIdentifier() int64 {
	f.nextID++
	return f.nextID
}

// addSale seeds one supplier shipment of an order owned by the buyer side.
func (f *fakeOrders) addSale(org int64, orderNumber, status string, created time.Time) {
	id := f.nextIdentifier()
	f.sales = append(f.sales, fakeSale{org: org, created: created, row: SalesRow{
		OrderID: id + 100000, ShipmentID: id, OrderNumber: orderNumber, ShipmentNumber: orderNumber + "-1",
		Status: status, CreatedAt: created, UpdatedAt: created,
	}})
}

// addPurchase seeds one buyer order.
func (f *fakeOrders) addPurchase(buyer int64, orderNumber, status string, created time.Time) {
	id := f.nextIdentifier()
	f.purchase = append(f.purchase, fakePurchase{org: buyer, created: created, row: PurchaseRow{
		OrderID: id, OrderNumber: orderNumber, Status: status, CreatedAt: created, UpdatedAt: created,
	}})
}

// importReference returns the stored ERP reference of a sales order, or "".
func (f *fakeOrders) importReference(orderNumber string) string {
	for _, s := range f.sales {
		if s.row.OrderNumber == orderNumber {
			return f.imports[salesKey(s.row.ShipmentID)].ref
		}
	}
	return ""
}

func salesKey(shipmentID int64) string { return fmt.Sprintf("sales|%d", shipmentID) }

func purchaseKey(orderID int64) string { return fmt.Sprintf("purchase|%d", orderID) }

// stateFor mirrors the SQL CASE: no import is new, the same acknowledged
// status is imported, anything else is changed.
func (f *fakeOrders) stateFor(key, status string) (string, fakeImport, bool) {
	imp, ok := f.imports[key]
	switch {
	case !ok:
		return "new", imp, false
	case imp.status == status:
		return "imported", imp, true
	default:
		return "changed", imp, true
	}
}

func inOrderRange(created time.Time, flt OrderFilter) bool {
	if flt.From != nil && created.Before(*flt.From) {
		return false
	}
	return flt.To == nil || created.Before(*flt.To)
}

func matchesState(want, got string) bool {
	return want == "" || want == "all" || want == got
}

func (f *fakeOrders) ListSalesShipmentsAsSystem(_ context.Context, orgID, _ int64, flt OrderFilter) ([]SalesRow, error) {
	f.lastSalesFilter = flt
	var out []SalesRow
	for _, s := range f.sales {
		if s.org != orgID || s.row.ShipmentID <= flt.After || !inOrderRange(s.created, flt) {
			continue
		}
		row := s.row
		state, imp, ok := f.stateFor(salesKey(row.ShipmentID), row.Status)
		if !matchesState(flt.State, state) {
			continue
		}
		row.ImportState = state
		if ok {
			row.ErpReference = imp.ref
		}
		out = append(out, row)
		if flt.Limit > 0 && len(out) >= flt.Limit {
			break
		}
	}
	return out, nil
}

func (f *fakeOrders) GetSalesShipmentAsSystem(_ context.Context, orgID int64, orderNumber string) (SalesRow, error) {
	for _, s := range f.sales {
		if s.org == orgID && s.row.OrderNumber == orderNumber {
			row := s.row
			row.ImportState, _, _ = f.stateFor(salesKey(row.ShipmentID), row.Status)
			return row, nil
		}
	}
	return SalesRow{}, ErrNotFound
}

func (f *fakeOrders) ListPurchaseOrders(_ context.Context, buyer, _ int64, flt OrderFilter) ([]PurchaseRow, error) {
	var out []PurchaseRow
	for _, p := range f.purchase {
		if p.org != buyer || p.row.OrderID <= flt.After || !inOrderRange(p.created, flt) {
			continue
		}
		row := p.row
		state, imp, ok := f.stateFor(purchaseKey(row.OrderID), row.Status)
		if !matchesState(flt.State, state) {
			continue
		}
		row.ImportState = state
		if ok {
			row.ErpReference = imp.ref
		}
		out = append(out, row)
		if flt.Limit > 0 && len(out) >= flt.Limit {
			break
		}
	}
	return out, nil
}

func (f *fakeOrders) GetPurchaseOrder(_ context.Context, buyer int64, orderNumber string) (PurchaseRow, error) {
	for _, p := range f.purchase {
		if p.org == buyer && p.row.OrderNumber == orderNumber {
			row := p.row
			row.ImportState, _, _ = f.stateFor(purchaseKey(row.OrderID), row.Status)
			return row, nil
		}
	}
	return PurchaseRow{}, ErrNotFound
}

func (f *fakeOrders) ListPurchaseShipmentsAsSystem(_ context.Context, buyer int64, orderIDs []int64) ([]PurchaseShipment, error) {
	var out []PurchaseShipment
	for _, p := range f.purchase {
		if p.org != buyer {
			continue
		}
		for _, id := range orderIDs {
			if id == p.row.OrderID {
				out = append(out, p.shipments...)
			}
		}
	}
	return out, nil
}

func (f *fakeOrders) UpsertOrderImport(_ context.Context, imp OrderImport, _ AuditRecord) (AckOutcome, error) {
	key := purchaseKey(imp.OrderID)
	if imp.Direction == "sales" && imp.ShipmentID != nil {
		key = salesKey(*imp.ShipmentID)
	}
	cur, ok := f.imports[key]
	if !ok {
		f.imports[key] = fakeImport{ref: imp.ErpReference, status: imp.AckedStatus, count: 1}
		return AckOutcome{Result: "acknowledged"}, nil
	}
	if cur.ref != imp.ErpReference {
		return AckOutcome{Result: "conflict", Code: "ERP_ACK_REFERENCE_CONFLICT", ExistingReference: cur.ref}, nil
	}
	cur.status = imp.AckedStatus
	cur.count++
	f.imports[key] = cur
	return AckOutcome{Result: "already_acknowledged"}, nil
}
