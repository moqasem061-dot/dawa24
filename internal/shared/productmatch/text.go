package productmatch

import "github.com/muhiya/dawa24-store/internal/shared/sheet"

// Text primitives shared by every importer.
//
// These moved here from internal/modules/compare, which had grown its own copy
// of normalisation and string similarity alongside the ones in this package.
// Two implementations of "are these the same product name" is one too many:
// they drift, and the drift is invisible until two features disagree about the
// same spreadsheet. The algorithms below are compare's, carried across
// unchanged so that its behaviour is identical before and after the move.
//
// They are deliberately simpler than Index's scorer. Index compares a row
// against thirty thousand catalogue products and can afford precomputation;
// these answer a one-off "how alike are these two strings" and are used for
// header matching and small-set disambiguation.

// NormalizeText reduces a product name to comparable form: folded Arabic
// letters, ASCII digits, lower case, punctuation to spaces, whitespace
// collapsed.
//
// It IS sheet.NormalizeName, and saying so in one line is the point of this
// function. It used to be its own implementation over arabic.Normalize, and the
// two disagreed about six letters — ئ ؤ ڤ چ گ پ, which sheet folds and
// arabic.Normalize does not. That matters because the two normalisers decide
// the same thing from opposite ends: sheet.NormalizeName produces the tokens
// the scorer compares, and this produces the identity KEY those comparisons are
// cached and grouped under (smartorder.run_lines.norm_name, the decision
// memory, compare's market dataset). A pair the engine matched could therefore
// be filed under two different keys, and the answer looked up again a second
// later was not there.
//
// arabic.Normalize is deliberately left alone: it is a parity port of the
// legacy scorer, and its curve is what every stored min_similarity_score was
// tuned against. It is not the right normaliser for an identity key and never
// was.
//
// Both of them turn a dropped character into a space rather than nothing.
// Deleting it glued the figures on either side into one: the pack-offer
// notation every Egyptian price list uses turned "1+1" into "11" and "2+1" into
// "21", so "بانادول 2+1" and "بانادول 21 قرص" normalised alike and could be
// matched to each other.
func NormalizeText(s string) string { return sheet.NormalizeName(s) }
