package ingest

// What the commit is about to do, worked out before it does it.
//
// The review screen used to describe the import mode in prose and then print
// the MATCHED count beside it, whatever the mode was. On "add new items only"
// that number is wrong for every product the vendor already stocks; on "update
// existing only" it is wrong for every product they do not; and on "treat this
// file as my whole catalogue" it said nothing at all about the part the vendor
// actually needs to know, which is how many of their existing items are about
// to come off sale.
//
// So the plan is computed rather than described. It runs the commit's own
// decide() over the same staged rows, against the same variant index, and
// counts what comes out — which means the screen cannot disagree with the run,
// because it IS the run, minus the writing.

import (
	"context"
	"fmt"
)

// CommitPlan is what committing this import would do to the vendor's catalogue.
type CommitPlan struct {
	// Insert and Update are the rows that would be written, split by whether
	// the vendor already has a variant for them.
	Insert int
	Update int
	// DuplicatesInFile is the count of repeated rows within this file that refer
	// to a product already mentioned in an earlier row of the file.
	DuplicatesInFile int
	// SkippedByMode is the rows the import mode declines even though they are
	// matched and confirmed — the existing items under "add new only", the new
	// ones under "update existing only".
	SkippedByMode int
	// Held is the included rows the vendor has not confirmed, which are never
	// written whatever the mode.
	Held int
	// Retire is how many of the vendor's own variants the replace mode would
	// take off sale, being the ones this file does not mention. Zero in every
	// other mode.
	Retire int
	// Stocked is how many warehouse balances the run would write.
	Stocked int
}

// Writes is the total number of the vendor's variants the run would touch.
func (p CommitPlan) Writes() int { return p.Insert + p.Update + p.DuplicatesInFile }

// PreviewCommit reports what committing this import would do, without doing it.
func (s *Service) PreviewCommit(ctx context.Context, publicID string) (CommitPlan, error) {
	var plan CommitPlan
	if s.imports == nil || s.catalog == nil {
		return plan, ErrImportStoreUnavailable
	}
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return plan, err
	}
	return s.previewFor(ctx, session)
}

// previewFor is the preview for a session already loaded, which is the form the
// review screen needs: it has the session in hand and must not load it twice.
func (s *Service) previewFor(ctx context.Context, session *Session) (CommitPlan, error) {
	var plan CommitPlan
	settings := session.Settings.Normalize()

	staged, err := s.imports.StagedRowsForCommit(ctx, session.ID)
	if err != nil {
		return plan, fmt.Errorf("preview staged rows: %w", err)
	}
	if _, held, err := s.imports.PendingRowIDs(ctx, session.ID, 1); err == nil {
		plan.Held = held
	}

	run, err := s.newCommitRun(ctx, session, settings)
	if err != nil {
		return plan, err
	}

	// Ids for the variants the plan says would be created. They have to look
	// real — the index refuses a non-positive one — and sit above every id the
	// vendor actually holds, so a later row can never resolve onto an existing
	// variant by colliding with one.
	nextPredicted := int64(1)
	for id := range run.variants.live {
		if id >= nextPredicted {
			nextPredicted = id + 1
		}
	}
	predicted := make(map[int64]bool, 16)

	// decide() is the commit's, and it mutates the run's counters and its
	// variant index exactly as a real commit would — including remembering a
	// variant a first row would create, so a file mentioning one product twice
	// is planned as one insert and one update rather than two inserts.
	touchedExisting := make(map[int64]bool, 16)
	for _, row := range staged {
		planned := run.decide(row)
		if planned == nil {
			continue
		}
		switch {
		case predicted[planned.existingID]:
			// A second row about a product a previous row in this file would create.
			// It is an in-file duplicate row.
			plan.DuplicatesInFile++
		case planned.existingID > 0:
			if touchedExisting[planned.existingID] {
				// A repeated row in this file for an existing catalog product.
				plan.DuplicatesInFile++
			} else if run.settings.WarehouseID <= 0 || run.variants.initialInWarehouse[planned.existingID] {
				// Genuine update in destination.
				touchedExisting[planned.existingID] = true
				plan.Update++
				run.touched = append(run.touched, planned.existingID)
			} else {
				// It exists in catalog, but is being introduced to this warehouse for the first time.
				touchedExisting[planned.existingID] = true
				plan.Insert++
				run.touched = append(run.touched, planned.existingID)
				run.variants.inWarehouse[planned.existingID] = true
			}
		default:
			plan.Insert++
			if planned.existingID > 0 {
				run.touched = append(run.touched, planned.existingID)
				run.variants.inWarehouse[planned.existingID] = true
			} else {
				predicted[nextPredicted] = true
				run.variants.remember(row.Payload, productOf(row), nextPredicted)
				nextPredicted++
			}
		}
		var stockVariantID int64
		if planned.existingID > 0 {
			stockVariantID = planned.existingID
		} else {
			stockVariantID = nextPredicted
		}
		if run.stockFor(row, stockVariantID) != nil {
			plan.Stocked++
		}
	}
	plan.SkippedByMode = run.skipped

	if settings.Mode == ModeReplace {
		plan.Retire = s.previewRetirement(ctx, run, predicted)
	}
	return plan, nil
}

// previewRetirement counts the vendor's variants the replace mode would take
// off sale, using the same keep-list the commit builds.
func (s *Service) previewRetirement(
	ctx context.Context, run *commitRun, predicted map[int64]bool,
) int {
	if len(run.touched) == 0 && len(predicted) == 0 {
		// The commit refuses to retire a catalogue on the strength of a run
		// that writes nothing, so the plan must not promise one.
		return 0
	}
	keep := make(map[int64]bool, len(run.touched))
	for _, id := range run.keepList(ctx) {
		keep[id] = true
	}
	if len(keep) == 0 {
		// keepList returns nothing when it could not read what the file
		// mentions, and the commit refuses to retire in that case too.
		return 0
	}
	retire := 0
	for id := range run.variants.active {
		// When scoped to a specific warehouse, only variants associated with that warehouse are counted.
		if run.settings.WarehouseID > 0 {
			if run.variants.initialInWarehouse == nil || !run.variants.initialInWarehouse[id] {
				continue
			}
		}
		if !keep[id] && !predicted[id] {
			retire++
		}
	}
	return retire
}
