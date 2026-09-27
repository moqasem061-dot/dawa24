package postgres

import (
	"os"
	"strings"
	"testing"
)

// Editing a pending order.
//
// The two defects below are properties of one SQL file, so they are checked
// there rather than round-tripped through a database this suite does not have.

func orderEditSource(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("order_edit.go")
	if err != nil {
		t.Fatalf("read order_edit.go: %v", err)
	}
	return string(body)
}

// Every shipment is recalculated from its own lines.
//
// The old code wrote the whole order's subtotal and total onto the first
// shipment and left the others untouched, so a pharmacy buying from three
// suppliers gave parcel 1 the entire order's value while parcels 2 and 3 kept
// pre-edit figures — and a shipment is exactly what each supplier reads.
func TestEveryShipmentIsRecalculatedFromItsOwnLines(t *testing.T) {
	src := orderEditSource(t)

	if strings.Contains(src, "WHERE id = $3;") &&
		strings.Contains(src, "UPDATE commerce.order_shipments\n\t\t\t\tSET subtotal = $1") {
		t.Fatal("shipment totals are still written to a single shipment id")
	}
	if !strings.Contains(src, "LEFT JOIN commerce.order_lines l ON l.shipment_id = sh.id") {
		t.Error("shipment totals are not derived from each shipment's own lines")
	}
	if !strings.Contains(src, "GROUP BY sh.id") {
		t.Error("the recalculation does not group per shipment")
	}
	// A vendor's delivery quote is not affected by a quantity change, so it
	// survives the recalculation rather than being recomputed from lines.
	if !strings.Contains(src, "COALESCE(s.shipping_fee, 0)") {
		t.Error("the shipment's own shipping fee is dropped from its total")
	}
}

// A pharmacy may change quantities and remove lines on its own pending order.
// It may not invent one.
//
// The removed path inserted a free-text row at a price the buyer typed, with
// sku 'CUSTOM', no product and no variant — so no stock check could run — and
// attached it to whichever shipment happened to be first, asking a supplier to
// ship something they had never listed.
func TestOrderEditRefusesManuallyAddedLines(t *testing.T) {
	src := orderEditSource(t)

	if strings.Contains(src, "INSERT INTO commerce.order_lines") {
		t.Error("order editing can still insert a line the supplier never listed")
	}
	if strings.Contains(src, `"CUSTOM"`) {
		t.Error("the CUSTOM sku path survives")
	}
	if !strings.Contains(src, `apperr.Validation("order.line_not_editable"`) {
		t.Error("a line with no id is not refused with a reason")
	}
}

// A supplier edit is confined to its own pending shipment. Every line id is
// checked against that shipment before anything is written, the shipment row
// is locked so an accept cannot race the edit, and the shipment cannot be
// emptied — a supplier that can supply nothing rejects the order instead.
func TestVendorOrderEditIsScopedToItsOwnPendingShipment(t *testing.T) {
	src := orderEditSource(t)

	for _, want := range []string{
		"WHERE order_id = $1 AND organization_id = $2\n\t\t\t\tORDER BY id ASC LIMIT 1 FOR UPDATE;",
		"commerce.OrderStatus(shipmentStatus) != commerce.StatusPending",
		`apperr.Forbidden("order.line_not_owned"`,
		`apperr.Validation("order.shipment_empty"`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("vendor edit guard missing: %q", want)
		}
	}
}

func TestShipmentAuditRunsAfterOrganizationRowsClose(t *testing.T) {
	src := orderEditSource(t)
	queryAt := strings.Index(src, "vRows, err := tx.Query(")
	if queryAt < 0 {
		t.Fatal("shipment audit organizations are not loaded")
	}
	auditAt := strings.Index(src[queryAt:], "database.WriteAudit")
	if auditAt < 0 {
		t.Fatal("vendor shipment audits are not written")
	}
	auditAt += queryAt
	closeAt := strings.LastIndex(src[queryAt:auditAt], "vRows.Close()") + queryAt
	if closeAt < queryAt || closeAt >= auditAt {
		t.Fatal("shipment organization rows must close before a write uses the same transaction connection")
	}
	if !strings.Contains(src[queryAt:auditAt], "vRows.Err()") {
		t.Fatal("shipment organization query iteration errors are not checked")
	}
}

func TestEditingStandaloneOfferCannotChangeItsUnitQuantity(t *testing.T) {
	body, err := os.ReadFile("order_edit_calc.go")
	if err != nil {
		t.Fatalf("read order_edit_calc.go: %v", err)
	}
	source := string(body)
	if !strings.Contains(source, "order.OfferID > 0 && dbProductID == nil && dbVariantID == nil && l.Quantity != dbOldQty") {
		t.Fatal("editing a standalone offer line can still change its quantity")
	}
}

func TestOrderEditLoadsOfferQuantityBounds(t *testing.T) {
	body, err := os.ReadFile("orders_query_repo.go")
	if err != nil {
		t.Fatalf("read orders_query_repo.go: %v", err)
	}
	source := string(body)
	for _, want := range []string{
		"COALESCE(NULLIF(op.custom_qty, 0), 1)",
		"WHEN o.offer_id IS NOT NULL AND l.product_id IS NULL AND l.product_variant_id IS NULL THEN 1",
		"COALESCE(op.max_qty_per_order, 0)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("order edit metadata missing %q", want)
		}
	}
}

func TestVendorOrderEditLoadsOfferQuantityBounds(t *testing.T) {
	body, err := os.ReadFile("shipment_detail.go")
	if err != nil {
		t.Fatalf("read shipment_detail.go: %v", err)
	}
	source := string(body)
	if !strings.Contains(source, "orderLineQuantityBoundsProjection") || !strings.Contains(source, "l.offer_product_id") {
		t.Fatal("vendor shipment lines do not load offer identifiers and quantity bounds")
	}
}
