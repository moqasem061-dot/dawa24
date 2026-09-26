package matchflow

// Turning a proposal and its pair check into one answer.

import "context"

// pending is one answer waiting on its pair check.
type pending struct {
	answer Answer
	item   Item
	target int64
}

// handle runs one request: propose, check, hand back.
func (r *runner) handle(ctx context.Context, req Request) {
	batch := req.Batch
	batch.Feature = r.spec.Feature
	batch.OrganizationID = r.spec.OrganizationID
	batch.UserID = r.spec.UserID

	decided := r.propose(ctx, batch, 0)

	var checks []pending
	for _, it := range batch.Items {
		d, ok := decided[it.Ref]
		if !ok {
			continue
		}
		key := req.Keys[it.Ref]
		a := Answer{Key: key, Decision: d, Proposed: d.ProductID, Offered: true}
		if d.ProductID != nil {
			_, a.Offered = req.Offered[*d.ProductID]
		}
		if target, ok := r.needsCheck(it, a); ok {
			checks = append(checks, pending{answer: a, item: it, target: target})
			continue
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

// needsCheck decides whether an answer is put to the pair check, and which
// product is checked.
//
// Only answers that would change something are checked. A confirmation of the
// engine's own settled match changes nothing; a "none" on an unsettled row
// changes nothing; a proposal the confidence floor or the identity guard will
// refuse changes nothing. What remains is the answers that would move a row —
// an unsettled row gaining a product, or a settled row being taken off the
// automatic path.
func (r *runner) needsCheck(it Item, a Answer) (int64, bool) {
	if r.verifier == nil {
		return 0, false
	}
	d := a.Decision
	if d.Confidence < r.floor || d.Confidence > 1 {
		return 0, false
	}
	if d.ProductID != nil && !a.Offered {
		return 0, false
	}

	if it.Settled {
		// The model disputes an applied match. Before a person is asked, the
		// engine's own product is put to the pair check: a dispute that the
		// check contradicts was the list talking, not the medicine.
		if it.CurrentGuess == nil || sameID(it.CurrentGuess, d.ProductID) {
			return 0, false
		}
		return *it.CurrentGuess, true
	}

	if d.ProductID == nil {
		return 0, false
	}
	if r.spec.Screen != nil && !r.spec.Screen(a.Key, *d.ProductID) {
		return 0, false
	}
	return *d.ProductID, true
}

// resolve combines a proposal with its pair check.
func (r *runner) resolve(p pending, chk Check, ok bool) Answer {
	a := p.answer
	if !ok {
		// The check did not come back. An unsettled row is not changed on an
		// unverified word; a settled row keeps the dispute, which sends it to
		// a person — the conservative end of both.
		if !p.item.Settled {
			a.Decision = Decision{Ref: a.Decision.Ref, Confidence: 0,
				Reason: "unverified"}
		}
		return a
	}
	a.Checked = true

	accepted := chk.Accepts(r.floor)
	switch {
	case accepted:
		r.count(func(s *RunStats) { s.CheckAccepted++ })
	case chk.Rejects(r.floor):
		r.count(func(s *RunStats) { s.CheckRejected++ })
	default:
		r.count(func(s *RunStats) { s.CheckUnsure++ })
	}

	if p.item.Settled {
		if accepted {
			// The engine's match holds up on its own. Report it as a
			// confirmation, so Verdict keeps it.
			id := p.target
			a.Decision = Decision{Ref: a.Decision.Ref, ProductID: &id,
				Confidence: chk.Confidence, Reason: "confirmed by pair check"}
			a.Offered = true
		}
		return a
	}

	switch {
	case accepted:
		a.Decision.Confidence = minFloat(a.Decision.Confidence, chk.Confidence)
	case chk.Rejects(r.floor):
		// A confident "these are different products" is a real answer and is
		// worth remembering as "none".
		a.Decision = Decision{Ref: a.Decision.Ref, Confidence: chk.Confidence,
			Reason: "pair check: " + firstNonBlank(chk.Reason, "different product")}
	default:
		// Unsure. Not applied, and not remembered — it is asked again next
		// time, when the catalogue may have grown the product.
		a.Decision = Decision{Ref: a.Decision.Ref, Confidence: 0,
			Reason: "pair check unsure"}
	}
	return a
}
