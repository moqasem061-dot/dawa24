package catalog

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// How confident the main-catalogue import has to be before it applies a match.
//
// Split out of import_match.go, which had grown past the 400-line limit
// AGENTS.md sets. Everything here is about the thresholds; import_match.go is
// about the passes that use them.

// MatchSimilar is a match made on name similarity with corroborating
// attributes rather than on an identifier. It updates an existing product, so
// the review screen marks it for a second look.
const MatchSimilar MatchReason = "similar"

// similarityFloor is the score at or above which a similarity match is offered
// as an update on the strength of the name alone.
//
// Higher than any other importer's, because the consequences differ: a vendor's
// wrong match mislabels one of their own offers, while a wrong match here
// overwrites the shared catalogue entry every pharmacy reads.
//
// What that produced, measured on the live table: 37,576 rows staged as
// `insert` with no reason, 82 as `similar`, 26 by name. A match rate of 0.29%,
// which means every re-upload of the same administrative file duplicates the
// catalogue. The floor was not wrong about the risk; it was the only tool being
// used to manage it.
const similarityFloor = 0.86

// corroboratedFloor is the score at or above which a match is offered as an
// update when something other than the name agrees as well.
//
// The engine's own settled levels — a barcode hit, a code hit, an exact folded
// name — are already identifiers rather than judgements. Below those, a strong
// score whose dose and pharmaceutical form both corroborate is a different
// proposition from a strong score on the name alone, and the identity re-check
// that guards the AI tier is available to say so.
//
// So the rule is: a bare name similarity still needs 0.86, and a corroborated
// one needs 0.78 — the figure the vendor import has always used, applied here
// only where the extra evidence exists.
const corroboratedFloor = 0.78

// matchFloors turns the administrator's one setting — أقل نسبة مطابقة — into the
// two floors this importer actually uses.
//
// Every import tool on the platform now offers the same control, defaulting to
// the same 50%, because a setting that means one thing in the vendor import and
// nothing at all in the admin import is worse than no setting. What differs is
// what the number buys: here it is the floor for a match the catalogue's own
// record corroborates — same dose, same pharmaceutical form, no identity
// conflict — and a match with nothing but the name agreeing still has to clear
// the far higher bare-name floor. Lowering the setting widens what a
// corroborated match may be; it never lets an uncorroborated one through.
//
// The bare-name floor is never allowed below its constant, whatever the
// administrator types. That is the one number protecting the row every pharmacy
// on the platform reads.
func matchFloors(minScore float64) (bare, corroborated float64) {
	corroborated = minScore
	if corroborated <= 0 {
		corroborated = productmatch.DefaultMinStrong
	}
	corroborated = min(max(corroborated, productmatch.DefaultMinReview), 1)
	return max(similarityFloor, corroborated), corroborated
}

// aiFloor is the confidence a model must express before its choice is applied.
//
// From the shared table rather than a local number, and the shared table sets
// it higher here than anywhere else — 0.90 against 0.80 — because this is the
// one importer whose wrong match overwrites the catalogue entry every pharmacy
// reads. The local constant was 0.70, which is below what the other two tools
// demand for a decision with a far smaller blast radius.
var aiFloor = catalogCeilings.MinApplyConfidence

// catalogCeilings is what one administrative import may spend, from the shared
// table.
//
// It used to be two constants declared here — 24 batches of 25 rows — and that
// is precisely the drift internal/shared/matchflow was created to stop. The
// vendor importer and the smart order had each grown their own copy of the same
// numbers, every one of them documented as measured, and no two of them
// agreeing. Reading the shared table means a change to what a run may spend is
// one edit rather than three, and the master-catalogue profile is deliberately
// the most conservative of the three because a wrong match here overwrites the
// entry every pharmacy reads.
//
// It also lifts the ceiling this import was working under. 24 by 25 was 600
// rows: on a thirty-thousand-row file that is two per cent of the residue, and
// nothing told the administrator that the other ninety-eight per cent had never
// been looked at. The shared profile allows 12 requests of 100.
var catalogCeilings = matchflow.For(matchflow.ProfileCatalog)

// maxAIAdjudicationBatches bounds the AI tier for one import.
//
// It exists because an import must finish: a fifty-thousand-row file whose
// every row is ambiguous would otherwise spend the afternoon in a model, and
// the deterministic outcome it already has — "this is a new product" — is a
// serviceable answer. Rows past it keep that outcome and the screen says the
// ceiling was reached.
var maxAIAdjudicationBatches = catalogCeilings.MaxRequestsPerRun

// aiBatchSize is how many rows go in one adjudication request. Batched, never
// per row: the same rule the smart order's adjudication follows, for the same
// reason — one request per row turns a three-minute import into an hour.
var aiBatchSize = catalogCeilings.MaxItemsPerRequest

// MatchAdjudicationRequest is one batch of ambiguous rows, attributed to the
// organisation whose import asked for it so AI spend is billed and capped per
// tenant rather than against one platform key.
type MatchAdjudicationRequest struct {
	Items []MatchAdjudicationItem `json:"items"`

	OrganizationID int64 `json:"-"`
	UserID         int64 `json:"-"`
}

// MatchAdjudicationItem is one ambiguous row and the shortlist it may resolve
// to. Nothing else is sent, and the model may only answer with an id from the
// shortlist.
type MatchAdjudicationItem struct {
	// Ref is the caller's index into its own row slice. It travels out and back
	// so an answer can be tied to the row that asked the question.
	Ref        int64
	Text       string
	Candidates []MatchAdjudicationCandidate
	// Settled says the similarity tier already applied CurrentGuess to this
	// row, so the question is verification rather than resolution: is this the
	// right catalogue entry to overwrite, not which entry is it.
	//
	// The tier used to be shown only the rows similarity could not settle,
	// which means the rows it settled WRONGLY — the ones that overwrite a
	// product every pharmacy reads, at a score high enough that nobody looks —
	// were the only rows in the file never examined twice.
	Settled      bool
	CurrentGuess *int64
	CurrentScore float64
}

// MatchAdjudicationCandidate is a catalogue product as the adjudicator sees it.
type MatchAdjudicationCandidate struct {
	ProductID     int64
	Name          string
	NameEN        string
	Scientific    string
	DosageForm    string
	Concentration string
	Manufacturer  string
}

// MatchAdjudicationResult is one decision. A nil ProductID means "none of
// these", which is a useful and frequent answer.
type MatchAdjudicationResult struct {
	Ref        int64
	ProductID  *int64
	Confidence float64
	Reason     string
}

// MatchAdjudicator resolves rows the deterministic tiers left ambiguous.
//
// It is a port rather than a dependency on the gateway, so the import can be
// tested without one and so an unconfigured deployment simply skips the tier.
type MatchAdjudicator interface {
	AdjudicateMatches(ctx context.Context, req MatchAdjudicationRequest) ([]MatchAdjudicationResult, error)
}

// SetMatchAdjudicator installs the AI matching port.
func (s *Service) SetMatchAdjudicator(a MatchAdjudicator) { s.adjudicator = a }

// MatchStats accounts for what each tier resolved, for the review screen.
type MatchStats struct {
	Exact      int `json:"exact"`
	Similar    int `json:"similar"`
	AI         int `json:"ai"`
	Unmatched  int `json:"unmatched"`
	AIRequests int `json:"ai_requests"`
	// Verified counts rows the similarity tier had already settled and the
	// model was asked to check anyway; Disputed counts the ones it would not
	// confirm, each of which is now waiting for an administrator instead of
	// overwriting a catalogue entry.
	Verified int `json:"verified"`
	Disputed int `json:"disputed"`
	// CacheHits counts rows answered from the shared decision cache without a
	// request. Reported because it is the difference between an import that
	// cost money and one that did not.
	CacheHits int `json:"cache_hits"`
	// CeilingHit means the AI tier stopped early and some rows kept their
	// deterministic outcome. Reported so a low match rate on a huge file is not
	// mistaken for a bad catalogue.
	CeilingHit bool `json:"ceiling_hit"`
}

// Matched is how many rows were tied to an existing catalogue product.
func (m MatchStats) Matched() int { return m.Exact + m.Similar + m.AI }

// Total is how many rows the matcher considered.
func (m MatchStats) Total() int { return m.Matched() + m.Unmatched }

// RatePercent is the share of rows tied to an existing product, 0–100.
func (m MatchStats) RatePercent() int {
	if m.Total() == 0 {
		return 0
	}
	return m.Matched() * 100 / m.Total()
}

// acceptsUpdate decides whether a match is safe to apply to the shared
// catalogue without asking an administrator.
//
// Three ways in, in descending order of how little judgement each requires:
//
//   - the engine settled it on an identifier or an exact folded name;
//   - the score clears the bare-name floor;
//   - the score clears the corroborated floor AND the catalogue's own record
//     agrees about the product's identity — the same re-check that validates an
//     answer from the model before it is written.
//
// Ambiguity is refused in every case. Two products that fit equally well is not
// a weaker match; it is a question, and the review screen exists to ask it.
func acceptsUpdate(
	index *productmatch.Index, row *productmatch.Row, res productmatch.MatchResult,
	bare, corroborated float64,
) bool {
	if res.Level == productmatch.MatchAmbiguous {
		return false
	}
	if res.Level == productmatch.MatchBarcode || res.Level == productmatch.MatchCode ||
		res.Level == productmatch.MatchExact {
		return true
	}
	if res.Score >= bare {
		return true
	}
	if res.Score < corroborated {
		return false
	}
	return index.IdentityConflict(row, res.ProductID).None()
}
