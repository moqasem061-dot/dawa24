package ingest

// Committing: writing what the vendor reviewed and accepted.
//
// Nothing here matches anything. Every row arriving at this stage already
// carries the catalogue product it resolved to — deterministically, by the AI
// stage, or by the vendor's own hand on the review screen — and this file's
// only job is to turn that into the vendor's variants and warehouse balances
// in as few statements as possible.
//
// What it does have to get right is the settings. Every one of them is a
// promise the vendor made a decision on two screens ago, and a commit that
// quietly ignores one of them is worse than a commit that refuses: the vendor
// believes their catalogue is in the state they asked for. So each setting is
// applied at exactly one place below, and each place says which setting it is.

import (
	"context"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// commitBatchSize is how many of the vendor's rows are written per round trip.
const commitBatchSize = 250

// commitRun carries the state of one commit.
type commitRun struct {
	svc      *Service
	session  *Session
	settings Settings
	variants *variantIndex

	inserted int
	updated  int
	skipped  int
	errors   int
	// stocked counts the warehouse balances actually written, which is the
	// number a vendor checks when they ask "did the quantities land?".
	stocked int

	// touched are the variants this run wrote, and the seed of the keep-list
	// for the mode that declares the file to be the whole catalogue.
	touched []int64
	// retired is how many of the vendor's other variants that mode took off
	// sale, which is the number the results screen leads with.
	retired  int
	outcomes []RowOutcome
}

// CommitImport writes every reviewed, non-excluded staged row to the vendor's
// catalogue and to the warehouse they chose.
func (s *Service) CommitImport(ctx context.Context, publicID string) (*Session, error) {
	session, err := s.prepareCommit(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if !s.runs.claim(publicID) {
		return nil, apperr.Conflict("import.running", i18n.TDefault("w4_mod.w4str_192_192"))
	}
	defer s.runs.release(publicID)
	return s.commit(ctx, session)
}

// ConfirmImport executes the final commit of reviewed staged rows.
func (s *Service) ConfirmImport(ctx context.Context, publicID string) (*Session, error) {
	return s.CommitImport(ctx, publicID)
}

// prepareCommit loads an import and refuses one that cannot be committed.
func (s *Service) prepareCommit(ctx context.Context, publicID string) (*Session, error) {
	if s.imports == nil || s.catalog == nil {
		return nil, ErrImportStoreUnavailable
	}
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if session.Phase == PhaseProcessing || s.runs.running(publicID) {
		return nil, apperr.Conflict("import.running", i18n.TDefault("w4_mod.w4str_190_190"))
	}
	if session.Settings.WarehouseID <= 0 {
		return nil, apperr.Validation("import.warehouse_required",
			i18n.TDefault("w4_mod.w4str_191_191"), nil)
	}
	return session, nil
}

// commit performs the write. The caller owns the run claim.
func (s *Service) commit(ctx context.Context, session *Session) (*Session, error) {
	settings := session.Settings.Normalize()

	staged, err := s.imports.StagedRowsForCommit(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("load staged rows for commit: %w", err)
	}

	// Rows the vendor never confirmed are not written, and the count of them is
	// carried into the outcome rather than left out of it. A commit that writes
	// four hundred rows out of nine thousand and reports only the four hundred
	// is telling the truth about what it did and nothing about what it did not.
	_, held, err := s.imports.PendingRowIDs(ctx, session.ID, 1)
	if err != nil {
		s.log.WarnContext(ctx, "pending row count unavailable at commit",
			"import", session.PublicID, "error", err)
	}

	run, err := s.newCommitRun(ctx, session, settings)
	if err != nil {
		return nil, err
	}

	for start := 0; start < len(staged); start += commitBatchSize {
		end := min(start+commitBatchSize, len(staged))
		if err := run.writeBatch(ctx, staged[start:end]); err != nil {
			return nil, err
		}
		s.note(ctx, session, commitPercent(end, len(staged)),
			fmt.Sprintf(i18n.TDefault("w4_mod.d_384"), end))
	}

	retirement := run.retireAbsent(ctx)

	if len(run.outcomes) > 0 {
		if err := s.imports.UpdateCommittedRows(ctx, session.ID, run.outcomes); err != nil {
			s.log.WarnContext(ctx, "commit row ledger not recorded",
				"import", session.PublicID, "error", err)
		}
	}

	if s.memory != nil && len(staged) > 0 {
		ctxWithTenant := database.WithTenant(ctx, session.OrganizationID)
		decisions := make([]CachedDecision, 0, len(staged))
		for _, row := range staged {
			if row.ProductID != nil && *row.ProductID > 0 && (row.IsManuallyMatched || row.MatchLevel == "exact" || row.MatchLevel == "strong") {
				norm := strings.ToLower(strings.TrimSpace(row.DisplayName))
				if norm != "" {
					decisions = append(decisions, CachedDecision{
						Key:             "manual:" + norm,
						NormName:        norm,
						ChosenProductID: row.ProductID,
						Confidence:      1.0,
						Reason:          "مطابقة مؤكدة عند اعتماد الاستيراد",
						PromptVersion:   "manual:v1",
						Scope:           "org",
						Source:          "manual",
					})
				}
			}
		}
		if len(decisions) > 0 {
			_ = s.memory.SaveDecisions(ctxWithTenant, decisions)
		}
	}

	session.InsertedRows = run.inserted
	session.UpdatedRows = run.updated
	session.SkippedRows = run.skipped + held
	session.ErrorRows = run.errors
	session.ErrorMessage = commitNotice(run, retirement)
	session.Phase = PhaseCompleted
	if err := s.imports.Finish(ctx, session); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "vendor catalogue import committed",
		"import", session.PublicID, "inserted", run.inserted, "updated", run.updated,
		"skipped", session.SkippedRows, "errors", run.errors, "balances", run.stocked)
	return session, nil
}

// newCommitRun loads the one index a commit needs: the vendor's own variants,
// told which warehouse and branch this import is writing to.
func (s *Service) newCommitRun(
	ctx context.Context, session *Session, settings Settings,
) (*commitRun, error) {
	keys, err := s.catalog.ListVariantKeys(ctx, session.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("load existing variants: %w", err)
	}
	var inWarehouse map[int64]bool
	if s.inventory != nil && settings.WarehouseID > 0 {
		inWarehouse, err = s.inventory.VariantIDsInWarehouse(ctx, settings.WarehouseID)
		if err != nil {
			// Not fatal. Without it the index falls back to refusing an
			// ambiguous product rather than choosing wrongly, which is the
			// behaviour that existed before this lookup did.
			s.log.WarnContext(ctx, "warehouse variant set unavailable",
				"import", session.PublicID, "warehouse", settings.WarehouseID, "error", err)
			inWarehouse = nil
		}
	}
	return &commitRun{
		svc:      s,
		session:  session,
		settings: settings,
		variants: newVariantIndex(keys, inWarehouse, settings.BranchID),
	}, nil
}

// plannedRow is one staged row that survived the import mode, paired with the
// variant it will be written as.
type plannedRow struct {
	row     *RowOutcome
	variant *catalog.ProductVariant
	// existingID is the vendor's variant this row resolved onto before the
	// write, and zero for a row that will create one.
	existingID int64
}

// writeBatch decides, writes and records one batch of staged rows.
func (c *commitRun) writeBatch(ctx context.Context, chunk []*RowOutcome) error {
	planned := c.plan(chunk)

	// planned is indexed by the row's position in the chunk and holds nil where
	// the import mode declined the row, so that a reference surviving the round
	// trip still points at the row it came from.
	writes := make([]catalog.VariantWriteRow, 0, len(planned))
	for ref, p := range planned {
		if p == nil {
			continue
		}
		writes = append(writes, catalog.VariantWriteRow{Ref: ref, Variant: p.variant})
	}
	if len(writes) == 0 {
		return nil
	}

	result, err := c.svc.catalog.BulkWriteVariants(ctx, c.session.OrganizationID, writes)
	if err != nil {
		return fmt.Errorf("bulk write variants: %w", err)
	}

	stocks, stockRefs := c.applyVariantResult(planned, result)
	c.writeStocks(ctx, stocks, stockRefs)
	return nil
}

// plan resolves each row onto a variant and applies the import mode, recording
// the rows it declines. The result is indexed by the row's position in the
// chunk, which is the reference carried through the write and back.
func (c *commitRun) plan(chunk []*RowOutcome) []*plannedRow {
	planned := make([]*plannedRow, len(chunk))
	for ref, sr := range chunk {
		planned[ref] = c.decide(sr)
	}
	return planned
}

// applyVariantResult reads the write's outcome onto the ledger and builds the
// balances for everything that landed.
//
// It walks the plan in the chunk's own order rather than the result map's. Two
// runs of the same file must produce the same ledger, and a map's iteration
// order is deliberately not the same twice.
func (c *commitRun) applyVariantResult(
	planned []*plannedRow, result catalog.VariantWriteResult,
) ([]inventory.StockWriteRow, map[int]*RowOutcome) {
	failed := make(map[int]string, len(result.Failures))
	for _, failure := range result.Failures {
		failed[failure.Ref] = failure.Message
	}

	stocks := make([]inventory.StockWriteRow, 0, len(planned))
	stockRefs := make(map[int]*RowOutcome, len(planned))

	for ref, p := range planned {
		if p == nil {
			continue
		}
		// The variant we were trying to write is recorded even on failure. A
		// ledger row that says "error" with no variant id makes a failed update
		// indistinguishable from a failed insert, which is exactly how nine
		// hundred refused updates were read as nine hundred refused inserts.
		if message, refused := failed[ref]; refused {
			c.errors++
			c.record(p.row, OutcomeError, nullableVariant(p.existingID), message)
			continue
		}
		id := result.IDs[ref]
		if id <= 0 {
			c.errors++
			c.record(p.row, OutcomeError, nullableVariant(p.existingID),
				i18n.TDefault("w4_mod.w4str_125_125"))
			continue
		}

		// The port says whether it created a row, because it is the only party
		// that knows: a row sent as an update whose variant has since been
		// deleted is inserted instead. Falling back on what we asked for keeps
		// a port that reports nothing from mislabelling every insert.
		wasInDestination := p.existingID > 0 && (c.settings.WarehouseID <= 0 || c.variants.initialInWarehouse[p.existingID])
		if result.InsertedRefs[ref] || !wasInDestination {
			c.inserted++
			c.record(p.row, OutcomeInserted, &id, i18n.TDefault("w4_mod.s_380_380"))
		} else {
			c.updated++
			c.record(p.row, OutcomeUpdated, &id, i18n.TDefault("w4_mod.s_380_380"))
		}

		c.variants.remember(p.row.Payload, productOf(p.row), id)
		c.touched = append(c.touched, id)

		if stock := c.stockFor(p.row, id); stock != nil {
			stock.Ref = ref
			stocks = append(stocks, *stock)
			stockRefs[ref] = p.row
		}
	}
	return stocks, stockRefs
}

// writeStocks writes one batch of balances and reports each failure against the
// row that caused it.
func (c *commitRun) writeStocks(
	ctx context.Context, rows []inventory.StockWriteRow, byRef map[int]*RowOutcome,
) {
	if len(rows) == 0 {
		return
	}
	result, err := c.svc.inventory.BulkWriteStocks(ctx, c.settings.StockMode, rows)
	if err != nil {
		c.svc.log.ErrorContext(ctx, "bulk write stocks failed",
			"import", c.session.PublicID, "error", err)
		for _, row := range rows {
			if sr, ok := byRef[row.Ref]; ok {
				c.amend(sr, i18n.TDefault("w4_mod.w4str_214_214")+
					i18n.TDefault("w4_mod.w4str_226_226"))
			}
		}
		return
	}
	c.stocked += result.Written
	for _, failure := range result.Failures {
		if sr, ok := byRef[failure.Ref]; ok {
			c.amend(sr, i18n.TDefault("w4_mod.w4str_214_214")+failure.Message)
		}
	}
}

