package ingest

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// retireAbsent takes off sale every variant the file does not mention, for the
// mode that declares the file to be the whole catalogue. It returns what the
// vendor should be told about it.
func (c *commitRun) retireAbsent(ctx context.Context) string {
	if c.settings.Mode != ModeReplace {
		return ""
	}
	if len(c.touched) == 0 {
		return i18n.TDefault("ingest.commit.retire_skipped_empty")
	}

	retired, err := c.svc.catalog.RetireVariantsExcept(
		ctx, c.session.OrganizationID, c.settings.WarehouseID, c.keepList(ctx))
	if err != nil {
		c.svc.log.WarnContext(ctx, "replace-mode variant retirement failed",
			"import", c.session.PublicID, "error", err)
		return i18n.TDefault("ingest.commit.retire_failed")
	}
	c.retired = len(retired)
	if c.retired == 0 {
		return ""
	}
	c.svc.log.InfoContext(ctx, "replace-mode variants retired",
		"import", c.session.PublicID, "retired", c.retired)
	return fmt.Sprintf(i18n.TDefault("ingest.commit.retired_format"), c.retired)
}

// keepList is every variant that survives a replace: strictly the ones this run wrote.
// Everything not written by this file is retired and deleted.
func (c *commitRun) keepList(ctx context.Context) []int64 {
	keep := make(map[int64]struct{}, len(c.touched))
	for _, id := range c.touched {
		if id > 0 {
			keep[id] = struct{}{}
		}
	}
	out := make([]int64, 0, len(keep))
	for id := range keep {
		out = append(out, id)
	}
	return out
}

// zeroBalances empties the retired variants' stock in the warehouse this import
// wrote to.
func (c *commitRun) zeroBalances(ctx context.Context, retired []catalog.RetiredVariant) {
	if c.svc.inventory == nil || c.settings.WarehouseID <= 0 {
		return
	}
	rows := make([]inventory.StockWriteRow, 0, len(retired))
	for _, v := range retired {
		if !c.variants.inWarehouse[v.ID] {
			continue
		}
		rows = append(rows, inventory.StockWriteRow{
			HasQuantity: true,
			Stock: &inventory.Stock{
				OrganizationID:   c.session.OrganizationID,
				WarehouseID:      c.settings.WarehouseID,
				ProductID:        v.ProductID,
				ProductVariantID: v.ID,
				Quantity:         0,
			},
		})
	}
	if len(rows) == 0 {
		return
	}
	if _, err := c.svc.inventory.BulkWriteStocks(ctx, inventory.StockReplace, rows); err != nil {
		c.svc.log.WarnContext(ctx, "replace-mode balance clearing failed",
			"import", c.session.PublicID, "variants", len(rows), "error", err)
	}
}

// record appends one row's outcome to the ledger the results screen reads.
func (c *commitRun) record(sr *RowOutcome, outcome string, variantID *int64, message string) {
	if !c.settings.RecordRows {
		return
	}
	c.outcomes = append(c.outcomes, RowOutcome{
		ID:        sr.ID,
		Outcome:   outcome,
		VariantID: variantID,
		Message:   message,
	})
}

// amend adds a note to the last outcome recorded for a row, which is the one
// its variant write produced.
func (c *commitRun) amend(sr *RowOutcome, note string) {
	for i := len(c.outcomes) - 1; i >= 0; i-- {
		if c.outcomes[i].ID == sr.ID {
			c.outcomes[i].Message = appendMessage(c.outcomes[i].Message, note)
			return
		}
	}
}

// commitNotice is the one sentence the results screen leads with when something
// about the run needs saying.
func commitNotice(run *commitRun, retirement string) string {
	if run.errors > 0 {
		return appendMessage(
			fmt.Sprintf(i18n.TDefault("ingest.commit.row_errors_format"), run.errors), retirement)
	}
	return retirement
}

// commitPercent maps rows written onto the progress bar, leaving the last few
// per cent for the retirement pass and the ledger write.
func commitPercent(done, total int) int {
	if total <= 0 {
		return 95
	}
	return min(5+done*90/total, 95)
}

// stockThreshold prefers the file's alert level and falls back to the vendor's
// default for this import.
func stockThreshold(row, fallback int) int {
	if row > 0 {
		return row
	}
	if fallback > 0 {
		return fallback
	}
	return 0
}

// productOf reads the catalogue product a staged row resolved to.
func productOf(sr *RowOutcome) int64 {
	if sr == nil || sr.ProductID == nil {
		return 0
	}
	return *sr.ProductID
}

// nullableVariant renders a variant id for the ledger, or nil for a row that
// never had one.
func nullableVariant(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}
