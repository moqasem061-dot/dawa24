package matchflow

// Packing a run into requests.
//
// The whole cost argument for the AI stage lives here. Each row retrieves about
// a dozen catalogue products, and across a few hundred rows those sets overlap
// heavily — the same twenty antihypertensives come back for every
// antihypertensive line. Sending each row with its own copy of its candidates
// repeats that overlap once per row; sending ONE window that every row
// references by id does not.
//
// Measured on a live 1,473-row file whose deterministic pass left 1,123 rows
// unresolved: 13,453 candidate references collapse to 10,294 catalogue rows,
// and the file fits in eight requests instead of the fifteen the same content
// would need one shortlist per row.
//
// This was written three times — once in the smart order, once in the saving
// list, once in the master-catalogue import — and the three had drifted. Only
// one of them enforced the byte budget; only one collapsed duplicate questions;
// the saving list built ONE window for the whole file and then sent that entire
// window with every request, so a long list paid for its whole catalogue window
// thirty times over. On a twenty-five-thousand-row price list that is not a
// slow import, it is a request too large to answer.

import "sort"

// Question is one row put to the model, with everything needed to send it and
// to judge the answer.
type Question struct {
	// Key identifies the question for the decision cache. Rows with the same
	// key are one question however many rows asked it.
	Key string
	// Item is the row as the model sees it. Ref is assigned by Plan.
	Item Item
	// Window is the catalogue products this row may be answered with, best
	// first. Plan de-duplicates them into the request's shared block.
	Window []CatalogEntry
	// Risk orders what is asked first when the budget cannot cover everything.
	// See Plan.
	Risk float64
}

// Request is one call: a de-duplicated catalogue window, the items to resolve
// against it, and the question keys those items came from.
type Request struct {
	Batch Batch
	// Keys maps a request-local ref back to the question key, so an answer can
	// be tied to every row that asked it.
	Keys map[int]string
	// Offered is every product id the model was shown, which is the set an
	// answer must come from.
	Offered map[int64]struct{}
}

// Plan groups questions into as few requests as the budget allows.
//
// Two orderings are at work and both matter.
//
// Questions are taken in order of RISK, so that when the ceiling cuts the run
// short, what it cuts is the safest tail. That is what makes it affordable to
// put a whole file in front of the model rather than only its residue: a
// twenty-five-thousand-row price list is mostly rows the engine settled exactly,
// at a score whose measured accuracy is better than 99.9%, and asking about
// those is nearly pure waste. Asking about the ambiguous ones is where the
// errors are.
//
// Within a batch, items are grouped by name, because alphabetically adjacent
// lines retrieve overlapping products and overlap is exactly what the shared
// window converts into savings.
func Plan(questions []Question, c Ceilings) ([]Request, bool) {
	collapsed := collapse(questions)
	sort.SliceStable(collapsed, func(i, j int) bool {
		if collapsed[i].Risk != collapsed[j].Risk {
			return collapsed[i].Risk > collapsed[j].Risk
		}
		return collapsed[i].Key < collapsed[j].Key
	})

	var (
		out     []Request
		cur     = newRequest()
		size    int
		ceiling bool
	)
	flush := func() {
		if len(cur.Batch.Items) == 0 {
			return
		}
		sort.SliceStable(cur.Batch.Catalog, func(i, j int) bool {
			return cur.Batch.Catalog[i].ProductID < cur.Batch.Catalog[j].ProductID
		})
		out = append(out, cur)
		cur = newRequest()
		size = 0
	}

	for _, q := range collapsed {
		if len(out) >= c.MaxRequestsPerRun {
			ceiling = true
			break
		}
		cost := questionCost(q, cur.Offered)
		if len(cur.Batch.Items) > 0 &&
			(size+cost > c.MaxInputBytes || len(cur.Batch.Items) >= c.MaxItemsPerRequest) {
			flush()
			if len(out) >= c.MaxRequestsPerRun {
				ceiling = true
				break
			}
			cost = questionCost(q, cur.Offered)
		}

		ref := len(cur.Batch.Items) + 1
		item := q.Item
		item.Ref = ref
		item.Options = item.Options[:0:0]
		for _, e := range q.Window {
			item.Options = append(item.Options, e.ProductID)
			if _, seen := cur.Offered[e.ProductID]; seen {
				continue
			}
			cur.Offered[e.ProductID] = struct{}{}
			cur.Batch.Catalog = append(cur.Batch.Catalog, e)
		}
		cur.Batch.Items = append(cur.Batch.Items, item)
		cur.Keys[ref] = q.Key
		size += cost
	}
	flush()
	return out, ceiling
}

func newRequest() Request {
	return Request{
		Keys:    make(map[int]string),
		Offered: make(map[int64]struct{}),
	}
}

// collapse folds questions that ask exactly the same thing into one.
//
// Identical text against an identical shortlist has an identical answer, and
// paying for it twice is pure waste. The decision cache would catch it on the
// NEXT import; this catches it on the current one, which is what a
// twenty-five-thousand-row price list needs — those files repeat a product
// across warehouses and batch numbers, and the distinct-question count is
// routinely a third of the row count.
//
// The kept copy is the riskiest of the group, so a question asked once for a
// settled row and once for an ambiguous one is asked at the ambiguous row's
// priority.
func collapse(questions []Question) []Question {
	at := make(map[string]int, len(questions))
	out := make([]Question, 0, len(questions))
	for _, q := range questions {
		if i, seen := at[q.Key]; seen {
			if q.Risk > out[i].Risk {
				out[i] = q
			}
			continue
		}
		at[q.Key] = len(out)
		out = append(out, q)
	}
	return out
}

// questionCost estimates the bytes one question adds to a request: its own item
// row, plus a catalogue row for each candidate the window does not already hold.
//
// An estimate rather than a rendering, because rendering every candidate set to
// measure it would double the work of planning. It errs high, which is the safe
// direction: a request slightly under budget costs nothing, one over it fails.
func questionCost(q Question, offered map[int64]struct{}) int {
	cost := len(q.Item.Text) + len(q.Item.Brand) + len(q.Item.Manufacturer) + 96 +
		len(q.Window)*8
	for _, e := range q.Window {
		if _, seen := offered[e.ProductID]; seen {
			continue
		}
		cost += len(e.NameAR) + len(e.NameEN) + len(e.Scientific) +
			len(e.Manufacturer) + len(e.DosageForm) + len(e.Concentration) + 56
	}
	return cost
}

// Risk grades how much is gained by asking about one row.
//
// It is the ordering key Plan spends the budget by, and it encodes what the
// labelled benchmarks measured. A row the engine settled at 0.99 with nothing
// contradicting it is right better than 999 times in 1,000; a row it could not
// separate from a sibling is right about half the time. Both are worth asking
// about — a whole file is put to the model, which is the point — but they are
// not worth asking about in the same order, and a ceiling that cuts the run
// short should cut the first kind.
//
// settled says the engine applied its answer; ambiguous says it found two it
// could not choose between; score is what it reported.
func Risk(settled, ambiguous bool, score float64) float64 {
	switch {
	case ambiguous:
		// Two candidates and no way to choose. Nothing in the file is more
		// worth a second opinion than this.
		return 1.0
	case !settled:
		// Unresolved or below the applied threshold: the lower the score, the
		// less the engine had to say, and the more room there is to improve on
		// it. Bounded below the ambiguous band so an ambiguity is always asked
		// about first.
		return 0.90 - 0.30*clampUnit(score)
	default:
		// Settled. Verification, and the confident end of it goes last.
		return 0.55 * (1 - clampUnit(score))
	}
}

func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
