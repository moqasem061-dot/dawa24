package catalog

import (
	"context"
	"log/slog"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// Scoring a file in pieces, so somebody can watch it and stop it.
//
// The scoring pass was one call. It took the whole file, returned when it was
// done, and between those two moments the import could not say where it had
// reached, could not be cancelled, and could not be timed. An administrator
// watching a twenty-thousand-row file saw the bar report "0 of 19,996" at the
// first number of the matching band and sit there — not because the work was
// stuck, but because the only two progress reports in the phase were one before
// the call and one after it.
//
// That is the bug behind every "it hangs at N%" report this tool has had, and
// it is not fixed by making the work faster. A step that cannot report is
// indistinguishable from a step that has died, so the fix is for it to report.
//
// Three things fall out of chunking, and all three were missing:
//
//   - The bar moves, on real counts rather than on a drift.
//   - ctx is checked between chunks, so "إلغاء العملية" stops the run and the
//     thirty-minute ceiling is enforced instead of being noticed afterwards.
//   - Each pass is timed and logged, so "it was slow" becomes a number naming
//     which pass was slow.
//
// The chunk only decides how often the loop comes up for air.

// matchChunk is how many rows are scored between progress reports.
//
// Five hundred is a fifth of a second of work on the measured throughput, which
// is frequent enough that the bar moves visibly and rare enough that the
// reporting costs nothing. It is not a tuning knob: below about a hundred the
// per-chunk overhead starts to show, and above a few thousand the bar goes back
// to looking stuck on a large file.
const matchChunk = 500

// scoreRows scores every row against the catalogue, reporting as it goes.
//
// done is called after each chunk with the number of rows scored so far. The
// returned slice is parallel to rows; a cancelled run returns what it had and
// the error, and the caller decides whether a partial answer is worth anything
// (it is not — prepare stops).
func scoreRows(
	ctx context.Context, log *slog.Logger, matcher matching.Service, index *productmatch.Index, rows []*productmatch.Row,
	opts productmatch.MatchOptions, done func(scored int),
) ([]productmatch.MatchResult, error) {
	out := make([]productmatch.MatchResult, 0, len(rows))
	start := time.Now()

	for from := 0; from < len(rows); from += matchChunk {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		to := min(from+matchChunk, len(rows))
		chunk := rows[from:to]

		if matcher != nil {
			reqRows := make([]productmatch.Row, len(chunk))
			for i, r := range chunk {
				if r != nil {
					reqRows[i] = *r
				}
			}
			prepRes, err := matcher.Prepare(ctx, productmatch.PrepareRequest{
				Scope: productmatch.Scope{
					Workflow: productmatch.WorkflowCatalog,
				},
				Rows:     reqRows,
				Required: productmatch.GranularityProduct,
			})
			if err != nil {
				return out, err
			}
			for _, dec := range prepRes.Decisions {
				cols := matchflow.Columns(dec)
				var prodID int64
				if cols.MatchedProductID != nil {
					prodID = *cols.MatchedProductID
				} else if dec.ProductID != nil && cols.MatchLevel != "none" {
					prodID = *dec.ProductID
				}

				score := 0.0
				switch dec.State {
				case matchflow.StateAccepted, matchflow.StateConfirmed:
					if dec.Basis == matchflow.BasisIdentifier || dec.Basis == matchflow.BasisExact {
						score = 1.0
					} else {
						score = 0.9
					}
				case matchflow.StateNeedsReview:
					score = 0.6
				case matchflow.StateAmbiguous:
					score = 0.5
				}

				reason := cols.MatchLevel
				if len(dec.Reasons) > 0 {
					reason = string(dec.Reasons[0].Code)
				}

				res := productmatch.MatchResult{
					ProductID: prodID,
					Level:     productmatch.MatchLevel(cols.MatchLevel),
					Score:     score,
					Reason:    reason,
				}
				if index != nil {
					for _, c := range dec.Candidates {
						if cp, ok := index.Lookup(c.ProductID); ok {
							res.Candidates = append(res.Candidates, productmatch.MatchCandidate{
								ProductID:     cp.ID,
								Name:          cp.NameAR,
								Scientific:    cp.Scientific,
								DosageForm:    cp.DosageForm,
								Concentration: cp.Concentration,
								Manufacturer:  cp.Manufacturer,
							})
						}
					}
				}
				out = append(out, res)
			}
		} else {
			out = append(out, productmatch.MatchAll(index, chunk, opts, 0)...)
		}
		done(to)
	}

	log.DebugContext(ctx, "catalogue similarity scoring complete",
		"rows", len(rows), "took", time.Since(start).Round(time.Millisecond))
	return out, nil
}

// recallRows retrieves an AI shortlist for every row, reporting as it goes.
//
// Same shape and same reasons as scoreRows. It is a separate pass rather than a
// reuse of the scorer's candidates because by the time a row reaches the model
// the scorer has already decided it cannot answer, and handing over the top of
// the pool that defeated it asks a question the shortlist has answered wrongly.
func recallRows(
	ctx context.Context, log *slog.Logger, index *productmatch.Index, rows []*productmatch.Row,
	opts productmatch.RecallOptions, done func(retrieved int),
) ([][]productmatch.MatchCandidate, error) {
	out := make([][]productmatch.MatchCandidate, 0, len(rows))
	start := time.Now()

	for from := 0; from < len(rows); from += matchChunk {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		to := min(from+matchChunk, len(rows))
		out = append(out, productmatch.RecallAll(index, rows[from:to], opts, 0)...)
		done(to)
	}

	log.DebugContext(ctx, "ai shortlist retrieval complete",
		"rows", len(rows), "took", time.Since(start).Round(time.Millisecond))
	return out, nil
}

// matchingProgress builds the progress reporter for one pass of the matching
// phase.
//
// The phase runs one pass over the residue without AI and two with it, and both
// share the phase's single band. Each pass therefore advances the count by its
// own share, so the bar crosses the band once rather than running to the end on
// the scoring pass and sitting still through retrieval — which is the same
// complaint one step further along.
//
// settled is what the exact tiers already resolved, counted so the bar reflects
// the whole file rather than only the part still in question.
func matchingProgress(progress ProgressFunc, settled, residual, total, pass, passes int) func(int) {
	return func(processed int) {
		if passes <= 0 {
			return
		}
		advanced := (pass*residual + processed) / passes
		progress.report(ImportPhaseMatching, min(settled+advanced, total), total)
	}
}
