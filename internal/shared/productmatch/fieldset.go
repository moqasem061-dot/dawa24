package productmatch

// Narrowing the question a resolver is asked.
//
// The field catalogue is the union of everything any importer can read, and
// that union is wrong for every importer taken alone. A pharmacy uploading a
// shopping list has a name and a quantity; offering its two columns to
// twenty-nine fields invites the resolver to bind one of them to "cost price"
// on weak evidence, and the buyer then sees a mapping screen asking them to
// confirm a field their file does not contain. A saving-products file has four
// columns and the same problem.
//
// Before this existed, each importer answered it by writing its own resolver
// with its own shorter list — which is how the same header ended up meaning the
// item code in one system and the barcode in another. A field set says which
// questions to ask; it does not change how any of them are answered.

// FieldSet is the subset of the field catalogue one importer can use.
//
// A nil FieldSet allows everything, so a caller that has no opinion behaves
// exactly as before.
type FieldSet struct {
	name    string
	allowed map[Field]bool
}

// NewFieldSet builds a set from an explicit list.
func NewFieldSet(name string, fields ...Field) *FieldSet {
	fs := &FieldSet{name: name, allowed: make(map[Field]bool, len(fields))}
	for _, f := range fields {
		fs.allowed[f] = true
	}
	return fs
}

// Name identifies the set in a mapping note, so a vendor reading "لم يتم
// التعرف على عمود" can be told which vocabulary was consulted.
func (fs *FieldSet) Name() string {
	if fs == nil {
		return "all"
	}
	return fs.name
}

// Allows reports whether a field is in the set. A nil set allows everything.
func (fs *FieldSet) Allows(f Field) bool {
	if fs == nil || len(fs.allowed) == 0 {
		return true
	}
	return fs.allowed[f]
}

// Specs lists the catalogue entries this set permits, in catalogue order, so a
// mapping screen renders the same field ordering everywhere.
func (fs *FieldSet) Specs() []Spec {
	out := make([]Spec, 0, len(Specs))
	for _, s := range Specs {
		if fs.Allows(s.Field) {
			out = append(out, s)
		}
	}
	return out
}

// The four sets the platform actually uses.
//
// Each is the answer to "what can this importer do with a column?", and each is
// deliberately no wider than that. A field a system cannot write is a field its
// users should never be asked about.
var (
	// VendorFields is a supplier's own product variants and their stock.
	//
	// It used to be nil, meaning "every field the platform knows", and that was
	// the bug behind a whole class of vendor-import complaints: a supplier was
	// offered التصنيف, المادة الفعالة and الوصف as mapping targets, mapped a
	// column to one of them, and then found the import had written nothing —
	// because those four columns belong to catalog.products, which a vendor
	// import has not been allowed to touch since master-product creation was
	// removed from it. A field an importer cannot write is a field its users
	// must not be asked about.
	//
	// What remains is exactly catalog.product_variants plus the stock ledger,
	// plus the identity columns the match needs in order to find which master
	// product the row is about. The identity columns are read, never written.
	VendorFields = NewFieldSet("vendor",
		// Identity — read to match against the shared catalogue.
		FieldName, FieldNameEN, FieldSKU, FieldBarcode, FieldProductID,
		FieldScientific, FieldManufacturer, FieldDosageForm,
		FieldConcentration, FieldUnit, FieldPackSize,
		// Pricing — the vendor's own commercial terms.
		FieldPublicPrice, FieldPrice, FieldNetPrice, FieldCostPrice, FieldCostDiscountPct,
		FieldDiscountPct, FieldDiscountAmt, FieldBonus,
		// Stock — the vendor's own holding.
		FieldQuantity, FieldExpiryDate, FieldBatchNumber,
		FieldMinOrderQty, FieldMinThreshold, FieldWarehouse, FieldBranch,
		// Variant attributes, all of them columns on product_variants.
		FieldStatus, FieldNegotiable, FieldNotes,
	)

	// CatalogFields is the master catalogue an administrator maintains.
	//
	// It has no stock and no batch, because the shared catalogue records what a
	// medicine *is*, not what anyone is holding. It gains the two description
	// columns and the active ingredient, which only the master record carries.
	CatalogFields = NewFieldSet("catalog",
		FieldName, FieldNameEN, FieldSKU, FieldBarcode, FieldProductID,
		FieldScientific, FieldActiveIngredient,
		FieldManufacturer, FieldDosageForm, FieldConcentration,
		FieldUnit, FieldPackSize, FieldCategory,
		FieldPublicPrice, FieldPrice, FieldCostPrice, FieldCostDiscountPct,
		FieldDiscountPct, FieldDiscountAmt,
		FieldDescription, FieldDescriptionEN,
		FieldStatus, FieldImage,
	)

	// OrderFields is a pharmacy's shopping list: what to buy and how much of
	// it. Nothing else in the file is the pharmacy's business to state — the
	// price and the supplier are what the run works out.
	OrderFields = NewFieldSet("order",
		FieldName, FieldSKU, FieldBarcode, FieldQuantity,
	)

	// SavingFields is a pharmacy's or supplier's own reference list, which
	// carries a price and a quantity alongside the identity so it can be used
	// as a personal catalogue.
	//
	// All four price fields are admitted, not just the public one: a pharmacy
	// records what it paid, a supplier records what it lists, and files in
	// production head that column "سعر الشراء", "سعر البيع" and "الصافي"
	// interchangeably. Admitting one and rejecting the rest left the column
	// unmapped and the entry priced at zero.
	SavingFields = NewFieldSet("saving",
		FieldName, FieldSKU, FieldBarcode, FieldProductID, FieldQuantity,
		FieldPublicPrice, FieldPrice, FieldCostPrice, FieldNetPrice,
	)
)
