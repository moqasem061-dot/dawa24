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

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
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
	session.MatchRevision++
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
		if keys, err := s.catalog.ListVariantKeys(ctx, session.OrganizationID); err == nil { // nolint:errswallow: the else branch logs it
			var inWarehouse map[int64]bool
			if s.inventory != nil && session.Settings.WarehouseID > 0 {
				set, werr := s.inventory.VariantIDsInWarehouse(ctx, session.Settings.WarehouseID)
				if werr != nil {
					s.log.WarnContext(ctx, "warehouse variant set unavailable for staging", "import", session.PublicID, "error", werr)
				}
				inWarehouse = set
			}
			variantIdx = newVariantIndex(keys, inWarehouse, session.Settings.BranchID)
		} else {
			s.log.WarnContext(ctx, "variant keys unavailable; staging without variant matching", "import", session.PublicID, "error", err)
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
