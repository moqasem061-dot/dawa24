package corpus

// Scoring a labelled run.
//
// The single number this exists to produce is WrongApplied: rows the engine
// settled on its own and got wrong. Everything else in the report is context
// for it.
//
// That asymmetry is the whole design. A missed match costs a person a minute at
// a review screen. An applied wrong match prices one medicine as another,
// publishes it to every pharmacy on the platform, and gives nobody a signal
// that anything happened. The two are not tradeable at par, and a report that
// prints only a match rate invites exactly that trade.

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// Accuracy is what a labelled run produced.
type Accuracy struct {
	Name   string `json:"name"`
	Labels int    `json:"labels"`

	// Applied counts rows the engine settled without asking — the levels
	// MatchLevel.Settled reports true for.
	Applied int `json:"applied"`
	// RightApplied and WrongApplied split that population by whether the
	// product chosen was the labelled one.
	RightApplied int `json:"right_applied"`
	WrongApplied int `json:"wrong_applied"`

	// Offered counts rows given a product below the settle line — review and
	// ambiguous — which a person is asked to confirm.
	Offered      int `json:"offered"`
	RightOffered int `json:"right_offered"`
	WrongOffered int `json:"wrong_offered"`

	// Ambiguous and Review split the offered population by why it was not
	// applied: two candidates nothing in the row separates, or one candidate
	// too weak to apply. They need different fixes and are reported apart.
	Ambiguous      int `json:"ambiguous"`
	AmbiguousRight int `json:"ambiguous_right"`
	Review         int `json:"review"`
	ReviewRight    int `json:"review_right"`

	// Missed counts rows reported as having no match at all.
	Missed int `json:"missed"`
	// TruthInShortlist counts rows whose correct product was somewhere in the
	// candidate list even though it was not chosen. It separates a retrieval
	// failure from a ranking failure, which need different fixes.
	TruthInShortlist int `json:"truth_in_shortlist"`

	// Buckets records, per ten points of reported score, how many settled rows
	// fell there and how many of those were right. A score that means what it
	// says has a rising, monotone accuracy curve; one that does not is a number
	// the review screen should not be printing.
	Buckets [10]Bucket `json:"buckets"`

	// Hard is the same accounting restricted to labels whose brand family has
	// more than one member — the rows where choosing the wrong sibling is
	// possible at all.
	HardLabels  int `json:"hard_labels"`
	HardApplied int `json:"hard_applied"`
	HardWrong   int `json:"hard_wrong"`

	// Samples holds a bounded set of wrong applied matches, for reading.
	Samples []Mistake `json:"samples,omitempty"`
}

// Bucket is one decile of reported score.
type Bucket struct {
	Applied int `json:"applied"`
	Right   int `json:"right"`
}

// Mistake is one applied match that was wrong.
type Mistake struct {
	Query  string  `json:"query"`
	Got    string  `json:"got"`
	Want   string  `json:"want"`
	Score  float64 `json:"score"`
	Level  string  `json:"level"`
	Reason string  `json:"reason,omitempty"`
}

// maxSamples bounds what a report keeps for reading. Enough to see the shape of
// the failures, few enough that the baseline file stays reviewable.
const maxSamples = 25

// Score runs every label through the index and accounts for the outcome.
//
// Parallel across cores because the labelled sets are twenty thousand rows and
// this is run on every change to the scorer; the index is read-only once built,
// which is what makes that safe.
func Score(name string, idx *productmatch.Index, labels []Labelled,
	opts productmatch.MatchOptions) Accuracy {

	type outcome struct {
		res   productmatch.MatchResult
		label Labelled
	}

	results := make([]outcome, len(labels))
	workers := runtime.NumCPU()
	if workers > len(labels) {
		workers = len(labels)
	}
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	chunk := (len(labels) + workers - 1) / workers
	for w := 0; w < workers; w++ {
		start := w * chunk
		end := start + chunk
		if start >= len(labels) {
			break
		}
		if end > len(labels) {
			end = len(labels)
		}
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			for i := lo; i < hi; i++ {
				results[i] = outcome{res: idx.Match(labels[i].Row, opts), label: labels[i]}
			}
		}(start, end)
	}
	wg.Wait()

	acc := Accuracy{Name: name, Labels: len(labels)}
	for _, o := range results {
		hard := o.label.Family > 1
		if hard {
			acc.HardLabels++
		}
		right := o.res.Matched() && o.res.ProductID == o.label.WantID

		switch {
		case o.res.Level.Settled():
			acc.Applied++
			if hard {
				acc.HardApplied++
			}
			b := bucketOf(o.res.Score)
			acc.Buckets[b].Applied++
			if right {
				acc.RightApplied++
				acc.Buckets[b].Right++
			} else {
				acc.WrongApplied++
				if hard {
					acc.HardWrong++
				}
				if len(acc.Samples) < maxSamples {
					acc.Samples = append(acc.Samples, Mistake{
						Query: o.label.Row.DisplayName(),
						Got:   idx.Name(o.res.ProductID),
						Want:  idx.Name(o.label.WantID),
						Score: o.res.Score, Level: string(o.res.Level),
						Reason: o.res.Reason,
					})
				}
			}
		case o.res.Matched():
			acc.Offered++
			if o.res.Level == productmatch.MatchAmbiguous {
				acc.Ambiguous++
				if right {
					acc.AmbiguousRight++
				}
			} else {
				acc.Review++
				if right {
					acc.ReviewRight++
				}
			}
			if right {
				acc.RightOffered++
			} else {
				acc.WrongOffered++
			}
		default:
			acc.Missed++
		}

		if !right {
			for _, c := range o.res.Candidates {
				if c.ProductID == o.label.WantID {
					acc.TruthInShortlist++
					break
				}
			}
		}
	}
	return acc
}

// bucketOf maps a score onto its decile, with 1.0 falling in the top one.
func bucketOf(score float64) int {
	b := int(score * 10)
	if b > 9 {
		b = 9
	}
	if b < 0 {
		b = 0
	}
	return b
}

// PrecisionPct is the share of applied matches that were right, 0–100.
//
// It is reported to one decimal because the interesting movements are in
// tenths: on twenty thousand labels the difference between 99.1% and 98.4% is
// a hundred and forty wrongly priced medicines.
func (a Accuracy) PrecisionPct() float64 {
	if a.Applied == 0 {
		return 0
	}
	return float64(a.RightApplied) * 100 / float64(a.Applied)
}

// RecallPct is the share of all labels the engine applied correctly.
func (a Accuracy) RecallPct() float64 {
	if a.Labels == 0 {
		return 0
	}
	return float64(a.RightApplied) * 100 / float64(a.Labels)
}

// HardPrecisionPct is precision restricted to brand families of more than one.
func (a Accuracy) HardPrecisionPct() float64 {
	if a.HardApplied == 0 {
		return 0
	}
	return float64(a.HardApplied-a.HardWrong) * 100 / float64(a.HardApplied)
}

// Format renders the headline numbers on one line.
func (a Accuracy) Format() string {
	return fmt.Sprintf(
		"%-14s labels=%-6d applied=%-6d WRONG=%-5d precision=%6.2f%%  recall=%6.2f%%  "+
			"hard-precision=%6.2f%% (wrong=%d/%d)  ambiguous=%d(ok=%d)  review=%d(ok=%d)  "+
			"missed=%d  truth-in-list=%d",
		a.Name, a.Labels, a.Applied, a.WrongApplied, a.PrecisionPct(), a.RecallPct(),
		a.HardPrecisionPct(), a.HardWrong, a.HardApplied,
		a.Ambiguous, a.AmbiguousRight, a.Review, a.ReviewRight,
		a.Missed, a.TruthInShortlist)
}

// Calibration renders the reported-score buckets, which is how a score is
// judged rather than asserted.
func (a Accuracy) Calibration() string {
	var b strings.Builder
	b.WriteString("  score bucket | applied | right | actual accuracy\n")
	for i := range a.Buckets {
		if a.Buckets[i].Applied == 0 {
			continue
		}
		pct := float64(a.Buckets[i].Right) * 100 / float64(a.Buckets[i].Applied)
		fmt.Fprintf(&b, "  %2d–%3d%%      | %7d | %5d | %6.2f%%\n",
			i*10, i*10+10, a.Buckets[i].Applied, a.Buckets[i].Right, pct)
	}
	return b.String()
}

// FormatSamples renders the kept mistakes, worst score first — a high score on
// a wrong answer is a worse defect than a low one.
func (a Accuracy) FormatSamples() string {
	s := make([]Mistake, len(a.Samples))
	copy(s, a.Samples)
	sort.SliceStable(s, func(i, j int) bool { return s[i].Score > s[j].Score })
	var b strings.Builder
	for _, m := range s {
		fmt.Fprintf(&b, "  %-6s %.2f | %-44s | got: %-40s | want: %s\n",
			m.Level, m.Score, trunc(m.Query, 44), trunc(m.Got, 40), trunc(m.Want, 40))
	}
	return b.String()
}

// trunc shortens a name to fit a fixed-width report column.
func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
