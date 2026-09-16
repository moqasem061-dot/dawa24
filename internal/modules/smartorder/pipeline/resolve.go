// Package pipeline runs a smart order from staged rows to selected suppliers.
//
// Every stage is a set operation over the whole file. None of them issues a
// query per row — that constraint (FR-017a) is what keeps a ten-thousand-line
// import inside its five-minute budget, and it is the single easiest thing to
// regress. A test asserts the query count does not scale with row count.
//
// The order is deliberate. Cheap, exact tiers run first over the whole file and
// settle the majority; only what survives them reaches the fuzzy scorer, and
// only what survives *that* is eligible for AI. On a typical pharmacy file the
// funnel is roughly 10,000 rows in, a few hundred reaching the scorer, and tens
// reaching adjudication.
package pipeline

import (
	"context"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// Cutoff separates "resolved" from "needs help".
//
// A line at or above this is never sent for AI adjudication and is never
// overwritten by it. Below it, the line is a candidate for AI and — if still
// below afterwards — is reported honestly as unmatched rather than guessed into
// the order. A wrong confident match is a worse failure than an honest
// non-match, because the pharmacy receives the wrong medicine and has no signal
// that anything went wrong.
const Cutoff = 0.850

// Confidence assigned by each deterministic tier. These are not guesses: an
// exact barcode is the same physical package, a buyer's own confirmed mapping is
// their own assertion, and a fuzzy name match is a judgement.
const (
	confSavingProduct  = 1.000
	confLearnedMapping = 1.000
	confBarcode        = 1.000
	confSKU            = 1.000
	confExactName      = 0.980
	confFuzzyDB        = 0.960
	confAlias          = 0.950
	confContains       = 0.930
)

// Resolver holds what the deterministic tiers need.
type Resolver struct {
	repo smartorder.Repository
	cfg  *smartorder.Config
}

// NewResolver constructs the deterministic stage.
func NewResolver(repo smartorder.Repository, cfg *smartorder.Config) *Resolver {
	return &Resolver{repo: repo, cfg: cfg}
}

// Normalize computes the comparison keys for every line, in memory.
//
// Pure CPU with no I/O, so it parallelises freely and costs nothing in round
// trips. Done as its own pass because every later tier compares against these
// keys, and computing them lazily inside each tier would repeat the work.
func Normalize(lines []*smartorder.Line) {
	for _, l := range lines {
		cleaned := productmatch.StripTradeAnnotations(l.RawName)
		l.NormName = productmatch.NormalizeText(cleaned)
	}
}

// Resolve applies the deterministic tiers to every unresolved line.
//
// Each tier is one query for the whole file. After each, the resolved lines drop
// out and the next tier sees a smaller set.
func (r *Resolver) Resolve(ctx context.Context, lines []*smartorder.Line) error {
	// Tier 0 — Saving Products. First because it is the buyer's own explicit
	// assertion about their own vocabulary, and because on live data 8,777 of
	// 8,778 entries carry a catalogue link. Skipped entirely when the toggle is
	// off: FR-015 requires that the links are then consulted by no tier at all.
	if r.cfg.UseSavingProducts {
		if err := r.applySaving(ctx, lines); err != nil {
			return err
		}
	}

	// Tier 1 — the buyer's confirmed corrections. This is what makes the third
	// import of a recurring file need no manual work.
	if err := r.applyLearned(ctx, lines); err != nil {
		return err
	}

	// Tiers 2 and 3 — barcode and SKU, one query covering both.
	if err := r.applyCodes(ctx, lines); err != nil {
		return err
	}

	// Tier 4 — exact names. Matches the normalised name directly against the
	// catalogue's Arabic and English product names. This is the tier that was
	// previously missing: a pharmacy typing the same name the catalogue carries
	// would fall through to the scorer, and from there to AI or to "unmatched".
	// On a typical pharmacy file it resolves the bulk of the residue.
	if err := r.applyExactNames(ctx, lines); err != nil {
		return err
	}

	// Tier 5 — aliases confirmed against the shared catalogue.
	if err := r.applyAliases(ctx, lines); err != nil {
		return err
	}

	// Tier 6 — substring containment against catalogue names.
	if err := r.applyContains(ctx, lines); err != nil {
		return err
	}

	// Tier 7 — PostgreSQL trigram fuzzy matching.
	return r.applyFuzzyDB(ctx, lines)
}

func (r *Resolver) applySaving(ctx context.Context, lines []*smartorder.Line) error {
	names, skus := unresolvedKeys(lines)
	if len(names) == 0 && len(skus) == 0 {
		return nil
	}
	hits, err := r.repo.ResolveBySaving(ctx, r.cfg.OrganizationID, names, skus)
	if err != nil {
		return err
	}
	assign(lines, hits, smartorder.MethodSavingProduct, confSavingProduct, func(l *smartorder.Line) []string {
		return []string{l.NormName, strings.ToLower(strings.TrimSpace(l.RawSKU))}
	})
	return nil
}

func (r *Resolver) applyLearned(ctx context.Context, lines []*smartorder.Line) error {
	names, _ := unresolvedKeys(lines)
	if len(names) == 0 {
		return nil
	}
	hits, err := r.repo.ResolveByLearned(ctx, r.cfg.OrganizationID, names)
	if err != nil {
		return err
	}
	assign(lines, hits, smartorder.MethodLearnedMapping, confLearnedMapping, func(l *smartorder.Line) []string {
		return []string{l.NormName}
	})
	return nil
}

func isStandardBarcode(b string) bool {
	b = strings.TrimSpace(b)
	if len(b) < 8 || len(b) > 18 {
		return false
	}
	for _, r := range b {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (r *Resolver) applyCodes(ctx context.Context, lines []*smartorder.Line) error {
	var skus, barcodes []string
	seenSKU := make(map[string]bool)
	seenBarcode := make(map[string]bool)
	for _, l := range lines {
		if l.Matched() {
			continue
		}
		if sku := strings.ToLower(strings.TrimSpace(l.RawSKU)); sku != "" && !seenSKU[sku] {
			seenSKU[sku] = true
			skus = append(skus, sku)
		}
		if b := strings.ToLower(strings.TrimSpace(l.RawBarcode)); b != "" && isStandardBarcode(b) {
			if seenBarcode[b] {
				continue
			}
			seenBarcode[b] = true
			barcodes = append(barcodes, b)
		}
	}
	if len(skus) == 0 && len(barcodes) == 0 {
		return nil
	}
	hits, err := r.repo.ResolveByCodes(ctx, skus, barcodes)
	if err != nil {
		return err
	}

	for _, l := range lines {
		if l.Matched() {
			continue
		}
		if sku := strings.ToLower(strings.TrimSpace(l.RawSKU)); sku != "" {
			if id, ok := hits[sku]; ok {
				setMatch(l, id, smartorder.MethodSKU, confSKU)
				continue
			}
		}
		if b := strings.ToLower(strings.TrimSpace(l.RawBarcode)); b != "" && isStandardBarcode(b) {
			if id, ok := hits[b]; ok {
				setMatch(l, id, smartorder.MethodBarcode, confBarcode)
			}
		}
	}
	return nil
}

func (r *Resolver) applyExactNames(ctx context.Context, lines []*smartorder.Line) error {
	names, _ := unresolvedKeys(lines)
	if len(names) == 0 {
		return nil
	}
	hits, err := r.repo.ResolveByExactName(ctx, names, r.cfg.MatchLanguage)
	if err != nil {
		return err
	}
	assign(lines, hits, smartorder.MethodExactName, confExactName, func(l *smartorder.Line) []string {
		return []string{l.NormName}
	})
	return nil
}

func (r *Resolver) applyFuzzyDB(ctx context.Context, lines []*smartorder.Line) error {
	names, _ := unresolvedKeys(lines)
	if len(names) == 0 {
		return nil
	}
	hits, err := r.repo.ResolveByFuzzyDB(ctx, names, r.cfg.MatchLanguage)
	if err != nil {
		// Non-fatal tier: skip and let residual fall through to Stage 3 in-memory matcher
		return nil
	}
	assign(lines, hits, smartorder.MethodFuzzy, confFuzzyDB, func(l *smartorder.Line) []string {
		return []string{l.NormName}
	})
	return nil
}

func (r *Resolver) applyContains(ctx context.Context, lines []*smartorder.Line) error {
	names, _ := unresolvedKeys(lines)
	if len(names) == 0 {
		return nil
	}
	hits, err := r.repo.ResolveByContains(ctx, names, r.cfg.MatchLanguage)
	if err != nil {
		// Non-fatal tier: skip and let residual fall through to Stage 3 in-memory matcher
		return nil
	}
	assign(lines, hits, smartorder.MethodFuzzy, confContains, func(l *smartorder.Line) []string {
		return []string{l.NormName}
	})
	return nil
}

func (r *Resolver) applyAliases(ctx context.Context, lines []*smartorder.Line) error {
	names, _ := unresolvedKeys(lines)
	if len(names) == 0 {
		return nil
	}
	hits, err := r.repo.ResolveByAlias(ctx, names)
	if err != nil {
		return err
	}
	assign(lines, hits, smartorder.MethodAlias, confAlias, func(l *smartorder.Line) []string {
		return []string{l.NormName}
	})
	return nil
}

// unresolvedKeys collects the lookup keys of lines still needing a match.
func unresolvedKeys(lines []*smartorder.Line) (names, skus []string) {
	seenName := make(map[string]bool)
	seenSKU := make(map[string]bool)
	for _, l := range lines {
		if l.Matched() {
			continue
		}
		if l.NormName != "" && !seenName[l.NormName] {
			seenName[l.NormName] = true
			names = append(names, l.NormName)
		}
		if s := strings.ToLower(strings.TrimSpace(l.RawSKU)); s != "" && !seenSKU[s] {
			seenSKU[s] = true
			skus = append(skus, s)
		}
	}
	return names, skus
}

// assign applies a tier's hits to whichever lines its keys match.
func assign(lines []*smartorder.Line, hits map[string]int64, method smartorder.MatchMethod,
	confidence float64, keysOf func(*smartorder.Line) []string) {
	if len(hits) == 0 {
		return
	}
	for _, l := range lines {
		if l.Matched() {
			continue
		}
		for _, k := range keysOf(l) {
			if k == "" {
				continue
			}
			if id, ok := hits[k]; ok {
				setMatch(l, id, method, confidence)
				break
			}
		}
	}
}

// setMatch records a resolution, never overwriting an earlier, stronger tier.
func setMatch(l *smartorder.Line, productID int64, method smartorder.MatchMethod, confidence float64) {
	if l.Matched() && l.MatchConfidence >= confidence {
		return
	}
	l.MatchedProductID = &productID
	l.MatchMethod = method
	l.MatchConfidence = confidence
}

// Unresolved returns the lines still below the cutoff — the only ones the fuzzy
// scorer and, after it, AI are permitted to touch.
func Unresolved(lines []*smartorder.Line) []*smartorder.Line {
	var out []*smartorder.Line
	for _, l := range lines {
		if !l.Matched() || l.MatchConfidence < Cutoff {
			out = append(out, l)
		}
	}
	return out
}
