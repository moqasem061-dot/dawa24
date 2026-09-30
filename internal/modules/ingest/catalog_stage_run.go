package ingest

// Staging: reading the file, matching every row, and asking the model about the
// rows the matcher could not settle — without writing anything the vendor has
// not yet seen.
//
// The order of the two matching passes is the whole design. The deterministic
// engine runs while the rows stream past, because it is pure arithmetic and
// there is no reason to hold nine thousand rows in memory to do it. The AI
// stage runs ONCE, afterwards, over the residue the engine left — because
// everything that makes it cheap needs to see the whole file at once:
// duplicate rows collapse onto one question, and overlapping shortlists collapse
// into one shared catalogue window. Running it inside the streaming callback,
// as this used to, threw both savings away five hundred rows at a time.

import (
	"context"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// stagingRun carries the state of one staging pass.
type stagingRun struct {
	svc        *Service
	session    *Session
	index      *productmatch.Index
	variantIdx *variantIndex
	match      productmatch.MatchOptions

	counts counters

	// byNorm collects the residue as QUESTIONS rather than rows: every row with
	// the same normalised name asks the same thing, so they share one entry and
	// one retrieval. On a supplier file that repeats a product per warehouse
	// this is where most of the saving comes from, and it costs one map.
	byNorm map[string]*openRow
	open   []*openRow
	// bucketOf remembers which counter each open row was first tallied under,
	// so an AI match can move it without the review screen disagreeing with
	// itself.
	bucketOf map[int]matchBucket
	// ceilingHit records that the residue outgrew what one run may ask about.
	ceilingHit bool
}

// stage matches one batch of parsed rows and writes them to the staging table.
func (r *stagingRun) stage(ctx context.Context, batch []*productmatch.Row) error {
	staged := make([]RowOutcome, 0, len(batch))

	// The whole batch is scored at once, across every core.
	//
	// Matching is pure CPU over a read-only index, so it divides perfectly, and
	// on the twenty-five-thousand-row price lists this importer exists for the
	// difference is most of the wall clock: measured on a twelve-core machine,
	// twelve thousand rows a second against eight thousand.
	results := productmatch.MatchAll(r.index, batch, r.match, 0)

	if r.svc.memory != nil && len(batch) > 0 {
		lookupKeys := make([]string, 0, len(batch)*4)
		for _, row := range batch {
			name := row.DisplayName()
			if name == "" {
				continue
			}
			norm := productmatch.NormalizeText(productmatch.StripTradeAnnotations(name))
			rawNorm := strings.ToLower(strings.TrimSpace(name))
			if norm != "" {
				lookupKeys = append(lookupKeys, norm, "manual:"+norm)
			}
			if rawNorm != "" && rawNorm != norm {
				lookupKeys = append(lookupKeys, rawNorm, "manual:"+rawNorm)
			}
		}
		if len(lookupKeys) > 0 {
			lookupCtx := database.WithTenant(ctx, r.session.OrganizationID)
			if decs, err := r.svc.memory.LookupDecisions(lookupCtx, lookupKeys); err == nil && len(decs) > 0 {
				for i, row := range batch {
					if results[i].Level == productmatch.MatchBarcode {
						continue
					}
					name := row.DisplayName()
					norm := productmatch.NormalizeText(productmatch.StripTradeAnnotations(name))
					rawNorm := strings.ToLower(strings.TrimSpace(name))
					var dec *CachedDecision
					if d, ok := decs["manual:"+norm]; ok {
						dec = &d
					} else if d, ok := decs[norm]; ok {
						dec = &d
					} else if d, ok := decs["manual:"+rawNorm]; ok {
						dec = &d
					} else if d, ok := decs[rawNorm]; ok {
						dec = &d
					}

					// Only a person's confirmed mapping is honoured by name. An AI
					// answer is reused only through its versioned question key, in
					// the AI stage, where the identity guard re-checks it; applied
					// here by name it was an unverified guess — from any prompt
					// version, against any shortlist — promoted to an exact match.
					if dec != nil && dec.Source == "manual" && dec.ChosenProductID != nil && *dec.ChosenProductID > 0 {
						{
							if p, ok := r.index.Lookup(*dec.ChosenProductID); ok && p != nil {
								reason := "مطابقة يدوية معتمدة من ذاكرة القرارات"
								pName := p.NameAR
								if pName == "" {
									pName = p.NameEN
								}
								results[i] = productmatch.MatchResult{
									ProductID: p.ID,
									Level:     productmatch.MatchExact,
									Score:     1.0,
									Reason:    reason,
									Candidates: []productmatch.MatchCandidate{
										{
											ProductID:     p.ID,
											Name:          pName,
											Scientific:    p.Scientific,
											DosageForm:    p.DosageForm,
											Concentration: p.Concentration,
											Manufacturer:  p.Manufacturer,
											Score:         1.0,
											Reason:        reason,
										},
									},
								}
							}
						}
					}
				}
			}
		}
	}

	for i, row := range batch {
		m := results[i]
		bucket := bucketOf(m)
		r.count(bucket, 1)
		r.bucketOf[row.Number] = bucket

		outcome := OutcomeStaged
		if row.HasErrors() {
			outcome = OutcomeError
			r.counts.errors++
		}

		// A row the reader rejected is not worth asking about: it will not be
		// committed whatever the answer, and paying for it is pure waste.
		//
		// Everything else is remembered, settled or not. A settled row is asked
		// a different question — "is this right?" rather than "what is this?" —
		// and the planner spends the run's budget on the uncertain rows first,
		// so putting the whole file in front of the model costs the confident
		// rows only what is left over.
		if outcome != OutcomeError {
			r.remember(row, m)
		}

		var productID *int64
		var variantID *int64
		inWarehouse := false
		if m.ProductID > 0 {
			id := m.ProductID
			productID = &id
		}
		if r.variantIdx != nil {
			pID := int64(0)
			if productID != nil {
				pID = *productID
			}
			if vID, _ := r.variantIdx.resolve(row, pID, nil); vID > 0 {
				variantID = &vID
				inWarehouse = r.variantIdx.inWarehouse != nil && r.variantIdx.inWarehouse[vID]
			}
		}

		staged = append(staged, RowOutcome{
			SourceRow:         row.Number,
			Outcome:           outcome,
			MatchLevel:        string(m.Level),
			MatchScore:        m.Score,
			ProductID:         productID,
			VariantID:         variantID,
			InWarehouse:       inWarehouse,
			DisplayName:       row.Name,
			SourceCode:        row.SKU,
			CustomVariantName: row.Name,
			Payload:           row,
			Candidates:        m.Candidates,
			Issues:            row.Issues,
			Message:           m.Reason,
			AIState:           string(matchflow.AIStateOff),
		})
	}

	return r.svc.imports.AppendRows(ctx, r.session.ID, r.session.OrganizationID, staged)
}

// remember files an unsettled row under the question it asks.
//
// The first row to ask a question owns it; later rows carrying the same
// normalised name attach their row number and nothing else. The parsed row of
// the first is the one the identity guard re-reads, which is correct because
// the guard reads the name and the strength — the very things that made these
// rows one question.
func (r *stagingRun) remember(row *productmatch.Row, m productmatch.MatchResult) {
	if len(r.byNorm) >= maxOpenQuestions {
		r.ceilingHit = true
		return
	}
	key := productmatch.NormalizeText(row.DisplayName())
	if key == "" {
		return
	}
	if known, ok := r.byNorm[key]; ok {
		known.sourceRows = append(known.sourceRows, row.Number)
		return
	}
	q := &openRow{
		row:        row,
		sourceRows: []int{row.Number},
		normName:   key,
		score:      m.Score,
		settled:    m.Level.Settled() && verifiable(m.Level),
		ambiguous:  m.Level == productmatch.MatchAmbiguous,
	}
	if m.ProductID > 0 {
		id := m.ProductID
		q.guess = &id
	}
	r.byNorm[key] = q
	r.open = append(r.open, q)
}
