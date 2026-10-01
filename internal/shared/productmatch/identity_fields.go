package productmatch

// Telling two products apart when all we have is their columns.
//
// IdentityConflict answers "can this ROW be this INDEXED PRODUCT" and needs an
// Index to do it. Two callers need the same answer without one:
//
//   - the importer folding duplicate rows of a single spreadsheet together,
//     where neither side is in the catalogue yet;
//   - the exact-name tier of the master-catalogue upsert, which runs inside a
//     transaction over rows read straight out of catalog.products.
//
// Both used to compare names and nothing else. What that produced, on the
// catalogue this platform actually holds: "اوجمنتين 375 مجم" and "اوجمنتين 1 جم"
// share a normalised name, so the upsert matched them to one row and the second
// strength overwrote the first — and inside one file the folding step merged
// them outright, so the second strength never reached the review screen at all.
// Same medicine, different dose, different product, one row.
//
// Discriminate is that comparison done properly, over the fields a catalogue
// row and a parsed spreadsheet row both carry. It reuses the same dose, form
// and line-extension logic IdentityConflict runs on, so the importer, the
// scorer and the adjudicator refuse the same pairs for the same reasons.
//
// What it deliberately does NOT do is require agreement. Silence is not
// disagreement: a catalogue that records the dose only inside the product name,
// as much of this one does, is silent constantly, and demanding a stated dose
// on both sides would turn every sparse row into a duplicate. Only attributes
// present on BOTH sides can conflict.

// ProductFields is a product's identifying attributes as a catalogue row or a
// spreadsheet row states them.
//
// Every field is optional. An empty one is "not stated", which never conflicts
// with anything.
type ProductFields struct {
	NameAR        string
	NameEN        string
	Concentration string
	DosageForm    string
	Unit          string
}

// text is everything the field set says about identity, in one string.
//
// The dose and form readers parse prose, and a catalogue writes the strength
// inside the name as often as it fills the column. Concatenating means a
// product that states 500 mg in its name and one that states it in its
// concentration column are read the same way, which is the entire point of
// comparing fields rather than columns.
func (f ProductFields) text() string {
	return f.NameAR + " " + f.NameEN + " " + f.Concentration + " " + f.Unit
}

// nameText is the part that carries the brand and its line extensions.
//
// The concentration is left out on purpose: "بلس" in a name is Panadol Plus and
// "plus" in a strength column is not a word anybody wrote to name a product.
func (f ProductFields) nameText() string {
	return f.NameAR + " " + f.NameEN
}

// formText is everything that could name a pharmaceutical form.
func (f ProductFields) formText() string {
	return f.NameAR + " " + f.NameEN + " " + f.DosageForm
}

// Discriminate reports whether two products cannot be the same product.
//
// Its zero value — MatchConflict{} — means nothing separates them, which is NOT
// the same as "they are the same product": the caller still has to have decided
// the names refer to one thing. This says only that no stated attribute
// forbids it.
//
// The checks run in the order a person would: the dose first, because two
// strengths of one brand are always two products and the check has no false
// positives to trade away; then the line extension, because that is the
// reported failure — بانادول against بانادول اكسترا — and it is a word rather
// than a number; then the form, which is the weakest of the three because
// catalogues and pharmacies name forms loosely.
func Discriminate(a, b ProductFields) MatchConflict {
	if strengthsConflict(a.text(), b.text()) {
		return MatchConflict{Kind: "strength", Detail: "اختلاف التركيز"}
	}
	aName, bName := a.nameText(), b.nameText()
	if mod, differ := modifierSetsDiffer(
		modifiersIn(aName), modifiersIn(bName), aName, bName,
	); differ {
		return MatchConflict{Kind: "modifier", Detail: "اختلاف في صنف المنتج داخل نفس العلامة: " + mod}
	}
	if fieldFormsConflict(a, b) {
		return MatchConflict{Kind: "form", Detail: "اختلاف الشكل الصيدلي"}
	}
	return MatchConflict{}
}

// Separable is Discriminate as a yes-or-no, for callers that only need to know
// whether to keep two rows apart.
func Separable(a, b ProductFields) bool { return !Discriminate(a, b).None() }

// fieldFormsConflict compares two pharmaceutical forms under the same
// collapsing rules the indexed comparison uses: tablets and capsules are one
// class, a wash never vetoes, and a form neither side names cannot disagree.
func fieldFormsConflict(a, b ProductFields) bool {
	aForm := vetoableForm(formKeyOf(a.formText()))
	bForm := vetoableForm(formKeyOf(b.formText()))
	if aForm == "" || bForm == "" {
		return false
	}
	return aForm != bForm
}
