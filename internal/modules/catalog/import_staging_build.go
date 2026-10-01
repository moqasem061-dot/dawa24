package catalog

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// buildStagingRows decides what each parsed product would do under the chosen
// mode, and packages it for review.
func buildStagingRows(
	parsed *ParseResult, matches map[int]ExistingMatch, mode ImportMode,
) []*StagingRow {
	issuesByRow := groupIssuesByRow(parsed.Issues)
	rows := make([]*StagingRow, 0, len(parsed.Products))

	for i, product := range parsed.Products {
		sourceRow := 0
		if i < len(parsed.SourceRows) {
			sourceRow = parsed.SourceRows[i]
		}

		row := &StagingRow{
			SourceRow: sourceRow,
			Product:   product,
			Included:  true,
			Issues:    issuesByRow[sourceRow],
		}
		if match, matched := matches[i]; matched {
			id := match.ProductID
			row.MatchedProductID = &id
			row.MatchReason = match.Reason
			row.MatchState = string(matchflow.StateAccepted)
			row.MatchBasis = string(matchflow.BasisSimilarity)
			if match.Reason == MatchSKU || match.Reason == MatchBarcode {
				row.MatchBasis = string(matchflow.BasisIdentifier)
			} else if match.Reason == MatchName {
				row.MatchBasis = string(matchflow.BasisExact)
			} else if match.Reason == MatchAI {
				row.MatchBasis = string(matchflow.BasisAI)
			}
			row.MatchGranularity = string(matchflow.GranularityProduct)

			dec := matchflow.RowDecision{
				V:         1,
				State:     matchflow.StateAccepted,
				Basis:     matchflow.Basis(row.MatchBasis),
				ProductID: &id,
				Granularity: matchflow.GranularityPair{
					Required: matchflow.GranularityProduct,
					Achieved: matchflow.GranularityProduct,
				},
			}
			row.MatchDecision = &dec
			row.BaselineDecision = &dec
			row.Proposal = ClassifyProposal(dec, 0)
		} else {
			dec := matchflow.RowDecision{
				V:     1,
				State: matchflow.StateNoCandidate,
				Granularity: matchflow.GranularityPair{
					Required: matchflow.GranularityProduct,
				},
			}
			row.MatchState = string(matchflow.StateNoCandidate)
			row.MatchDecision = &dec
			row.BaselineDecision = &dec
			row.Proposal = ClassifyProposal(dec, 0)
		}

		row.Action = actionFor(mode, row.MatchedProductID != nil)
		if row.Action == ActionSkip {
			row.Included = false
		}

		// A row carrying a blocking finding is never silently committed, whatever
		// the mode says. The admin sees it in the review table as excluded.
		if row.HasErrors() {
			row.Action = ActionSkip
			row.Included = false
		}
		// Nor is a row the two matchers disagree about, or that is duplicate or unresolved.
		// The proposed product stays on the row so the administrator can see what was nearly
		// done to the shared catalogue, and the row waits for them to say.
		if row.MatchReason == MatchDisputed || row.Proposal == ProposalDuplicate || row.Proposal == ProposalUnresolved {
			row.Action = ActionSkip
			row.Included = false
		}
		rows = append(rows, row)
	}
	return rows
}

// actionFor applies the chosen strategy to one row.
func actionFor(mode ImportMode, matched bool) RowAction {
	switch mode {
	case ModeAddNewOnly:
		if matched {
			return ActionSkip
		}
		return ActionInsert

	case ModeUpdateExistingOnly:
		if matched {
			return ActionUpdate
		}
		return ActionSkip

	case ModeClearAndAdd:
		// The catalogue is archived first, so every row is new by definition —
		// including the ones that match something about to be retired.
		return ActionInsert

	default: // ModeUpdateAndAdd
		if matched {
			return ActionUpdate
		}
		return ActionInsert
	}
}

// groupIssuesByRow indexes findings so each staged row carries its own.
func groupIssuesByRow(issues []RowIssue) map[int][]RowIssue {
	out := map[int][]RowIssue{}
	for _, issue := range issues {
		out[issue.Row] = append(out[issue.Row], issue)
	}
	return out
}
