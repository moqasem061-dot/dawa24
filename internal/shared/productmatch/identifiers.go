package productmatch

// Deciding whether an identifier column may settle a match at all.
//
// A barcode hit and a code hit both bypass scoring entirely: they return before
// the name, the dose or the pharmaceutical form is ever looked at. When the
// column really does hold the catalogue's own identifier that is exactly right
// and enormously valuable. When it does not, it is the worst failure this
// engine has, because there is nothing downstream to catch it — the row is
// linked at confidence 1.0 to a product that shares nothing with it but a
// number, and the review screen shows it as certain.
//
// Two things made that likely rather than theoretical:
//
//  1. Barcode matching ran unconditionally. Any value of eight digits or more
//     in the barcode field with a single hit in the catalogue won, whatever the
//     names said. A pharmacy's internal item numbering is very often eight or
//     nine digits.
//  2. The savings import forced BOTH identifier options on for every file
//     regardless of what the user had mapped, and fed the SAME column into the
//     code slot and the barcode slot — so one internal item number was offered
//     to both tiers, and the barcode tier does not even consult the name.
//
// The rule here is the one the operator asked for, and it is the right one: an
// identifier tier is off unless the user both mapped that column in step one
// AND switched the tier on. Mapping alone is not consent — a file can carry a
// code column that is the supplier's own numbering — and consent without a
// mapped column is meaningless.

// MappedColumns records which identifier columns the user bound in step one.
//
// It is what makes a toggle offerable. A tool must not present "match by
// barcode" for a file with no barcode column: the switch would do nothing, and
// a user who turns it on and sees no change concludes the matching is broken.
type MappedColumns struct {
	// Code is true when a كود صنف column was mapped.
	Code bool
	// Barcode is true when a باركود column was mapped.
	Barcode bool
}

// IdentifierChoices is what the user actually switched on.
//
// Every field defaults to false, and that zero value is the intended default
// for every tool. A toggle that starts on is one the user never chose.
type IdentifierChoices struct {
	// ByCode allows the file's item code to settle a match.
	ByCode bool
	// ByBarcode allows the file's barcode to settle a match.
	ByBarcode bool
	// CodeIsCatalogCode declares that the mapped code column holds دوا 24's
	// own codes rather than the file owner's internal numbering. Only then is a
	// code hit accepted without the name agreeing as well.
	CodeIsCatalogCode bool
}

// WithIdentifiers applies the user's identifier choices, honouring what they
// actually mapped.
//
// A choice whose column was never mapped is dropped rather than obeyed. That is
// not defensiveness about a form: the two are set in different steps of a
// wizard, a user can map a code column, switch the toggle on, then go back and
// unmap the column, and the stored settings would otherwise still say "trust
// the code" for a column that is no longer bound to anything.
func (o MatchOptions) WithIdentifiers(mapped MappedColumns, chosen IdentifierChoices) MatchOptions {
	o.TrustSupplierCode = mapped.Code && chosen.ByCode
	o.TrustBarcode = mapped.Barcode && chosen.ByBarcode
	// Authority is a claim about the code column, so it cannot outlive the
	// tier it qualifies.
	o.CodeIsAuthoritative = o.TrustSupplierCode && chosen.CodeIsCatalogCode
	return o
}

// OfferableIdentifiers reports which identifier toggles a tool should show for
// a given mapping, so a screen can hide a switch that could not do anything.
func OfferableIdentifiers(mapped MappedColumns) IdentifierChoices {
	return IdentifierChoices{
		ByCode:            mapped.Code,
		ByBarcode:         mapped.Barcode,
		CodeIsCatalogCode: mapped.Code,
	}
}

// MappedIdentifiers reports which identifier columns a confirmed mapping binds.
//
// It is the bridge between step one of a wizard and the matching options, so a
// tool never has to decide for itself what "the user mapped a code column"
// means.
func (m *Mapping) MappedIdentifiers() MappedColumns {
	if m == nil {
		return MappedColumns{}
	}
	_, hasCode := m.Column(FieldSKU)
	_, hasBarcode := m.Column(FieldBarcode)
	return MappedColumns{Code: hasCode, Barcode: hasBarcode}
}
