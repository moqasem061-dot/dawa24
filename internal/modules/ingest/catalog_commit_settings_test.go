package ingest

// What the settings actually do.
//
// Every test here covers a setting that was, at some point, a switch on a
// screen that changed nothing: the publish toggle was overwritten with `true`
// in three separate places, the blank-quantity toggle could not be turned off
// because an absent checkbox was not read as "off", and the variant resolver
// could not tell one of a vendor's two variants of a product from the other, so
// a file's quantity landed on a brand-new third variant while the balance the
// pharmacy buys from never moved.
//
// They are cheap and they are here because all three are easy to undo by
// accident: a stray assignment, a helper that "normalises" a bool, an index
// that goes back to refusing an ambiguous product.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// commitFixture wires a service against the three mocks and returns them.
func commitFixture(session *Session, staged []*RowOutcome, keys []catalog.VariantKey,
	inWarehouse map[int64]bool,
) (*Service, *mockCommitCatalogPort, *mockCommitInventoryPort, *mockCommitImportStore) {
	cat := &mockCommitCatalogPort{keys: keys, nextVariantID: 9000}
	inv := &mockCommitInventoryPort{
		warehouses:  []*inventory.Warehouse{{ID: 1, OrganizationID: 10}},
		inWarehouse: inWarehouse,
	}
	store := &mockCommitImportStore{session: session, stagedRows: staged}

	svc := NewService(nil, slog.Default())
	svc.SetImportStore(store)
	svc.SetCatalogPort(cat)
	svc.SetInventoryPort(inv)
	return svc, cat, inv, store
}

// stagedRow is one reviewed row ready for commit.
func stagedRow(id int64, productID *int64, sku, name string, qty int, hasQty bool) *RowOutcome {
	return &RowOutcome{
		ID:         id,
		SourceRow:  int(id),
		ProductID:  productID,
		SourceCode: sku,
		Payload: &productmatch.Row{
			Number:      int(id),
			SKU:         sku,
			Name:        name,
			PublicPrice: money.FromMajor(100),
			Quantity:    qty,
			HasQuantity: hasQty,
		},
	}
}

func session(publicID string, settings Settings) *Session {
	return &Session{
		ID: 1, PublicID: publicID, OrganizationID: 10,
		Phase: PhaseReview, Settings: settings,
	}
}

// "Publish immediately" is on by default so imported variants go live right away.
func TestPublishImmediatelyIsOnByDefault(t *testing.T) {
	if !DefaultSettings().PublishImmediately {
		t.Error("publish immediately defaults to off, want on")
	}
	on := DefaultSettings()
	if !on.Normalize().PublishImmediately {
		t.Error("Normalize turned publish immediately off")
	}
	off := DefaultSettings()
	off.PublishImmediately = false
	if off.Normalize().PublishImmediately {
		t.Error("Normalize turned publish immediately back on")
	}
}

// A new variant is created inactive when the vendor left the switch off, and
// an updated one keeps whatever status it already had.
func TestPublishImmediatelyDecidesNewVariantStatus(t *testing.T) {
	ctx := context.Background()
	product := int64(101)
	existing := catalog.VariantKey{ID: 501, ProductID: 202, SKU: "SKU-OLD"}

	rows := []*RowOutcome{
		stagedRow(1, &product, "SKU-NEW", "New item", 5, true),
		stagedRow(2, ptr(int64(202)), "SKU-OLD", "Existing item", 7, true),
	}

	for _, tc := range []struct {
		name      string
		publish   bool
		wantFresh catalog.ProductStatus
	}{
		{"off creates inactive", false, catalog.StatusInactive},
		{"on creates active", true, catalog.StatusActive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.WarehouseID = 1
			settings.PublishImmediately = tc.publish
			svc, cat, _, _ := commitFixture(session("imp-"+tc.name, settings), rows,
				[]catalog.VariantKey{existing}, nil)

			if _, err := svc.CommitImport(ctx, "imp-"+tc.name); err != nil {
				t.Fatalf("CommitImport: %v", err)
			}

			var fresh, updated *catalog.ProductVariant
			for _, w := range cat.writtenVariants {
				if w.Variant.ID > 0 {
					updated = w.Variant
				} else {
					fresh = w.Variant
				}
			}
			if fresh == nil || updated == nil {
				t.Fatalf("expected one insert and one update, got %d writes", len(cat.writtenVariants))
			}
			if fresh.Status != tc.wantFresh {
				t.Errorf("new variant status = %q, want %q", fresh.Status, tc.wantFresh)
			}
			// The empty status is what tells the UPDATE statement to leave the
			// column alone. Sending 'inactive' here would delist a vendor's
			// whole live catalogue on a routine price refresh.
			if updated.Status != "" {
				t.Errorf("updated variant status = %q, want it left alone", updated.Status)
			}
		})
	}
}

// The warehouse decides which of a vendor's two variants of one product a row
// is about — which is the difference between updating a balance and creating a
// third variant nobody buys from.
func TestVariantResolutionPrefersTheVariantHoldingStockHere(t *testing.T) {
	product := int64(101)
	keys := []catalog.VariantKey{
		{ID: 501, ProductID: product, Unit: "علبة"},
		{ID: 502, ProductID: product, Unit: "شريط"},
	}
	row := &productmatch.Row{Name: "صنف", Number: 1}

	t.Run("one of them stocks it here", func(t *testing.T) {
		idx := newVariantIndex(keys, map[int64]bool{502: true}, nil)
		if got, _ := idx.resolve(row, product, nil); got != 502 {
			t.Errorf("resolved to %d, want the variant stocked in this warehouse (502)", got)
		}
	})

	t.Run("neither stocks it here: refuse rather than guess", func(t *testing.T) {
		idx := newVariantIndex(keys, nil, nil)
		if got, _ := idx.resolve(row, product, nil); got != 0 {
			t.Errorf("resolved to %d, want 0 — guessing here moves stock between live offers", got)
		}
	})

	t.Run("both stock it here: the branch breaks the tie", func(t *testing.T) {
		branchA, branchB := int64(7), int64(8)
		withBranches := []catalog.VariantKey{
			{ID: 501, ProductID: product, BranchID: &branchA},
			{ID: 502, ProductID: product, BranchID: &branchB},
		}
		idx := newVariantIndex(withBranches, map[int64]bool{501: true, 502: true}, &branchB)
		if got, _ := idx.resolve(row, product, nil); got != 502 {
			t.Errorf("resolved to %d, want the variant in the import's own branch (502)", got)
		}
	})

	t.Run("a variant this row already wrote wins outright", func(t *testing.T) {
		idx := newVariantIndex(keys, map[int64]bool{502: true}, nil)
		known := int64(501)
		if got, _ := idx.resolve(row, product, &known); got != 501 {
			t.Errorf("resolved to %d, want the variant the row is already linked to (501)", got)
		}
	})

	t.Run("a single variant needs no tie-break", func(t *testing.T) {
		idx := newVariantIndex(keys[:1], nil, nil)
		if got, _ := idx.resolve(row, product, nil); got != 501 {
			t.Errorf("resolved to %d, want 501", got)
		}
	})
}

// An existing variant's balance is written, not only a new one's. This is the
// "the stock never updated" report: the row resolved onto a variant the vendor
// already had, the update failed, and no balance was ever queued for it.
func TestUpdatedVariantsGetTheirBalanceWritten(t *testing.T) {
	ctx := context.Background()
	product := int64(202)
	settings := DefaultSettings()
	settings.WarehouseID = 1
	settings.StockMode = inventory.StockReplace

	rows := []*RowOutcome{stagedRow(1, &product, "SKU-OLD", "Existing item", 42, true)}
	keys := []catalog.VariantKey{{ID: 501, ProductID: product, SKU: "SKU-OLD"}}

	svc, _, inv, _ := commitFixture(session("imp-stock", settings), rows, keys, map[int64]bool{501: true})
	sess, err := svc.CommitImport(ctx, "imp-stock")
	if err != nil {
		t.Fatalf("CommitImport: %v", err)
	}
	if sess.UpdatedRows != 1 {
		t.Fatalf("updated = %d, want 1", sess.UpdatedRows)
	}
	if len(inv.writtenRows) != 1 {
		t.Fatalf("stock rows written = %d, want 1", len(inv.writtenRows))
	}
	written := inv.writtenRows[0]
	if written.Stock.ProductVariantID != 501 {
		t.Errorf("balance written against variant %d, want the existing 501",
			written.Stock.ProductVariantID)
	}
	if written.Stock.Quantity != 42 || !written.HasQuantity {
		t.Errorf("balance = %d (has=%v), want 42 from the file",
			written.Stock.Quantity, written.HasQuantity)
	}
	if inv.lastMode != inventory.StockReplace {
		t.Errorf("stock mode = %q, want the mode the vendor chose", inv.lastMode)
	}
}

// The blank-quantity rule reaches existing variants too. A vendor who says
// "this file is my whole stocktake" means the items it omits are at zero,
// including the ones they already had.
func TestBlankQuantityIsZeroAppliesToUpdatesToo(t *testing.T) {
	ctx := context.Background()
	product := int64(202)
	keys := []catalog.VariantKey{{ID: 501, ProductID: product, SKU: "SKU-OLD"}}
	rows := []*RowOutcome{stagedRow(1, &product, "SKU-OLD", "Existing item", 0, false)}

	t.Run("on: the blank cell is written as zero", func(t *testing.T) {
		settings := DefaultSettings()
		settings.WarehouseID = 1
		settings.BlankQuantityIsZero = true
		svc, _, inv, _ := commitFixture(session("imp-blank-on", settings), rows, keys, nil)
		if _, err := svc.CommitImport(ctx, "imp-blank-on"); err != nil {
			t.Fatalf("CommitImport: %v", err)
		}
		if len(inv.writtenRows) != 1 || !inv.writtenRows[0].HasQuantity {
			t.Fatalf("expected one balance stated as zero, got %#v", inv.writtenRows)
		}
	})

	t.Run("off: the balance is left alone", func(t *testing.T) {
		settings := DefaultSettings()
		settings.WarehouseID = 1
		settings.BlankQuantityIsZero = false
		svc, _, inv, _ := commitFixture(session("imp-blank-off", settings), rows, keys, nil)
		if _, err := svc.CommitImport(ctx, "imp-blank-off"); err != nil {
			t.Fatalf("CommitImport: %v", err)
		}
		if len(inv.writtenRows) != 1 || inv.writtenRows[0].HasQuantity {
			t.Fatalf("expected one balance marked 'file said nothing', got %#v", inv.writtenRows)
		}
	})
}

// Replace mode retires what the file does not mention — including while other
// rows are still awaiting a decision, which is the state every real price list
// arrives in.
//
// It used to switch itself off entirely whenever a single row was held, so the
// vendor chose "this file is my whole catalogue", the import wrote the matched
// rows, and their catalogue was left exactly as it was.
func TestReplaceModeRetiresWhatTheFileDoesNotMention(t *testing.T) {
	ctx := context.Background()
	written := int64(202)
	held := int64(303)
	settings := DefaultSettings()
	settings.WarehouseID = 1
	settings.Mode = ModeReplace

	rows := []*RowOutcome{stagedRow(1, &written, "SKU-WRITTEN", "Written item", 3, true)}
	keys := []catalog.VariantKey{
		{ID: 501, ProductID: written, SKU: "SKU-WRITTEN"},
		{ID: 502, ProductID: held, SKU: "SKU-HELD"},
		{ID: 503, ProductID: 404, SKU: "SKU-ABSENT"},
	}

	svc, cat, inv, store := commitFixture(session("imp-replace-held", settings), rows, keys, nil)
	// The file mentions two products: one whose row was confirmed, and one the
	// vendor left in the review queue. Only the third variant is absent.
	store.pendingRows = 1
	store.mentions = []RowMention{
		{ProductID: written, SourceCode: "SKU-WRITTEN"},
		{ProductID: held, SourceCode: "SKU-HELD"},
	}
	inv.inWarehouse = map[int64]bool{501: true, 502: true, 503: true}

	sess, err := svc.CommitImport(ctx, "imp-replace-held")
	if err != nil {
		t.Fatalf("CommitImport: %v", err)
	}

	kept := map[int64]bool{}
	for _, id := range cat.deactivatedExcept {
		kept[id] = true
	}
	if !kept[501] {
		t.Error("the variant this run wrote was not protected from retirement")
	}
	if kept[502] || kept[503] {
		t.Error("a variant not written in this commit must not be protected from retirement")
	}
	if len(cat.retired) != 2 {
		t.Errorf("retired = %+v, want 2 retired variants (502 and 503)", cat.retired)
	}
	if sess.ErrorMessage == "" {
		t.Error("the vendor was not told how many items were delisted")
	}

	// The delisted variant is retired through RetireVariantsExcept so that
	// its warehouse stock and product variant are removed, without writing a zero balance back.
	for _, row := range inv.writtenRows {
		if row.Stock != nil && row.Stock.ProductVariantID == 503 {
			t.Error("a retired variant should not have a zero stock written back to inventory")
		}
	}
}

// A commit that wrote nothing at all must not empty a catalogue.
func TestReplaceModeRefusesToRetireAfterWritingNothing(t *testing.T) {
	ctx := context.Background()
	settings := DefaultSettings()
	settings.WarehouseID = 1
	settings.Mode = ModeReplace

	keys := []catalog.VariantKey{{ID: 501, ProductID: 202, SKU: "SKU-OLD"}}
	svc, cat, _, _ := commitFixture(session("imp-replace-empty", settings), nil, keys, nil)

	sess, err := svc.CommitImport(ctx, "imp-replace-empty")
	if err != nil {
		t.Fatalf("CommitImport: %v", err)
	}
	if cat.deactivatedExcept != nil {
		t.Error("replace mode retired a catalogue on the strength of a run that wrote nothing")
	}
	if sess.ErrorMessage == "" {
		t.Error("the vendor was not told why nothing was delisted")
	}
}

func ptr[T any](v T) *T { return &v }
