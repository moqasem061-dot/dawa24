package ingest

import (
	"context"
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// revalidateRows checks every staged row against Matcher.Revalidate before
// writing, and drops only the rows whose association no longer holds.
//
// Three things a row's commit must NOT depend on, each of which once skipped
// rows that were right:
//
//   - The decision record's revision against the row's revision column. Both
//     are read from the same row at the same moment; the AI applier and every
//     review action bump the column, and a compare-and-set at write time is what
//     guards concurrent edits. Comparing the two here rejected every row a
//     model had improved or a person had touched.
//   - Whether anything in the catalogue changed since staging. Only the
//     product this row is tied to matters, and Revalidate checks exactly that.
//   - A pack-level match where the import was prepared at product level.
//
// The row's columns are what review actions write and the decision record is
// what matching wrote, so the columns win for the state and the product: a
// vendor's own confirmation is the most recent word on the row.
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
		if row.MatchDecision == nil && row.MatchState == "" {
			// Staged before decisions were recorded: nothing to revalidate
			// against, and the staging rules of its own time admitted it.
			valid = append(valid, row)
			continue
		}

		dec := matchflow.RowDecision{Granularity: matchflow.GranularityPair{
			Required: matchflow.GranularityProduct,
		}}
		if row.MatchDecision != nil {
			dec = *row.MatchDecision
		}
		if row.MatchState != "" {
			dec.State = matchflow.State(row.MatchState)
		}
		if row.MatchBasis != "" {
			dec.Basis = matchflow.Basis(row.MatchBasis)
		}
		if row.ProductID != nil {
			id := *row.ProductID
			if dec.ProductID == nil || *dec.ProductID != id {
				// A reviewer chose another product; the fingerprint recorded
				// for the engine's choice says nothing about this one.
				dec.Versions.Target = ""
			}
			dec.ProductID = &id
		}
		if dec.State == matchflow.StateConfirmed && dec.Granularity.Achieved == "" {
			dec.Granularity.Achieved = matchflow.GranularityProduct
		}

		ref := productmatch.RowRef{ID: row.ID, Table: "ingest.catalog_import_rows", RowNumber: row.SourceRow}
		revalDec, err := c.svc.matcher.Revalidate(ctx, sc, ref, dec)
		if err != nil {
			c.skipped++
			c.revalidatedRejections++
			c.record(row, OutcomeSkipped, nil, "فشل التحقق من مطابقة الصنف قبل التثبيت")
			continue
		}

		switch {
		case hasFlag(revalDec.Flags, "catalog_changed"):
			c.skipped++
			c.revalidatedRejections++
			c.record(row, OutcomeSkipped, nil, "تم استبعاد الصنف لأن بيانات الصنف المختار تغيرت في الكتالوج")
		case revalDec.State != matchflow.StateAccepted && revalDec.State != matchflow.StateConfirmed:
			c.skipped++
			c.revalidatedRejections++
			c.record(row, OutcomeSkipped, nil, "تم استبعاد الصنف لأن المطابقة لم تعد مؤكدة")
		case granularityBelow(revalDec.Granularity.Achieved, revalDec.Granularity.Required):
			c.skipped++
			c.revalidatedRejections++
			c.record(row, OutcomeSkipped, nil, "تم استبعاد الصنف لأن مستوى التفاصيل أقل من العبوة")
		default:
			valid = append(valid, row)
		}
	}
	return valid
}

func hasFlag(flags []string, want string) bool {
	for _, f := range flags {
		if f == want {
			return true
		}
	}
	return false
}

// granularityBelow reports an achieved granularity short of the required one.
// An unset requirement is product level, which every accepted row meets.
func granularityBelow(achieved, required matchflow.Granularity) bool {
	rank := func(g matchflow.Granularity) int {
		switch g {
		case matchflow.GranularityPack:
			return 2
		case matchflow.GranularityProduct:
			return 1
		}
		return 0
	}
	if required == "" {
		required = matchflow.GranularityProduct
	}
	if achieved == "" {
		achieved = matchflow.GranularityProduct
	}
	return rank(achieved) < rank(required)
}
