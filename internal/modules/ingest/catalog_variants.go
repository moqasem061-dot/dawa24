package ingest

import (
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Which of the vendor's own variants is this row about?
//
// This is a different question from "which catalogue product is this", and it
// has to be answered second, because the answer usually depends on the first.
// A vendor may stock the same catalogue product in three pack sizes and two
// batches; those are three variants, and updating the wrong one silently moves
// stock between them.
//
// The whole of a vendor's catalogue is loaded once and indexed here, so the
// question costs a map lookup per row rather than the three queries the
// previous importer issued.
//
// The index is built for ONE warehouse — the one the import is writing to — and
// that is what makes the last resort safe. A vendor with two variants of the
// same product used to be treated as unanswerable, and the row was inserted as
// a third variant: the file's quantity landed on a variant nobody buys from,
// and the balance the pharmacy sees never moved. The variant that already holds
// a balance in this warehouse is the one the file is about, and asking that
// question is one set lookup.

// variantIndex resolves a parsed row onto an existing variant.
type variantIndex struct {
	bySKU     map[string]int64
	byBarcode map[string]int64
	productOf map[int64]int64
	// byProductPack keys on the catalogue product plus the packaging that
	// distinguishes two variants of it: the unit and the batch.
	byProductPack map[string]int64
	// byProduct holds every variant the vendor has of a product, in id order.
	// Where they have several, the warehouse and the branch break the tie
	// rather than the map's iteration order.
	byProduct map[int64][]int64

	// inWarehouse is the set of variants that already hold a balance row in the
	// warehouse this import writes to.
	inWarehouse map[int64]bool
	// initialInWarehouse records the snapshot of warehouse variants before any new row was planned.
	initialInWarehouse map[int64]bool
	// branchOf is each variant's branch, so a multi-branch vendor's import
	// prefers the branch the chosen warehouse belongs to.
	branchOf map[int64]int64
	// branchID is the branch the import is writing to, zero when unknown.
	branchID int64
	// active is the subset of live variants currently on sale, which is what
	// the replace mode's preview counts: a variant already off sale is not one
	// the run is about to take off sale.
	active map[int64]bool
	// live is every variant of the vendor's the index knows about, so a variant
	// id carried on a row can be checked in one lookup rather than by walking
	// the whole catalogue — which on a nine-thousand-row file against a
	// ten-thousand-variant catalogue is the difference between a map read and
	// ninety million comparisons.
	live map[int64]bool
	// justWritten are the variants this run created. A second row of the same
	// file about the same product belongs on the variant the first row made,
	// not on a third one beside it — a file that mentions a product twice must
	// not leave the vendor with two copies of it and the quantity on whichever
	// happened to be written last.
	justWritten map[int64]bool
}

// newVariantIndex indexes a vendor's existing variants for one destination.
func newVariantIndex(
	keys []catalog.VariantKey, inWarehouse map[int64]bool, branchID *int64,
) *variantIndex {
	idx := &variantIndex{
		bySKU:              make(map[string]int64, len(keys)),
		byBarcode:          make(map[string]int64, len(keys)),
		productOf:          make(map[int64]int64, len(keys)),
		byProductPack:      make(map[string]int64, len(keys)),
		byProduct:          make(map[int64][]int64, len(keys)),
		inWarehouse:        inWarehouse,
		initialInWarehouse: make(map[int64]bool, len(inWarehouse)),
		branchOf:           make(map[int64]int64, len(keys)),
		live:               make(map[int64]bool, len(keys)),
		active:             make(map[int64]bool, len(keys)),
		justWritten:        map[int64]bool{},
	}
	if inWarehouse != nil {
		for k, v := range inWarehouse {
			if v {
				idx.initialInWarehouse[k] = true
			}
		}
	}
	if idx.inWarehouse == nil {
		idx.inWarehouse = map[int64]bool{}
	}
	if branchID != nil {
		idx.branchID = *branchID
	}

	for _, k := range keys {
		idx.live[k.ID] = true
		if k.Active {
			idx.active[k.ID] = true
		}
		if key := sheet.NormalizeKey(k.SKU); key != "" {
			idx.bySKU[key] = k.ID
		}
		if code := sheet.DigitsOnly(k.Barcode); code != "" {
			idx.byBarcode[code] = k.ID
		}
		if k.BranchID != nil {
			idx.branchOf[k.ID] = *k.BranchID
		}
		if k.ProductID > 0 {
			idx.productOf[k.ID] = k.ProductID
			idx.byProductPack[packKey(k.ProductID, k.Unit, k.BatchNumber)] = k.ID
			idx.byProduct[k.ProductID] = append(idx.byProduct[k.ProductID], k.ID)
		}
	}
	return idx
}

// packKey identifies one packaging of one catalogue product.
func packKey(productID int64, unit, batch string) string {
	return sheet.NormalizeKey(unit) + "|" + sheet.NormalizeKey(batch) + "|" + itoa64(productID)
}

// resolve finds the existing variant a row refers to, and says how.
//
// The order is by how much the identifier means. A variant already recorded
// against this row by an earlier commit attempt is strongest: it is not an
// inference at all, it is what this very row wrote last time. The vendor's own
// item code is next — it is the number they use to mean "this line of my
// catalogue". The barcode follows but is weaker than it looks, since several
// pack sizes of one product legitimately share one, so it only settles a row
// that carried no code. Everything after that is inference and is treated as
// such.
func (idx *variantIndex) resolve(row *productmatch.Row, productID int64, known *int64) (int64, string) {
	if known != nil && *known > 0 && idx.exists(*known) {
		return *known, i18n.TDefault("w4_mod.s_392_392")
	}
	if row != nil {
		if key := sheet.NormalizeKey(row.SKU); key != "" {
			if id, ok := idx.bySKU[key]; ok {
				return id, i18n.TDefault("w4_mod.s_392_392")
			}
		}
		if code := sheet.DigitsOnly(row.Barcode); code != "" {
			if id, ok := idx.byBarcode[code]; ok {
				return id, i18n.TDefault("w4_mod.s_333_333")
			}
		}
	}
	if productID > 0 {
		if row != nil {
			if id, ok := idx.byProductPack[packKey(productID, row.Unit, row.BatchNumber)]; ok {
				return id, i18n.TDefault("w4_mod.s_393_393")
			}
		}
		if id := idx.pickForProduct(productID); id > 0 {
			return id, i18n.TDefault("w4_mod.s_394_394")
		}
	}
	return 0, ""
}

// pickForProduct chooses among the vendor's variants of one catalogue product.
//
// The warehouse decides. A variant that already holds a balance in the
// warehouse this import is writing to is the one this file's quantity belongs
// on; that is the whole reason the index knows which warehouse it is for. The
// branch is the next tie-break, and only where exactly one variant survives is
// an answer given — a genuine tie is still refused, because guessing there
// moves stock between two live offers.
func (idx *variantIndex) pickForProduct(productID int64) int64 {
	candidates := idx.byProduct[productID]
	switch len(candidates) {
	case 0:
		return 0
	case 1:
		return candidates[0]
	}

	if narrowed := filterIDs(candidates, func(id int64) bool { return idx.justWritten[id] }); len(narrowed) == 1 {
		return narrowed[0]
	} else if len(narrowed) > 1 {
		candidates = narrowed
	}

	if narrowed := filterIDs(candidates, func(id int64) bool { return idx.inWarehouse[id] }); len(narrowed) == 1 {
		return narrowed[0]
	} else if len(narrowed) > 1 {
		candidates = narrowed
	}

	if idx.branchID > 0 {
		if narrowed := filterIDs(candidates, func(id int64) bool {
			return idx.branchOf[id] == idx.branchID
		}); len(narrowed) == 1 {
			return narrowed[0]
		} else if len(narrowed) > 1 {
			candidates = narrowed
		}
	}

	if narrowed := filterIDs(candidates, func(id int64) bool {
		return idx.active[id]
	}); len(narrowed) == 1 {
		return narrowed[0]
	} else if len(narrowed) > 1 {
		candidates = narrowed
	}

	return 0
}

// filterIDs keeps the ids the predicate accepts.
func filterIDs(ids []int64, keep func(int64) bool) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if keep(id) {
			out = append(out, id)
		}
	}
	return out
}

// exists reports whether an id is one of the vendor's live variants.
func (idx *variantIndex) exists(id int64) bool { return idx.live[id] }

// mentioned resolves a staged row's identifiers onto one of the vendor's
// variants, for the replace mode's keep-list.
//
// Deliberately looser than resolve: it is answering "does this file talk about
// a product the vendor already stocks", not "which variant does this row write
// to". Where the vendor holds several variants of one product and nothing picks
// between them, resolve refuses — correctly, because writing to the wrong one
// moves stock. Here a refusal would delist all of them, so the question is
// answered per product rather than per row, and every variant of a mentioned
// product is kept.
func (idx *variantIndex) mentioned(m RowMention) int64 {
	if key := sheet.NormalizeKey(m.SourceCode); key != "" {
		if id, ok := idx.bySKU[key]; ok {
			return id
		}
	}
	if code := sheet.DigitsOnly(m.Barcode); code != "" {
		if id, ok := idx.byBarcode[code]; ok {
			return id
		}
	}
	return 0
}

// mentionedProduct lists every variant the vendor has of a catalogue product,
// which is what a row mentioning that product protects.
func (idx *variantIndex) mentionedProduct(productID int64) []int64 {
	if productID <= 0 {
		return nil
	}
	return idx.byProduct[productID]
}

// remember records a newly written variant so a later row in the same file
// updates it rather than inserting a second copy of it.
//
// It also joins the warehouse set: the row that has just been written is,
// by the time the next batch is decided, a variant holding a balance here.
func (idx *variantIndex) remember(row *productmatch.Row, productID, variantID int64) {
	if variantID <= 0 {
		return
	}
	if row != nil {
		if key := sheet.NormalizeKey(row.SKU); key != "" {
			idx.bySKU[key] = variantID
		}
		if code := sheet.DigitsOnly(row.Barcode); code != "" {
			idx.byBarcode[code] = variantID
		}
	}
	idx.live[variantID] = true
	idx.inWarehouse[variantID] = true
	idx.justWritten[variantID] = true
	if idx.branchID > 0 {
		if _, known := idx.branchOf[variantID]; !known {
			idx.branchOf[variantID] = idx.branchID
		}
	}
	if productID > 0 {
		if row != nil {
			idx.byProductPack[packKey(productID, row.Unit, row.BatchNumber)] = variantID
		}
		for _, id := range idx.byProduct[productID] {
			if id == variantID {
				return
			}
		}
		idx.byProduct[productID] = append(idx.byProduct[productID], variantID)
	}
}

// itoa64 renders an id without pulling in strconv for one call site.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
