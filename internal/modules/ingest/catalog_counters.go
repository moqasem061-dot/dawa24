package ingest

import "github.com/muhiya/dawa24-store/internal/shared/productmatch"

// How a run accounts for itself.
//
// One vocabulary, shared by the staging pass and the commit, so the numbers on
// the review screen and the numbers on the results screen mean the same thing.

// counters are what the staging pass tallies: how each row was classified, and
// how many the reader refused outright. The commit keeps its own totals on the
// run that produces them.
type counters struct {
	errors    int
	matched   int
	review    int
	unmatched int
}

// matchBucket is which of the three match counters a row belongs to. It is kept
// beside the row so the AI tier can move it from one to another without having
// to re-derive where it started.
type matchBucket int

const (
	bucketMatched matchBucket = iota
	bucketReview
	bucketUnmatched
)

func bucketOf(m productmatch.MatchResult) matchBucket {
	switch {
	case m.Level.Settled():
		return bucketMatched
	case m.Level == productmatch.MatchReview || m.Level == productmatch.MatchAmbiguous:
		return bucketReview
	default:
		return bucketUnmatched
	}
}

// appendMessage joins two notes about the same row without inventing an empty
// separator when only one of them exists.
func appendMessage(base, extra string) string {
	switch {
	case extra == "":
		return base
	case base == "":
		return extra
	default:
		return base + " — " + extra
	}
}
