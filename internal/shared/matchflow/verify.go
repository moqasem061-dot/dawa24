package matchflow

// Deciding what to do with an answer.
//
// Four importers each wrote this themselves, and they had drifted in the way
// that matters: the smart order refused an answer the identity guard rejected,
// the saving list refused it, the master-catalogue import refused it, and each
// of them recorded a different thing afterwards. Worse, all four agreed on the
// one rule that turns out to be wrong — a row the deterministic engine had
// already settled was never asked about at all, so the model could confirm
// nothing and correct nothing, and the engine's confident mistakes were the
// only mistakes in the file nobody ever looked at twice.
//
// Asking about a settled row changes what an answer means. A model that names a
// different product for a row the engine merely guessed at is offering a better
// guess, and taking it costs nothing. A model that names a different product for
// a row the engine APPLIED is disagreeing with a decision, and the honest
// response to two methods disagreeing is neither "the model wins" nor "the
// engine wins" — it is to stop and ask, because the one thing both would agree
// on is that this row is not obvious.
//
// That is the whole of Verdict.

// Judgement is everything needed to decide one answer, gathered by the caller.
type Judgement struct {
	// Settled says the deterministic engine applied its own answer.
	Settled bool
	// Current is what the engine chose, or nil where it chose nothing.
	Current *int64
	// Offered says the product the model named was in the window it was shown.
	// An id that was not is a hallucination and is never applied.
	Offered bool
	// Conflicts says the catalogue's own record of the proposed product
	// contradicts the row — a different strength, a different form, a different
	// line extension. See productmatch.Index.IdentityConflict.
	Conflicts bool
	// Confidence is what the model said about its own answer.
	Confidence float64
	// Floor is the confidence below which an answer is recorded but not acted
	// on, from the run's ceilings.
	Floor float64
}

// Verdict decides what to do with one answer.
//
// The rules, in the order they are checked, and the reason for each:
//
//  1. A confidence outside [0,1] is a malformed response. Nothing is done.
//  2. A product the model was not shown is a hallucination. Nothing is done,
//     and this is the guard that stops an invented id becoming an order.
//  3. Below the floor the model has said it does not know. A settled row keeps
//     its match; an unsettled one keeps its guess.
//  4. "None of these" on an unsettled row is a real answer and changes nothing.
//     On a SETTLED row it is a rejection of an applied match, and the row goes
//     to a person.
//  5. A proposed product the catalogue's own record contradicts is refused. On
//     an unsettled row the deterministic outcome stands; on a settled row the
//     match is withdrawn, because the model disagreed AND the disagreement
//     survived a mechanical re-check.
//  6. The same product the engine chose is a confirmation.
//  7. A different product on an unsettled row is applied — that is what the
//     stage is for. On a settled row it is a disagreement between two methods
//     that were each confident, and it goes to a person rather than to either.
func Verdict(j Judgement, proposed *int64) Outcome {
	if j.Confidence < 0 || j.Confidence > 1 {
		return OutcomeKeep
	}
	if proposed != nil && *proposed > 0 && !j.Offered {
		return OutcomeKeep
	}
	if j.Confidence < j.Floor {
		return OutcomeKeep
	}

	if proposed == nil || *proposed <= 0 {
		if j.Settled {
			return OutcomeReview
		}
		return OutcomeKeep
	}
	if j.Conflicts {
		if j.Settled && !sameID(j.Current, proposed) {
			return OutcomeReview
		}
		return OutcomeKeep
	}
	if sameID(j.Current, proposed) {
		return OutcomeKeep
	}
	if j.Settled {
		return OutcomeReview
	}
	return OutcomeApply
}

func sameID(a, b *int64) bool {
	return a != nil && b != nil && *a == *b
}

// Remember reports whether an answer is worth writing to the shared decision
// cache.
//
// A hesitant answer is used but not remembered: the cache is per organisation
// and long-lived, an entry written today answers the same question for months
// for every tool that asks it, and nobody is shown that it was a guess.
//
// An answer the identity guard overruled is never remembered either, whatever
// its confidence. Caching it would save a request next time and would also
// freeze a judgement the guard may later make differently — the modifier
// vocabulary grows, the catalogue gains a product — and a cached wrong premise
// outlives the run that made it.
func Remember(j Judgement, proposed *int64) bool {
	if j.Confidence < MinMemoryConfidence {
		return false
	}
	if proposed != nil && *proposed > 0 && (j.Conflicts || !j.Offered) {
		return false
	}
	return true
}
