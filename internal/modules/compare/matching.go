package compare

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"math"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// CandidateProduct represents a catalog product candidate evaluated during matching.
type CandidateProduct struct {
	ID                     int64  `json:"id"`
	SKU                    string `json:"sku"`
	NameAr                 string `json:"name_ar"`
	NameEn                 string `json:"name_en"`
	ScientificName         string `json:"scientific_name"`
	ManufacturingCompanies string `json:"manufacturing_companies"`
	Pharmacology           string `json:"pharmacology"`
	// Barcode is the catalogue's GTIN, and it exists because the ladder used
	// to compare a row's barcode against this struct's SKU for want of
	// anywhere else to look.
	Barcode      string       `json:"barcode"`
	Price        money.Amount `json:"price"`
	Image        string       `json:"image"`
	SearchSimple string       `json:"search_simple"`
}

// MatchResult encapsulates the resolved product match and confidence score.
type MatchResult struct {
	ProductID   *int64      `json:"product_id"`
	Confidence  float64     `json:"confidence"` // 0..100 scale (matching Laravel parity)
	Method      MatchMethod `json:"method"`
	MethodLabel string      `json:"method_label"`
}

// AIMatcher provides AI-augmented matching for low-confidence rows via the platform gateway (Rule R2, Rule R3).
type AIMatcher interface {
	MatchCandidate(ctx context.Context, query string, candidateNames []string) (string, float64)
}

// MatchLadder resolves a single uploaded row against the catalog using the deterministic match ladder (Plan V5 Phase 2 §2.4).
// Strategy 0: Saved Customer Product Mapping (100%)
// Strategy 1: SKU / Barcode Match (100%)
// Strategy 2: Exact Name Match (100%)
// Strategy 3: Trigram / Candidate Fuzzy Match (>= 90% or >= 60%)
// Strategy 4: First Meaningful Word / Token-subset Match (>= 55%)
// Strategy 5: Unmatched (< 55%)
func (s *Service) MatchLadder(ctx context.Context, orgID *int64, rawName string, sku string, barcode string, candidates []*CandidateProduct) (*MatchResult, error) {
	rawName = strings.TrimSpace(rawName)
	if rawName == "" {
		return &MatchResult{
			ProductID:   nil,
			Confidence:  0,
			Method:      MatchMethodUnmatched,
			MethodLabel: i18n.TDefault("w4_mod.s_364_364"),
		}, nil
	}

	normName := normalizeProductText(rawName)
	cleanSKU := strings.ToLower(strings.TrimSpace(sku))
	cleanBarcode := strings.ToLower(strings.TrimSpace(barcode))

	// -----------------------------------------------------------------------
	// Strategy 0: Saved Customer Product Mappings (catalog.customer_product_mappings)
	// -----------------------------------------------------------------------
	if s.repo != nil {
		if mappedID, err := s.repo.GetSavedProductMapping(ctx, orgID, rawName); err == nil && mappedID != nil {
			return &MatchResult{
				ProductID:   mappedID,
				Confidence:  100.0,
				Method:      MatchMethodSavedMapping,
				MethodLabel: i18n.TDefault("w4_mod.w4str_160_160"),
			}, nil
		}
	}

	// -----------------------------------------------------------------------
	// Strategy 1: identifiers — handled by the shared engine, not here
	// -----------------------------------------------------------------------
	//
	// This tool used to run its own code and barcode tiers, and both broke the
	// rule internal/shared/productmatch/identifiers.go exists to state.
	//
	// The barcode tier compared the ROW'S BARCODE against the CANDIDATE'S SKU.
	// CandidateProduct carries no barcode at all, so there was nothing else it
	// could compare against — it was not a near miss, it was two different
	// numbering schemes tested for equality and returned at confidence 100.
	//
	// The code tier accepted a bare SKU collision with no corroboration from
	// the name, which is exactly what every other importer refuses: a supplier's
	// "951" coincides with a catalogue code by accident more often than by
	// design, and a wrongly linked row prices the wrong medicine with no
	// downstream check to catch it.
	//
	// Both now go through Index.Match, under the same options the vendor
	// import, the admin catalogue import, the saving list and the smart order
	// use. The code tier is allowed only where the file actually carries a code,
	// and never authoritatively, so productmatch demands a unique catalogue hit
	// AND an agreeing name before it settles anything.

	// -----------------------------------------------------------------------
	// Strategy 2: Exact Name Match
	// -----------------------------------------------------------------------
	for _, c := range candidates {
		cNormAr := normalizeProductText(c.NameAr)
		cNormEn := normalizeProductText(c.NameEn)
		if (cNormAr != "" && cNormAr == normName) || (cNormEn != "" && cNormEn == normName) {
			return &MatchResult{
				ProductID:   &c.ID,
				Confidence:  100.0,
				Method:      MatchMethodExactName,
				MethodLabel: i18n.TDefault("w4_mod.s_373_373"),
			}, nil
		}
	}

	// -----------------------------------------------------------------------
	// Strategy 3: the shared matching engine
	// -----------------------------------------------------------------------
	//
	// This used to be a similarity blend of its own, and it was the loosest
	// matcher on the platform by a wide margin. It compared the row against
	// each candidate's name, its scientific name, its MANUFACTURER and its
	// PHARMACOLOGY, took the best of the five, and applied anything at 0.60 —
	// with no strength check, no dosage-form check and no line-extension check.
	//
	// Reading a manufacturer as a product name is not a near miss. "GSK" scores
	// against every row that mentions GSK, and the ladder then returned one
	// arbitrary GSK product at ninety per cent confidence. The same blend also
	// made 500 mg and 1 g interchangeable, which is the single mistake this
	// platform's engine exists to make impossible.
	//
	// So the candidates go through internal/shared/productmatch, exactly as the
	// smart order, the vendor import, the saving list and the master-catalogue
	// import do: the same rarity weighting, the same dose and form and
	// line-extension discrimination, the same refusal to choose between two
	// products the row cannot separate.
	if res, ok := s.matchAgainst(rawName, cleanSKU, cleanBarcode, candidates); ok {
		return res, nil
	}

	// -----------------------------------------------------------------------
	// Wave B: AI Gateway Enhancement (Plan V5 Phase 2 §2.6)
	// Only runs on rows the deterministic matcher left below the confidence cutoff (Rule R3).
	// -----------------------------------------------------------------------
	if s.aiMatcher != nil && len(candidates) > 0 {
		var candidateNames []string
		candMap := make(map[string]*CandidateProduct)
		for _, c := range candidates {
			candidateNames = append(candidateNames, c.NameAr)
			candMap[c.NameAr] = c
			if c.NameEn != "" {
				candidateNames = append(candidateNames, c.NameEn)
				candMap[c.NameEn] = c
			}
		}

		matchedName, aiScore := s.aiMatcher.MatchCandidate(ctx, rawName, candidateNames)
		if matchedName != "" && aiScore >= 0.60 {
			if matchedCand, ok := candMap[matchedName]; ok {
				aiConf := math.Round(aiScore * 100.0)
				return &MatchResult{
					ProductID:   &matchedCand.ID,
					Confidence:  aiConf,
					Method:      MatchMethodAI,
					MethodLabel: fmt.Sprintf(i18n.TDefault("w4_mod.d_164"), int(aiConf)),
				}, nil
			}
		}
	}

	// -----------------------------------------------------------------------
	// Strategy 5: Unmatched (< 55%)
	// -----------------------------------------------------------------------
	return &MatchResult{
		ProductID:   nil,
		Confidence:  0.0,
		Method:      MatchMethodUnmatched,
		MethodLabel: i18n.TDefault("w4_mod.s_364_364"),
	}, nil
}

// SaveManualCorrection records user-corrected match in compare.file_rows and persists it to catalog.customer_product_mappings (Plan V5 §2.4.3).
func (s *Service) SaveManualCorrection(ctx context.Context, orgID *int64, rowID int64, rawName string, productID int64) error {
	rawName = strings.TrimSpace(rawName)
	if rawName == "" || productID <= 0 {
		return apperr.Validation("correction.invalid", "Invalid product or name for correction.", nil)
	}

	// 1. Update the row in compare.file_rows
	if err := s.repo.UpdateFileRowMatch(ctx, rowID, &productID, MatchMethodManual, 100.0); err != nil {
		return err
	}

	// 2. Persist to catalog.customer_product_mappings for future auto-match reuse
	return s.repo.SaveCustomerProductMapping(ctx, orgID, rawName, productID, "manual")
}

// Helper utilities
//
// The normalisation and similarity functions that used to live here now live in
// internal/shared/productmatch, so that compare, ingest and smart ordering all
// answer "are these the same product name" the same way. These wrappers keep
// compare's call sites unchanged.

func normalizeProductText(s string) string { return productmatch.NormalizeText(s) }

// matchAgainst scores one row against its candidates with the shared engine.
//
// The index is built per call because the ladder's signature is per row and its
// candidate set is a shortlist the caller has already narrowed — a few dozen
// products, not the catalogue. A caller matching a whole file should build the
// index once and use productmatch.MatchAll; this exists so the per-row entry
// point cannot answer differently from the rest of the platform.
func (s *Service) matchAgainst(
	rawName, sku, barcode string, candidates []*CandidateProduct,
) (*MatchResult, bool) {
	if len(candidates) == 0 {
		return nil, false
	}

	masters := make([]productmatch.MasterProduct, 0, len(candidates))
	for _, c := range candidates {
		if c == nil || c.ID <= 0 {
			continue
		}
		// The manufacturer and the pharmacology travel in their own fields,
		// where the engine treats them as corroboration that can never decide a
		// match on its own. That is the whole difference from the blend this
		// replaced.
		masters = append(masters, productmatch.MasterProduct{
			ID:           c.ID,
			NameAR:       c.NameAr,
			NameEN:       c.NameEn,
			SKU:          c.SKU,
			Barcode:      c.Barcode,
			Scientific:   c.ScientificName,
			Manufacturer: c.ManufacturingCompanies,
		})
	}
	if len(masters) == 0 {
		return nil, false
	}

	// The identifier tiers are enabled from what the file actually carries.
	//
	// A value in the code slot means the wizard bound a code column to it, and
	// that is the "mapped" half of the consent rule in identifiers.go. It is
	// never authoritative here, so a code hit still has to survive
	// Index.matchByCode's two guards: exactly one catalogue product carries
	// that code, and its name agrees with the row. A barcode has to be a real
	// GTIN — eight digits or more, one hit — before it settles anything.
	opts := productmatch.DefaultMatchOptions().WithIdentifiers(
		productmatch.MappedColumns{Code: sku != "", Barcode: barcode != ""},
		productmatch.IdentifierChoices{ByCode: sku != "", ByBarcode: barcode != ""},
	)

	idx := productmatch.NewIndex(masters)
	res := idx.Match(&productmatch.Row{
		Name:    strings.TrimSpace(rawName),
		SKU:     strings.TrimSpace(sku),
		Barcode: strings.TrimSpace(barcode),
	}, opts)

	if !res.Matched() || !res.Level.Settled() {
		// Ambiguity and review both come back unmatched. Two products that fit
		// equally well is not a weaker match; it is a question, and this tool
		// has a screen for asking it.
		return nil, false
	}

	confidence := math.Round(res.Score * 100.0)
	method, label := compareMethodFor(res.Level, confidence)
	id := res.ProductID
	return &MatchResult{
		ProductID:   &id,
		Confidence:  confidence,
		Method:      method,
		MethodLabel: label,
	}, true
}

// compareMethodFor renders the engine's level in the vocabulary this tool's
// stored rows and its screens already use.
func compareMethodFor(level productmatch.MatchLevel, confidence float64) (MatchMethod, string) {
	switch level {
	case productmatch.MatchBarcode:
		return MatchMethodBarcode, i18n.TDefault("w4_mod.s_372_372")
	case productmatch.MatchCode:
		return MatchMethodSKU, i18n.TDefault("w4_mod.sku_161")
	case productmatch.MatchExact:
		return MatchMethodExactName, i18n.TDefault("w4_mod.s_373_373")
	default:
		return MatchMethodFuzzy, fmt.Sprintf(i18n.TDefault("w4_mod.d_162"), int(confidence))
	}
}
