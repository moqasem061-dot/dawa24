package erp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// maxPurchaseQuantity is the contract's upper bound for one line's quantity.
const maxPurchaseQuantity = 100000

// maxExternalReference is the contract's length limit for externalReference.
const maxExternalReference = 100

// purchaseAuditAction names the audit row written when a purchase order is
// created (plan.md S7 step 4).
const purchaseAuditAction = "erp.po.create"

// Offers lists the supplier products with the barcode, cheapest first. At most
// OffersMax come back. The read is named exception E6.
func (s *Service) Offers(ctx context.Context, p Principal, barcode string) ([]Offer, error) {
	if err := requireScope(p, ScopePurchaseCreate); err != nil {
		return nil, err
	}
	barcode = strings.TrimSpace(barcode)
	if barcode == "" || len(barcode) > 32 {
		return nil, NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"barcode"}})
	}
	offers, err := s.repo.ListOffersAsSystem(ctx, p.OrganizationID, barcode, OffersMax)
	if err != nil {
		return nil, NewError("ERP_INTERNAL", 500, nil)
	}
	for i := range offers {
		offers[i].UnitPrice = offerUnitPrice(offers[i].PublicPrice, offers[i].DiscountPercent)
	}
	sort.SliceStable(offers, func(i, j int) bool { return offers[i].UnitPrice.Minor() < offers[j].UnitPrice.Minor() })
	if len(offers) > OffersMax {
		offers = offers[:OffersMax]
	}
	return offers, nil
}

// CreatePurchaseOrder places one order for the buyer organization of the key.
// A retry with the same externalReference returns the stored order (Replay) and
// never a second one. A refusal deletes the idempotency row so the ERP can retry.
// A crash leaves the row in progress; after the stale window the order is found
// through its note marker.
func (s *Service) CreatePurchaseOrder(ctx context.Context, p Principal, req CreatePurchaseRequest) (PurchaseResult, error) {
	if err := requireScope(p, ScopePurchaseCreate); err != nil {
		return PurchaseResult{}, err
	}
	ref := req.ExternalReference
	if strings.TrimSpace(ref) == "" {
		return PurchaseResult{}, NewError("ERP_PO_REFERENCE_REQUIRED", 422, nil)
	}
	if len(ref) > maxExternalReference {
		return PurchaseResult{}, NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"externalReference"}})
	}
	if len(req.Lines) == 0 || len(req.Lines) > PurchaseLinesMax {
		return PurchaseResult{}, NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"lines"}})
	}
	if p.BuyingUserID == nil {
		return PurchaseResult{}, NewError("ERP_PO_BUYING_USER_MISSING", 409, nil)
	}
	active, err := s.repo.ActiveMember(ctx, p.OrganizationID, *p.BuyingUserID)
	if err != nil {
		return PurchaseResult{}, NewError("ERP_INTERNAL", 500, nil)
	}
	if !active {
		return PurchaseResult{}, NewError("ERP_PO_BUYING_USER_MISSING", 409, nil)
	}
	if s.ports.Placer == nil {
		return PurchaseResult{}, NewError("ERP_UNAVAILABLE", 503, nil)
	}
	if _, err := s.checkPurchaseLines(ctx, p, req.Lines); err != nil {
		return PurchaseResult{}, err
	}

	sha := purchaseRequestDigest(req)
	state, err := s.repo.BeginIdempotency(ctx, p.OrganizationID, p.IntegrationID, ref, sha)
	if err != nil {
		return PurchaseResult{}, NewError("ERP_INTERNAL", 500, nil)
	}
	if !state.Created {
		if state.RequestSHA != sha {
			return PurchaseResult{}, NewError("ERP_IDEMPOTENCY_CONFLICT", 409, nil)
		}
		if state.Replay {
			return replayPurchase(state.Response)
		}
		if !state.Stale {
			return PurchaseResult{}, NewError("ERP_PO_IN_PROGRESS", 409, nil)
		}
		repaired, found, err := s.repairPurchase(ctx, p, ref)
		if err != nil {
			return PurchaseResult{}, err
		}
		if found {
			return repaired, nil
		}
		// The crashed attempt wrote no order, so the stale row is dropped and the
		// order is placed again under a fresh row.
		if err := s.repo.DeleteIdempotency(ctx, p.OrganizationID, p.IntegrationID, ref); err != nil {
			return PurchaseResult{}, NewError("ERP_INTERNAL", 500, nil)
		}
		state, err = s.repo.BeginIdempotency(ctx, p.OrganizationID, p.IntegrationID, ref, sha)
		if err != nil || !state.Created {
			return PurchaseResult{}, NewError("ERP_PO_IN_PROGRESS", 409, nil)
		}
	}

	placed, err := s.ports.Placer.Place(ctx, PurchaseRequest{
		BuyerOrgID: p.OrganizationID, BuyingUserID: *p.BuyingUserID, BranchID: p.BranchID, BuyerOrgType: p.OrgType,
		Lines: req.Lines, Notes: purchaseNotes(p.Provider, ref, req.Notes),
	})
	if err != nil {
		var e *Error
		if !errors.As(err, &e) {
			return PurchaseResult{}, NewError("ERP_INTERNAL", 500, nil)
		}
		if e.Status < 500 {
			// A refusal is final for this attempt: free the reference for a retry.
			_ = s.repo.DeleteIdempotency(ctx, p.OrganizationID, p.IntegrationID, ref)
		}
		return PurchaseResult{}, e
	}
	row, err := s.purchaseRowFor(ctx, p, ref, placed.OrderNumber)
	if err != nil {
		return PurchaseResult{}, err
	}
	body, err := json.Marshal(row)
	if err != nil {
		return PurchaseResult{}, NewError("ERP_INTERNAL", 500, nil)
	}
	keyID := p.KeyID
	audit := AuditRecord{Action: purchaseAuditAction, EntityType: "order", EntityID: placed.OrderNumber,
		After: map[string]any{"orderNumber": placed.OrderNumber, "externalReference": ref}, ActorKeyID: &keyID, ActorKind: "api_key"}
	if err := s.repo.FinishIdempotency(ctx, p.OrganizationID, p.IntegrationID, ref, placed.OrderID, placed.OrderNumber, body, audit); err != nil {
		return PurchaseResult{}, NewError("ERP_INTERNAL", 500, nil)
	}
	return PurchaseResult{OrderID: placed.OrderID, OrderNumber: placed.OrderNumber, Row: row}, nil
}

// checkPurchaseLines validates each line against the supplier offers, in line
// order: quantity, item, own organization, active offer, supplier link, and
// finally the sent prices together. Nothing is written here.
func (s *Service) checkPurchaseLines(ctx context.Context, p Principal, lines []PurchaseLine) ([]VariantOfferRow, error) {
	rows := make([]VariantOfferRow, 0, len(lines))
	var mismatches []map[string]any
	for i, line := range lines {
		n := i + 1
		if line.Quantity < 1 || line.Quantity > maxPurchaseQuantity {
			return nil, NewError("ERP_PO_QUANTITY_INVALID", 422, map[string]any{"line": n})
		}
		row, err := s.repo.VariantOfferAsSystem(ctx, line.VariantID)
		if errors.Is(err, ErrNotFound) {
			return nil, NewError("ERP_PO_ITEM_NOT_FOUND", 422, map[string]any{"line": n, "variantId": line.VariantID})
		}
		if err != nil {
			return nil, NewError("ERP_INTERNAL", 500, nil)
		}
		if row.OrganizationID == p.OrganizationID {
			return nil, NewError("ERP_PO_SELF_SUPPLY", 422, nil)
		}
		if !row.Active || !row.InStock {
			return nil, NewError("ERP_PO_NO_ACTIVE_OFFER", 422, map[string]any{"line": n, "variantId": line.VariantID})
		}
		linked, err := s.repo.SupplierLinked(ctx, p.OrganizationID, row.OrganizationID)
		if err != nil {
			return nil, NewError("ERP_INTERNAL", 500, nil)
		}
		if !linked {
			return nil, NewError("ERP_PO_SUPPLIER_NOT_LINKED", 422, map[string]any{"line": n, "supplierName": displayName(row.SupplierName)})
		}
		correct := offerUnitPrice(row.PublicPrice, row.DiscountPercent)
		if line.UnitPrice != nil && line.UnitPrice.Minor() != correct.Minor() {
			mismatches = append(mismatches, map[string]any{"line": n, "sentPrice": line.UnitPrice.String(), "correctPrice": correct.String()})
		}
		rows = append(rows, row)
	}
	if len(mismatches) > 0 {
		return nil, NewError("ERP_PO_PRICE_MISMATCH", 409, map[string]any{"lines": mismatches})
	}
	return rows, nil
}

// repairPurchase answers a stale reference from the order that carries its
// marker, if a crashed attempt wrote one. It finishes the row so later retries
// replay the same order.
func (s *Service) repairPurchase(ctx context.Context, p Principal, ref string) (PurchaseResult, bool, error) {
	orderID, number, found, err := s.repo.FindOrderByNoteMarker(ctx, p.OrganizationID, purchaseMarker(p.Provider, ref))
	if err != nil {
		return PurchaseResult{}, false, NewError("ERP_INTERNAL", 500, nil)
	}
	if !found {
		return PurchaseResult{}, false, nil
	}
	row, err := s.purchaseRowFor(ctx, p, ref, number)
	if err != nil {
		return PurchaseResult{}, false, err
	}
	body, err := json.Marshal(row)
	if err != nil {
		return PurchaseResult{}, false, NewError("ERP_INTERNAL", 500, nil)
	}
	if err := s.repo.FinishIdempotency(ctx, p.OrganizationID, p.IntegrationID, ref, orderID, number, body, AuditRecord{}); err != nil {
		return PurchaseResult{}, false, NewError("ERP_INTERNAL", 500, nil)
	}
	return PurchaseResult{OrderID: orderID, OrderNumber: number, Replay: true, Row: row}, true, nil
}

// purchaseRowFor reads the order the buyer just received, with its shipments,
// and sets the external reference the ERP sent.
func (s *Service) purchaseRowFor(ctx context.Context, p Principal, ref, number string) (PurchaseRow, error) {
	row, err := s.repo.GetPurchaseOrder(ctx, p.OrganizationID, number)
	if err != nil {
		return PurchaseRow{}, NewError("ERP_INTERNAL", 500, nil)
	}
	rows := []PurchaseRow{row}
	if err := s.attachPurchaseShipments(ctx, p.OrganizationID, rows); err != nil {
		return PurchaseRow{}, err
	}
	rows[0].ExternalReference = ref
	return rows[0], nil
}

// replayPurchase returns the stored order of an earlier call with the same reference.
func replayPurchase(stored []byte) (PurchaseResult, error) {
	var row PurchaseRow
	if err := json.Unmarshal(stored, &row); err != nil {
		return PurchaseResult{}, NewError("ERP_INTERNAL", 500, nil)
	}
	return PurchaseResult{OrderID: row.OrderID, OrderNumber: row.OrderNumber, Replay: true, Row: row}, nil
}

// offerUnitPrice is the website's effective price: the public price less the
// discount percentage (basis points in minor units, as the catalog stores it).
func offerUnitPrice(public, discountPercent money.Amount) money.Amount {
	d := discountPercent.Minor()
	switch {
	case d <= 0:
		return public
	case d >= 10000:
		return money.Zero
	default:
		return public.ApplyPercent(10000 - d)
	}
}

// purchaseMarker is the note prefix that ties an order to its ERP reference.
func purchaseMarker(provider, ref string) string {
	return "[ERP:" + provider + ":" + ref + "]"
}

// purchaseNotes puts the marker first, so a stale repair can find the order.
func purchaseNotes(provider, ref, notes string) string {
	if notes == "" {
		return purchaseMarker(provider, ref)
	}
	return purchaseMarker(provider, ref) + "\n" + notes
}

// purchaseRequestDigest fingerprints the request body. The same reference with a
// different body is a conflict, not a replay.
func purchaseRequestDigest(req CreatePurchaseRequest) string {
	raw, _ := json.Marshal(req)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// displayName is the supplier's English name, or its Arabic name when there is none.
func displayName(name map[string]string) string {
	if en := name["en"]; en != "" {
		return en
	}
	return name["ar"]
}
