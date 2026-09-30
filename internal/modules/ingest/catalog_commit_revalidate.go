package ingest

import (
	"context"
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// revalidateRows checks every staged row against Matcher.Revalidate before writing.
// Rows whose revision changed, whose state is not accepted/confirmed, or whose
// granularity is below pack are rejected and reported in the commit summary.
func (c *commitRun) revalidateRows(ctx context.Context, staged []*RowOutcome) []*RowOutcome {
	if c.svc.matcher == nil || len(staged) == 0 {
		return staged
	}

	sc := productmatch.Scope{
		EffectiveOrg:    c.session.OrganizationID,
		SourceNamespace: fmt.Sprintf("supplier:%d", c.session.OrganizationID),
		Workflow:        productmatch.WorkflowVendor,
		EntryPoint:      "vendor_commit_revalidate",
	}

	valid := make([]*RowOutcome, 0, len(staged))
	for _, row := range staged {
		ref := productmatch.RowRef{
			ID:        row.ID,
			Table:     "ingest.catalog_import_rows",
			RowNumber: row.SourceRow,
		}

		dec := matchflow.RowDecision{
			State:    matchflow.State(row.MatchState),
			Basis:    matchflow.Basis(row.MatchBasis),
			Revision: row.MatchRevision,
			Granularity: matchflow.GranularityPair{
				Required: matchflow.GranularityPack,
				Achieved: matchflow.Granularity(row.MatchGranularity),
			},
		}
		if row.MatchDecision != nil {
			dec = *row.MatchDecision
		} else if row.ProductID != nil {
			dec.ProductID = row.ProductID
		}

		revalDec, err := c.svc.matcher.Revalidate(ctx, sc, ref, dec)
		if err != nil {
			c.skipped++
			c.revalidatedRejections++
			c.record(row, OutcomeSkipped, nil, "فشل التحقق من مطابقة الصنف قبل التثبيت")
			continue
		}

		hasCatalogChanged := false
		for _, f := range revalDec.Flags {
			if f == "catalog_changed" {
				hasCatalogChanged = true
				break
			}
		}

		if revalDec.Revision != row.MatchRevision || hasCatalogChanged {
			c.skipped++
			c.revalidatedRejections++
			c.record(row, OutcomeSkipped, nil, "تم استبعاد الصنف بسبب تغير مراجعة الكتالوج")
			continue
		}

		if revalDec.State != matchflow.StateAccepted && revalDec.State != matchflow.StateConfirmed {
			c.skipped++
			c.revalidatedRejections++
			c.record(row, OutcomeSkipped, nil, "تم استبعاد الصنف لأن المطابقة لم تعد مؤكدة")
			continue
		}

		if revalDec.Granularity.Achieved != matchflow.GranularityPack {
			c.skipped++
			c.revalidatedRejections++
			c.record(row, OutcomeSkipped, nil, "تم استبعاد الصنف لأن مستوى التفاصيل أقل من العبوة")
			continue
		}

		valid = append(valid, row)
	}

	return valid
}
