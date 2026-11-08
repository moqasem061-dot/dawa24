package erp

import (
	"context"
	"errors"
	"slices"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
)

// SalesPage and PurchasePage are one keyset page. NextAfter is zero on the last
// page; Limit is the effective page size the caller gets back.
type SalesPage struct {
	Rows      []SalesRow
	NextAfter int64
	Limit     int
}

type PurchasePage struct {
	Rows      []PurchaseRow
	NextAfter int64
	Limit     int
}

// AckResult is the outcome for one order in an acknowledgement call.
type AckResult struct {
	OrderNumber, Result, Code, ExistingReference string
}

var orderStates = []string{"new", "imported", "changed", "all"}

// requireScope names the missing scope so the caller can fix the key.
func requireScope(p Principal, scope string) error {
	if !p.HasScope(scope) {
		return NewError("ERP_SCOPE_MISSING", 403, map[string]any{"requiredScope": scope})
	}
	return nil
}

// requireVendor refuses sales calls from a pharmacy organization.
func requireVendor(p Principal) error {
	if !vendorCapability(p.OrgType) {
		return NewError("ERP_CAPABILITY_REQUIRED", 403, nil)
	}
	return nil
}

// normaliseFilter applies the page defaults and refuses a bad state or size.
func normaliseFilter(f OrderFilter) (OrderFilter, error) {
	if f.State == "" {
		f.State = "new"
	}
	if !slices.Contains(orderStates, f.State) {
		return f, NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"state"}})
	}
	if f.Limit == 0 {
		f.Limit = PageDefault
	}
	if f.Limit < 1 || f.Limit > PageMax {
		return f, NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"limit"}, "limit": PageMax})
	}
	return f, nil
}

// lookahead asks the repository for one row more than the page, so the page
// knows whether a next cursor exists without a second query.
func lookahead(f OrderFilter) OrderFilter {
	f.Limit++
	return f
}

// ListSales returns the supplier's own shipments, oldest id first.
func (s *Service) ListSales(ctx context.Context, p Principal, f OrderFilter) (SalesPage, error) {
	if err := requireScope(p, ScopeOrdersRead); err != nil {
		return SalesPage{}, err
	}
	if err := requireVendor(p); err != nil {
		return SalesPage{}, err
	}
	f, err := normaliseFilter(f)
	if err != nil {
		return SalesPage{}, err
	}
	rows, err := s.repo.ListSalesShipmentsAsSystem(ctx, p.OrganizationID, p.IntegrationID, lookahead(f))
	if err != nil {
		return SalesPage{}, NewError("ERP_INTERNAL", 500, nil)
	}
	page := SalesPage{Rows: rows, Limit: f.Limit}
	if len(rows) > f.Limit {
		page.Rows = rows[:f.Limit]
		page.NextAfter = page.Rows[f.Limit-1].ShipmentID
	}
	return page, nil
}

// GetSales returns one sales order of the key's organization. A missing order
// and another organization's order give the same answer.
func (s *Service) GetSales(ctx context.Context, p Principal, orderNumber string) (SalesRow, error) {
	if err := requireScope(p, ScopeOrdersRead); err != nil {
		return SalesRow{}, err
	}
	if err := requireVendor(p); err != nil {
		return SalesRow{}, err
	}
	row, err := s.repo.GetSalesShipmentAsSystem(ctx, p.OrganizationID, orderNumber)
	if errors.Is(err, ErrNotFound) {
		return SalesRow{}, NewError("ERP_ORDER_NOT_FOUND", 404, nil)
	}
	if err != nil {
		return SalesRow{}, NewError("ERP_INTERNAL", 500, nil)
	}
	return row, nil
}

// ListPurchase returns the buyer's own orders, each with its supplier shipments.
func (s *Service) ListPurchase(ctx context.Context, p Principal, f OrderFilter) (PurchasePage, error) {
	if err := requireScope(p, ScopeOrdersRead); err != nil {
		return PurchasePage{}, err
	}
	f, err := normaliseFilter(f)
	if err != nil {
		return PurchasePage{}, err
	}
	rows, err := s.repo.ListPurchaseOrders(ctx, p.OrganizationID, p.IntegrationID, lookahead(f))
	if err != nil {
		return PurchasePage{}, NewError("ERP_INTERNAL", 500, nil)
	}
	page := PurchasePage{Rows: rows, Limit: f.Limit}
	if len(rows) > f.Limit {
		page.Rows = rows[:f.Limit]
		page.NextAfter = page.Rows[f.Limit-1].OrderID
	}
	if err := s.attachPurchaseShipments(ctx, p.OrganizationID, page.Rows); err != nil {
		return PurchasePage{}, err
	}
	return page, nil
}

// GetPurchase returns one buyer order with its shipments.
func (s *Service) GetPurchase(ctx context.Context, p Principal, orderNumber string) (PurchaseRow, error) {
	if err := requireScope(p, ScopeOrdersRead); err != nil {
		return PurchaseRow{}, err
	}
	row, err := s.repo.GetPurchaseOrder(ctx, p.OrganizationID, orderNumber)
	if errors.Is(err, ErrNotFound) {
		return PurchaseRow{}, NewError("ERP_ORDER_NOT_FOUND", 404, nil)
	}
	if err != nil {
		return PurchaseRow{}, NewError("ERP_INTERNAL", 500, nil)
	}
	rows := []PurchaseRow{row}
	if err := s.attachPurchaseShipments(ctx, p.OrganizationID, rows); err != nil {
		return PurchaseRow{}, err
	}
	return rows[0], nil
}

// attachPurchaseShipments loads the shipments of the page's orders in one call
// and groups them under their order.
func (s *Service) attachPurchaseShipments(ctx context.Context, buyerOrgID int64, rows []PurchaseRow) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.OrderID)
	}
	shipments, err := s.repo.ListPurchaseShipmentsAsSystem(ctx, buyerOrgID, ids)
	if err != nil {
		return NewError("ERP_INTERNAL", 500, nil)
	}
	byOrder := map[int64][]PurchaseShipment{}
	for _, sh := range shipments {
		byOrder[sh.OrderID] = append(byOrder[sh.OrderID], sh)
	}
	for i := range rows {
		rows[i].Shipments = byOrder[rows[i].OrderID]
	}
	return nil
}

// Ack records that the ERP imported orders. direction is "sales" (the supplier
// acknowledges its own shipments) or "purchase" (the buyer acknowledges its
// orders). Each order gets its own result, so one bad order never hides the rest.
func (s *Service) Ack(ctx context.Context, p Principal, direction string, entries []provider.AckEntry) ([]AckResult, error) {
	if err := requireScope(p, ScopeOrdersAck); err != nil {
		return nil, err
	}
	if direction == "sales" {
		if err := requireVendor(p); err != nil {
			return nil, err
		}
	}
	if len(entries) == 0 || len(entries) > AckMax {
		return nil, NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"orders"}, "limit": AckMax})
	}
	for _, e := range entries {
		if e.OrderNumber == "" || len(e.ErpReference) < 1 || len(e.ErpReference) > 100 {
			return nil, NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"erpReference"}})
		}
	}
	out := make([]AckResult, 0, len(entries))
	for _, e := range entries {
		res, err := s.ackOne(ctx, p, direction, e)
		if err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, nil
}

func (s *Service) ackOne(ctx context.Context, p Principal, direction string, e provider.AckEntry) (AckResult, error) {
	imp := OrderImport{OrganizationID: p.OrganizationID, IntegrationID: p.IntegrationID, APIKeyID: p.KeyID, Direction: direction, OrderNumber: e.OrderNumber, ErpReference: e.ErpReference}
	switch direction {
	case "sales":
		row, err := s.repo.GetSalesShipmentAsSystem(ctx, p.OrganizationID, e.OrderNumber)
		if errors.Is(err, ErrNotFound) {
			return AckResult{OrderNumber: e.OrderNumber, Result: "not_found"}, nil
		}
		if err != nil {
			return AckResult{}, NewError("ERP_INTERNAL", 500, nil)
		}
		shipment := row.ShipmentID
		imp.OrderID, imp.ShipmentID, imp.AckedStatus = row.OrderID, &shipment, row.Status
	default:
		row, err := s.repo.GetPurchaseOrder(ctx, p.OrganizationID, e.OrderNumber)
		if errors.Is(err, ErrNotFound) {
			return AckResult{OrderNumber: e.OrderNumber, Result: "not_found"}, nil
		}
		if err != nil {
			return AckResult{}, NewError("ERP_INTERNAL", 500, nil)
		}
		imp.OrderID, imp.AckedStatus = row.OrderID, row.Status
	}
	keyID := p.KeyID
	audit := AuditRecord{Action: "erp.order.ack", EntityType: "order_import", EntityID: e.OrderNumber, ActorKeyID: &keyID, ActorKind: "api_key"}
	outcome, err := s.repo.UpsertOrderImport(ctx, imp, audit)
	if err != nil {
		return AckResult{}, NewError("ERP_INTERNAL", 500, nil)
	}
	return AckResult{OrderNumber: e.OrderNumber, Result: outcome.Result, Code: outcome.Code, ExistingReference: outcome.ExistingReference}, nil
}
