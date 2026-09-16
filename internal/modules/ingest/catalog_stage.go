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

	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// maxOpenQuestions bounds the residue the AI stage will even consider.
//
// It is the request ceiling times the items a request holds: the exact number
// of distinct questions a full run can carry. Retrieval past it could never be
// sent, so running it would be pure CPU spent on a batch that would be dropped
// at the planning step anyway. A file that exceeds it is one the column mapping
// is wrong on, and the vendor is told the ceiling was reached.
var maxOpenQuestions = ceilings.MaxRequestsPerRun * ceilings.MaxItemsPerRequest

// StageImport parses every row, scores it against the shared catalogue, asks
// the model about what is left, and stages the result for the vendor to review.
// Nothing reaches live variants or stocks until they confirm.
func (s *Service) StageImport(ctx context.Context, session *Session) error {
	analysis, err := s.analyse(ctx, session)
	if err != nil {
		return err
	}
	analysis.Complete()

	if err := s.imports.ClearRows(ctx, session.ID); err != nil {
		s.log.WarnContext(ctx, "clear import rows failed", "error", err)
	}

	index, err := s.loadCatalogIndex(ctx)
	if err != nil {
		return err
	}

	book, err := s.openBook(ctx, session)
	if err != nil {
		return err
	}
	defer func() { _ = book.Close() }()

	var variantIdx *variantIndex
	if s.catalog != nil {
		if keys, err := s.catalog.ListVariantKeys(ctx, session.OrganizationID); err == nil {
			var inWarehouse map[int64]bool
			if s.inventory != nil && session.Settings.WarehouseID > 0 {
				inWarehouse, _ = s.inventory.VariantIDsInWarehouse(ctx, session.Settings.WarehouseID)
			}
			variantIdx = newVariantIndex(keys, inWarehouse, session.Settings.BranchID)
		}
	}

	run := &stagingRun{
		svc:        s,
		session:    session,
		index:      index,
		variantIdx: variantIdx,
		// The live mapping, not the stored snapshot: staging runs immediately
		// after the vendor confirms their columns, so this is the freshest
		// statement of what is bound to what.
		match:    stagingMatchOptions(session.Settings, analysis.Mapping.MappedIdentifiers()),
		byNorm:   map[string]*openRow{},
		bucketOf: map[int]matchBucket{},
	}

	opts := productmatch.DefaultProcessOptions()
	opts.Parse = parseOptionsFrom(session.Settings)
	opts.Duplicates = session.Settings.Duplicates
	opts.Vocabulary = s.vocabulary(ctx, session.OrganizationID)

	// Progress is persisted rather than held in memory, so a vendor who closes
	// the tab and comes back sees where the run actually is.
	s.note(ctx, session, 10, i18n.TDefault("w4_mod.s_387_387"))

	result, err := productmatch.Process(book, analysis.Layout, analysis.Mapping, opts,
		func(batch []*productmatch.Row) error { return run.stage(ctx, batch) })
	if err != nil {
		return err
	}

	// The AI stage reports its own progress across 45–95%, per batch, so
	// nothing is published here that would jump backwards over it. What this
	// marks is the deterministic pass finishing, which is the point a vendor
	// with AI switched off sees the run reach.
	s.note(ctx, session, 45, i18n.TDefault("w4_mod.s_388_388"))
	run.enhance(ctx)

	session.Stats = result.Stats
	session.Findings = result.Issues
	session.TotalRows = result.Stats.SheetRows
	session.MatchedRows = run.counts.matched
	session.ReviewRows = run.counts.review
	session.UnmatchedRows = run.counts.unmatched
	session.ErrorRows = run.counts.errors + result.Stats.Rejected
	session.Phase = PhaseReview

	// FinishStaging, not SaveDraft: the session is in 'processing' and SaveDraft
	// refuses that phase on purpose. Writing the outcome through it matched
	// zero rows and wedged every import at 95% with the work already done.
	return s.imports.FinishStaging(ctx, session)
}

// note records how far the run has reached, without letting a failed write stop
// the run. Progress is for the vendor watching; the import is the work.
func (s *Service) note(ctx context.Context, session *Session, percent int, message string) {
	if s.imports == nil || session == nil {
		return
	}
	if err := s.imports.Progress(ctx, session.ID, percent, message); err != nil {
		s.log.WarnContext(ctx, "import progress not recorded",
			"import", session.PublicID, "percent", percent, "error", err)
	}
}

// loadCatalogIndex builds the in-memory shared catalogue both matching passes
// read. It is loaded as the system, because the shared catalogue belongs to no
// tenant.
func (s *Service) loadCatalogIndex(ctx context.Context) (*productmatch.Index, error) {
	products, err := s.catalog.ListMatchProducts(database.AsSystem(ctx))
	if err != nil {
		return nil, fmt.Errorf("load shared catalogue: %w", err)
	}
	master := make([]productmatch.MasterProduct, 0, len(products))
	for _, p := range products {
		master = append(master, productmatch.MasterProduct{
			ID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN, SKU: p.SKU,
			Barcode: p.Barcode, Scientific: p.Scientific, DosageForm: p.DosageForm,
			Concentration: p.Concentration, Unit: p.Unit,
			Manufacturer: p.Manufacturer, PublicPrice: p.PublicPrice,
		})
	}
	return productmatch.NewIndex(master), nil
}

// openBook re-reads the stored upload for the sheet the vendor chose.
func (s *Service) openBook(ctx context.Context, session *Session) (*sheet.Book, error) {
	content, err := s.imports.File(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	book, err := sheet.Open(content, session.Filename)
	if err != nil {
		return nil, apperr.Validation("import.unreadable", err.Error(), nil)
	}
	if session.Source.Sheet != "" {
		_ = book.Use(session.Source.Sheet)
	}
	return book, nil
}

// stagingMatchOptions are the deterministic engine's thresholds for staging.
//
// MinReview sits well below MinStrong on purpose: a row scored between the two
// is not applied, but it IS shown to the vendor with its candidates, and it is
// the residue the AI stage is asked about. Setting them equal would hide the
// rows most likely to be a real match written differently.
func stagingMatchOptions(settings Settings, mapped productmatch.MappedColumns) productmatch.MatchOptions {
	opts := productmatch.DefaultMatchOptions()
	opts.MinStrong = settings.MinMatchScore
	if opts.MinStrong <= 0 {
		opts.MinStrong = productmatch.DefaultMinStrong
	}
	// Never below the shared review floor. Halving the applied threshold used
	// to put it at 0.15, which is where the sixteen-per-cent suggestions came
	// from: a band wide enough to hold every coincidence in the catalogue.
	opts.MinReview = max(productmatch.DefaultMinReview, min(opts.MinStrong*0.7, 0.35))
	return opts.WithIdentifiers(mapped, settings.identifierChoices())
}

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

					if dec != nil && dec.ChosenProductID != nil && *dec.ChosenProductID > 0 {
						if dec.Source == "manual" || dec.Confidence >= 0.80 {
							if p, ok := r.index.Lookup(*dec.ChosenProductID); ok && p != nil {
								reason := "مطابقة معتمدة من ذاكرة القرارات"
								if dec.Source == "manual" {
									reason = "مطابقة يدوية معتمدة من ذاكرة القرارات"
								}
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
