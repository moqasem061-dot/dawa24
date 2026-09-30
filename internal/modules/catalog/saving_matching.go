package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// SavingMatchRow is an input row to match against the catalogue.
type SavingMatchRow struct {
	Index     int
	Name      string
	SKU       string
	Barcode   string
	ProductID *int64
	Quantity  float64
	Price     money.Amount
}

// SavingMatchItem is the result of matching an individual row.
type SavingMatchItem struct {
	Index             int
	ProductID         *int64
	MasterProductName string
	MasterProductSKU  string
	MatchType         string
	Confidence        float64
	RowDecision       *matchflow.RowDecision
}

// BuildSavingMatchEngine loads matching products and returns an in-memory matching engine.
func (s *Service) BuildSavingMatchEngine(ctx context.Context) (*SavingProductMatchEngine, error) {
	sources, err := s.ListMatchProducts(database.AsSystem(ctx))
	if err != nil {
		return nil, err
	}
	return NewSavingProductMatchEngine(sources), nil
}

// MatchSavingsRows executes matching across source rows using matching.Service when available,
// falling back to the in-memory engine.
func (s *Service) MatchSavingsRows(
	ctx context.Context,
	scope productmatch.Scope,
	choice SavingMatchChoice,
	rows []SavingMatchRow,
) ([]SavingMatchItem, *SavingProductMatchEngine, error) {
	engine, err := s.BuildSavingMatchEngine(ctx)
	if err != nil && s.matchingSvc == nil {
		return nil, nil, err
	}

	choice = choice.Normalize()
	items := make([]SavingMatchItem, len(rows))

	// Try matching.Service.Prepare when wired
	if s.matchingSvc != nil && len(rows) > 0 {
		reqRows := make([]productmatch.Row, len(rows))
		for i, r := range rows {
			reqRow := productmatch.Row{
				Number: r.Index,
				Name:   strings.TrimSpace(r.Name),
			}
			code := strings.TrimSpace(r.SKU)
			bcode := strings.TrimSpace(r.Barcode)
			if bcode != "" {
				reqRow.Barcode = bcode
			}
			if code != "" {
				// T066: single column never fed to both
				if bcode == "" && choice.ByBarcode && !choice.ByCode {
					reqRow.Barcode = code
				} else if bcode == "" && choice.ByBarcode && choice.ByCode && isBarcodeCandidate(code) {
					reqRow.Barcode = code
				} else {
					reqRow.SKU = code
				}
			}
			reqRows[i] = reqRow
		}

		identPolicy := productmatch.IdentifierPolicy{
			AllowGTIN:         choice.ByBarcode,
			AllowSupplierCode: choice.ByCode,
			AllowCatalogCode:  choice.CodeIsCatalogCode,
		}

		prepReq := productmatch.PrepareRequest{
			Scope:       scope,
			ConsumerRef: fmt.Sprintf("catalog.saving_products:%d", scope.EffectiveOrg),
			SourceRev:   1,
			Rows:        reqRows,
			Required:    productmatch.GranularityProduct,
			Identifiers: &identPolicy,
		}

		prepRes, pErr := s.matchingSvc.Prepare(ctx, prepReq)
		if pErr == nil && len(prepRes.Decisions) == len(rows) {
			for i, dec := range prepRes.Decisions {
				r := rows[i]
				item := SavingMatchItem{
					Index:       r.Index,
					RowDecision: &prepRes.Decisions[i],
				}

				// Direct ProductID in source file takes precedence if valid and enabled
				if choice.ByProductID && r.ProductID != nil && *r.ProductID > 0 && engine != nil && engine.Known[*r.ProductID] {
					item.ProductID = r.ProductID
					item.MatchType = "direct_id"
					item.Confidence = 1.0
				} else if (dec.State == matchflow.StateAccepted || dec.State == matchflow.StateConfirmed) && dec.ProductID != nil && *dec.ProductID > 0 {
					item.ProductID = dec.ProductID
					item.Confidence = 1.0
					if dec.Basis == matchflow.BasisIdentifier {
						if len(dec.Reasons) > 0 && strings.Contains(string(dec.Reasons[0].Code), "barcode") {
							item.MatchType = "barcode"
						} else {
							item.MatchType = "exact_sku"
						}
					} else if dec.Basis == matchflow.BasisExact {
						item.MatchType = "exact_name"
					} else {
						item.MatchType = "fuzzy_name"
						item.Confidence = 0.85
					}
				} else {
					item.MatchType = "unlinked"
				}

				if item.ProductID != nil && engine != nil {
					item.MasterProductName, item.MasterProductSKU = engine.Describe(*item.ProductID)
				}
				items[i] = item
			}
			return items, engine, nil
		}
	}

	// Fallback to in-process match engine
	for i, r := range rows {
		item := SavingMatchItem{Index: r.Index, MatchType: "unlinked"}
		if engine != nil {
			rawCode := r.SKU
			if rawCode == "" {
				rawCode = r.Barcode
			}
			res := engine.MatchUnified(choice, r.ProductID, rawCode, r.Name)
			if res.ProductID != nil {
				item.ProductID = res.ProductID
				item.MatchType = res.MatchType
				item.Confidence = res.Confidence
				item.MasterProductName, item.MasterProductSKU = engine.Describe(*res.ProductID)
			}
		}
		items[i] = item
	}

	return items, engine, nil
}

// RevalidateSavingDecision checks a saving product decision prior to commit against matching.Service.
func (s *Service) RevalidateSavingDecision(
	ctx context.Context,
	scope productmatch.Scope,
	rowRef productmatch.RowRef,
	dec matchflow.RowDecision,
) (matchflow.RowDecision, error) {
	if s.matchingSvc == nil {
		return dec, nil
	}
	return s.matchingSvc.Revalidate(ctx, scope, rowRef, dec)
}
