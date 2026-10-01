package matching

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// Search queries the master catalog for candidate products matching q, presenting diffs and hard conflicts against row.
func (s *serviceImpl) Search(ctx context.Context, sc Scope, q string, row productmatch.Row, limit int) ([]matchflow.CandidateView, error) {
	if sc.Workflow == "" {
		return nil, errors.New("matching: scope workflow is required")
	}

	catRev, err := s.repo.CurrentRevision(ctx, "master")
	if err != nil {
		catRev = 1
	}

	idx, err := s.indexCache.Get(ctx, "master", catRev)
	if err != nil || idx == nil {
		return nil, errors.New("matching: catalog unavailable")
	}

	if s.repo != nil {
		if aliases, aErr := s.repo.LoadAliases(ctx, sc.EffectiveOrg); aErr == nil && len(aliases) > 0 {
			idx = idx.WithAliases(aliases)
		}
	}

	if limit <= 0 {
		limit = 10
	} else if limit > 50 {
		limit = 50
	}

	var candidateIDs []int64
	scores := make(map[int64]float64)
	seen := make(map[int64]bool)

	trimmedQ := strings.TrimSpace(q)
	if trimmedQ != "" {
		if id, parseErr := strconv.ParseInt(trimmedQ, 10, 64); parseErr == nil && id > 0 {
			if _, ok := idx.Lookup(id); ok {
				candidateIDs = append(candidateIDs, id)
				scores[id] = 1.0
				seen[id] = true
			}
		}

		searchRow := row
		searchRow.Name = trimmedQ
		searchRow.Barcode = trimmedQ
		searchRow.SKU = trimmedQ
		collectCandidates(idx, &searchRow, limit, &candidateIDs, scores, seen)
	} else if strings.TrimSpace(row.Name) != "" || row.Barcode != "" || row.SKU != "" {
		collectCandidates(idx, &row, limit, &candidateIDs, scores, seen)
	}

	rowFacts := productmatch.ExtractRowFacts(&row, nil)
	views := make([]matchflow.CandidateView, 0, len(candidateIDs))

	for _, id := range candidateIDs {
		if len(views) >= limit {
			break
		}
		p, ok := idx.Lookup(id)
		if !ok {
			continue
		}

		pFacts := p.Facts()
		comp := productmatch.CompareFacts(rowFacts, pFacts)
		eligible := len(comp.Hard) == 0

		hardConflicts := make([]string, len(comp.Hard))
		for i, h := range comp.Hard {
			hardConflicts[i] = string(h) + "_conflict"
		}

		diffMap := make(map[string]string, len(comp.Attributes))
		for k, v := range comp.Attributes {
			diffMap[k] = string(v)
		}

		views = append(views, matchflow.CandidateView{
			ProductID:     p.ID,
			NameAR:        p.NameAR,
			NameEN:        p.NameEN,
			DosageForm:    p.DosageForm,
			Concentration: p.Concentration,
			Unit:          p.Unit,
			Manufacturer:  p.Manufacturer,
			PublicPrice:   p.PublicPrice,
			Score:         scores[id],
			Eligible:      eligible,
			HardConflicts: hardConflicts,
			Diff:          diffMap,
		})
	}

	return views, nil
}

func collectCandidates(
	idx *productmatch.Index,
	r *productmatch.Row,
	limit int,
	candidateIDs *[]int64,
	scores map[int64]float64,
	seen map[int64]bool,
) {
	opts := productmatch.MatchOptions{
		MinStrong:           0.1,
		MinReview:           0.05,
		PoolLimit:           400,
		MaxCandidates:       limit,
		TrustBarcode:        true,
		TrustSupplierCode:   true,
		CodeIsAuthoritative: true,
	}

	res := idx.Match(r, opts)
	if res.ProductID > 0 && !seen[res.ProductID] {
		*candidateIDs = append(*candidateIDs, res.ProductID)
		scores[res.ProductID] = res.Score
		seen[res.ProductID] = true
	}
	for _, c := range res.Candidates {
		if !seen[c.ProductID] {
			*candidateIDs = append(*candidateIDs, c.ProductID)
			scores[c.ProductID] = c.Score
			seen[c.ProductID] = true
		}
	}
}
