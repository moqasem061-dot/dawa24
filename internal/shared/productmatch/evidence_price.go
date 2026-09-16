package productmatch

// The printed price, as evidence of identity.
//
// In Egypt the public price — سعر الجمهور — is printed on the pack and is the
// same figure at every pharmacy in the country. Two lines carrying the same
// printed price for the same brand are, in practice, the same pack; two packs
// of the same brand at different printed prices are different packs. That makes
// it one of the strongest identity signals a supplier file carries, and until
// now the engine did not read it at all.
//
// It was not a small omission. Of the rows nine live supplier files failed to
// match, the great majority had a candidate whose printed price agreed to the
// piastre — بانتولوك at 56.00 against بانتولوك at 56.00, refused over a carton
// count.
//
// Agreement only, and never on its own. Round prices repeat across the
// catalogue and a price alone would match a hundred unrelated products, so this
// is corroboration in the same sense the dose and the form are: it is scaled by
// how much of the name agreed, and it cannot lift a candidate the name says
// nothing about. Disagreement is deliberately NOT a conflict: a supplier's
// printed price goes stale between price revisions, and the catalogue's does
// too, and refusing a match over that would undo far more than it fixed.

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// priceAgreementBonus is what an exactly equal printed price is worth.
//
// Sized against the dose bonus deliberately. A milligram figure and a printed
// price are similar evidence: both are stated by both sides, both are exact,
// and both are shared by enough unrelated products that neither settles a match
// alone.
const priceAgreementBonus = 0.10

// priceNearBonus is what a printed price within a piastre or two is worth.
//
// Rounding differs between a distributor's system and the catalogue on prices
// carrying a half piastre, and refusing those the full credit would penalise
// exactly the rows the tolerance exists for.
const priceNearBonus = 0.06

// priceTolerance is how far apart two printed prices may be and still be read
// as the same price, in minor units.
const priceTolerance = 2

// parsePriceMinor reads a catalogue price column into minor units, returning 0
// for anything unparseable or non-positive.
//
// The column is a string because that is how it leaves the database, and a
// zero-priced product is one the catalogue has no printed price for — silence,
// not a price of nothing.
func parsePriceMinor(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	amount, err := money.Parse(raw)
	if err != nil || !amount.IsPositive() {
		return 0
	}
	return amount.Minor()
}

// priceBonus is what the printed prices agreeing is worth, and zero where
// either side did not state one.
func priceBonus(rowMinor, productMinor int64) float64 {
	if rowMinor <= 0 || productMinor <= 0 {
		return 0
	}
	switch delta := rowMinor - productMinor; {
	case delta == 0:
		return priceAgreementBonus
	case delta <= priceTolerance && delta >= -priceTolerance:
		return priceNearBonus
	}
	return 0
}
