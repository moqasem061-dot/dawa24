package matchflow

// Turning a proposal and its pair check into one answer, enforcing per-row options
// and progressive emission.

import "context"

// pending is one answer waiting on its pair check.
type pending struct {
	answer Answer
	item   Item
	target int64
}

// handle runs one request: propose, check if configured, hand back progressively.
func (r *runner) handle(ctx context.Context, req Request) {
	batch := req.Batch
	batch.Feature = r.spec.Feature
	batch.OrganizationID = r.spec.OrganizationID
	batch.UserID = r.spec.UserID

	decided := r.propose(ctx, batch, req.Keys, 0)

	var checks []pending
	for _, it := range batch.Items {
		d, ok := decided[it.Ref]
		if !ok {
			continue
		}
		key := req.Keys[it.Ref]
		a := Answer{Key: key, Decision: d, Proposed: d.ProductID, Offered: true}

		if d.ProductID != nil {
			offeredInOptions := false
			if len(it.Options) > 0 {
				for _, opt := range it.Options {
					if opt == *d.ProductID {
						offeredInOptions = true
						break
					}
				}
			} else {
				_, offeredInOptions = req.Offered[*d.ProductID]
			}

			if !offeredInOptions {
				a.Offered = false
				a.Decision.ProductID = nil
				a.Decision.Reason = "ai_rejected_not_offered"
				r.count(func(s *RunStats) {
					s.NotOffered++
					s.Rejected++
				})
				r.emit(a)
				continue
			}
		} else {
			// Model abstained / returned null
			r.count(func(s *RunStats) {
				s.ProposedNull++
				s.Abstained++
			})
			r.emit(a)
			continue
		}

		if target, ok := r.needsCheck(it, a); ok {
			checks = append(checks, pending{answer: a, item: it, target: target})
			continue
		}

		// Direct emission without verification check
		if a.Decision.ProductID != nil && a.Offered {
			if a.Decision.Confidence < r.floor || a.Decision.Confidence > 1 {
				r.count(func(s *RunStats) {
					s.ProposedLow++
					s.Rejected++
				})
				a.Decision.ProductID = nil
			} else if r.spec.Screen != nil && !r.spec.Screen(a.Key, *a.Decision.ProductID) {
				r.count(func(s *RunStats) {
					s.Screened++
					s.Rejected++
				})
				a.Decision.ProductID = nil
			} else {
				r.count(func(s *RunStats) {
					s.Emitted++
					s.AcceptedNew++
				})
			}
		}
		r.emit(a)
	}

	if len(checks) == 0 {
		return
	}

	results := r.verify(ctx, batch, checks)
	for i, p := range checks {
		chk, ok := results[i]
		r.emit(r.resolve(p, chk, ok))
	}
}

func (r *runner) emit(a Answer) {
	if r.spec.OnAnswer != nil {
		r.spec.OnAnswer(a)
	}
}

// needsCheck decides whether an answer is put to the pair check.
// Verification is gated by spec.VerifyPass (default off) and counts toward attempt limit.
func (r *runner) needsCheck(it Item, a Answer) (int64, bool) {
	if !r.spec.VerifyPass || r.verifier == nil {
		return 0, false
	}
	d := a.Decision
	if d.Confidence < r.floor || d.Confidence > 1 {
		if d.ProductID != nil {
			r.count(func(s *RunStats) { s.ProposedLow++ })
		}
		return 0, false
	}
	if d.ProductID == nil {
		r.count(func(s *RunStats) { s.ProposedNull++ })
		return 0, false
	}
	if !a.Offered {
		r.count(func(s *RunStats) { s.NotOffered++ })
		return 0, false
	}
	if r.spec.Screen != nil && !r.spec.Screen(a.Key, *d.ProductID) {
		r.count(func(s *RunStats) { s.Screened++ })
		return 0, false
	}

	if it.Settled {
		if it.CurrentGuess == nil || sameID(it.CurrentGuess, d.ProductID) {
			return 0, false
		}
		if !r.recordAttempt(a.Key) {
			return 0, false
		}
		return *it.CurrentGuess, true
	}

	// Verify attempt counts toward the 3-attempt cap.
	if !r.recordAttempt(a.Key) {
		return 0, false
	}

	return *d.ProductID, true
}

func sameID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// resolve combines a proposal with its pair check.
func (r *runner) resolve(p pending, chk Check, ok bool) Answer {
	a := p.answer
	if !ok {
		return a
	}
	a.Checked = true

	accepted := chk.Accepts(r.floor)
	switch {
	case accepted:
		r.count(func(s *RunStats) {
			s.CheckAccepted++
			s.AcceptedNew++
		})
	case chk.Rejects(r.floor):
		r.count(func(s *RunStats) {
			s.CheckRejected++
			s.Rejected++
		})
	default:
		r.count(func(s *RunStats) {
			s.CheckUnsure++
		})
	}

	if p.item.Settled {
		if accepted {
			id := p.target
			a.Decision = Decision{
				Ref:        a.Decision.Ref,
				ProductID:  &id,
				Confidence: chk.Confidence,
				Reason:     "confirmed by pair check",
			}
			a.Offered = true
		}
		return a
	}

	switch {
	case accepted:
		a.Decision.Confidence = minFloat(a.Decision.Confidence, chk.Confidence)
	case chk.Rejects(r.floor):
		a.Decision.ProductID = nil
		a.Decision.Reason = "rejected by pair check"
	default:
		a.Decision.ProductID = nil
		a.Decision.Reason = "unconfirmed by pair check"
	}

	return a
}
