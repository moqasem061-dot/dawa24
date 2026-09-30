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
	"fmt"
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

	actorID := int64(0)
	if r.session.CreatedBy != nil {
		actorID = *r.session.CreatedBy
	}
	rev := r.session.MatchRevision
	if rev <= 0 {
		rev = 1
	}

	var decisions []matchflow.RowDecision
	var matchRunID *int64

	choices := r.session.Settings.identifierChoices()
	identPolicy := productmatch.IdentifierPolicy{
		AllowGTIN:         r.match.TrustBarcode,
		AllowSupplierCode: r.match.TrustSupplierCode,
		AllowCatalogCode:  choices.CodeIsCatalogCode,
		DeclaredNamespaces: make(map[int]string),
	}
	if r.session.Mapping != nil {
		for _, col := range r.session.Mapping.Columns {
			if col.Field == productmatch.FieldSKU {
				identPolicy.DeclaredNamespaces[col.Index] = string(choices.Namespaces[productmatch.FieldSKU])
			} else if col.Field == productmatch.FieldBarcode {
				identPolicy.DeclaredNamespaces[col.Index] = string(choices.Namespaces[productmatch.FieldBarcode])
			}
		}
	}

	if r.svc.matcher != nil {
		reqRows := make([]productmatch.Row, len(batch))
		for i, row := range batch {
			reqRows[i] = *row
		}
		prepReq := productmatch.PrepareRequest{
			Scope: productmatch.Scope{
				EffectiveOrg:    r.session.OrganizationID,
				ActorUserID:     actorID,
				SourceNamespace: fmt.Sprintf("supplier:%d", r.session.OrganizationID),
				Workflow:        productmatch.WorkflowVendor,
				EntryPoint:      "vendor_stage_run",
			},
			ConsumerRef: fmt.Sprintf("ingest.catalog_imports:%d", r.session.ID),
			SourceRev:   rev,
			Rows:        reqRows,
			Required:    productmatch.GranularityProduct,
			Identifiers: &identPolicy,
		}
		prepRes, err := r.svc.matcher.Prepare(ctx, prepReq)
		if err == nil && len(prepRes.Decisions) == len(batch) {
			decisions = prepRes.Decisions
			if prepRes.RunID > 0 {
				id := prepRes.RunID
				matchRunID = &id
			}
		}
	}

	// Fallback to in-process index Decide if matcher is unset or failed
	if len(decisions) != len(batch) {
		pol := productmatch.DefaultPolicy()
		pol.MinStrong = r.match.MinStrong
		pol.MinReview = r.match.MinReview
		pol.Identifiers = identPolicy
		decisions = make([]matchflow.RowDecision, len(batch))
		for i, row := range batch {
			decisions[i] = r.index.Decide(row, pol)
			decisions[i].Revision = rev
		}
	}

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
					if decisions[i].Basis == matchflow.BasisIdentifier {
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

					if dec != nil && dec.Source == "manual" && dec.ChosenProductID != nil && *dec.ChosenProductID > 0 {
						if p, ok := r.index.Lookup(*dec.ChosenProductID); ok && p != nil {
							decisions[i].ProductID = dec.ChosenProductID
							decisions[i].State = matchflow.StateAccepted
							decisions[i].Basis = matchflow.BasisKnowledgeHuman
						}
					}
				}
			}
		}
	}

	for i, row := range batch {
		dec := decisions[i]
		cols := matchflow.Columns(dec)

		productID := cols.MatchedProductID
		matchLevel := productmatch.MatchLevel(cols.MatchLevel)

		score := 0.0
		if cols.MatchedProductID != nil {
			score = 1.0
		}

		cands := make([]productmatch.MatchCandidate, 0, len(dec.Candidates))
		for _, c := range dec.Candidates {
			pName, pSci, pForm, pConc, pMfr := "", "", "", "", ""
			if p, ok := r.index.Lookup(c.ProductID); ok && p != nil {
				pName = p.NameAR
				if pName == "" {
					pName = p.NameEN
				}
				pSci = p.Scientific
				pForm = p.DosageForm
				pConc = p.Concentration
				pMfr = p.Manufacturer
			}
			cands = append(cands, productmatch.MatchCandidate{
				ProductID:     c.ProductID,
				Name:          pName,
				Scientific:    pSci,
				DosageForm:    pForm,
				Concentration: pConc,
				Manufacturer:  pMfr,
				Score:         1.0,
			})
		}

		m := productmatch.MatchResult{
			ProductID:  0,
			Level:      matchLevel,
			Score:      score,
			Candidates: cands,
		}
		if productID != nil {
			m.ProductID = *productID
		}

		bucket := bucketOf(m)
		r.count(bucket, 1)
		r.bucketOf[row.Number] = bucket

		outcome := OutcomeStaged
		if row.HasErrors() {
			outcome = OutcomeError
			r.counts.errors++
		}

		if outcome != OutcomeError {
			r.remember(row, m)
		}

		var variantID *int64
		inWarehouse := false
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

		baselineDec := dec
		staged = append(staged, RowOutcome{
			SourceRow:         row.Number,
			Outcome:           outcome,
			MatchLevel:        cols.MatchLevel,
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
			MatchState:        cols.MatchState,
			MatchBasis:        cols.MatchBasis,
			MatchGranularity:  cols.MatchGranularity,
			MatchRevision:     dec.Revision,
			HumanLocked:       false,
			AIState:           cols.AIState,
			MatchRunID:        matchRunID,
			MatchDecision:     &dec,
			BaselineDecision:  &baselineDec,
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
