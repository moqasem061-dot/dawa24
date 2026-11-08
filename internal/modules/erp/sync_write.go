package erp

import (
	"context"
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// writeResolved is plan S2 step 7 (g) and (h): one catalog Write for every
// created/updated row, then the quota map, then one stock Replace, then the
// links, item results, reviews and matcher knowledge in one repository call.
// A failure isolated to one row fails only that row (ERP_ITEM_WRITE_FAILED);
// a failure of the batch transport is an error so the chunk can be retried.
func (s *Service) writeResolved(ctx context.Context, cc chunkCtx, plan *chunkPlan) error {
	// ---- catalog writes -------------------------------------------------
	variantWrites := make([]VariantWrite, 0, len(plan.rows))
	ref := 0
	for i := range plan.rows {
		row := &plan.rows[i]
		if !row.wantWrite {
			continue
		}
		name := map[string]string{"ar": row.item.ArName, "en": row.item.ArName}
		discount := row.item.PublicPrice
		if row.item.Discount != nil {
			discount = *row.item.Discount
		}
		variantWrites = append(variantWrites, VariantWrite{
			Ref:         ref,
			VariantID:   row.variantID,
			ProductID:   row.productID,
			BranchID:    cc.BranchID,
			Name:        name,
			SKU:         row.sku,
			Barcode:     row.item.Barcode,
			Unit:        itemUnit(row.item),
			BatchNumber: row.item.Batch,
			Price:       row.item.PublicPrice,
			Discount:    discount,
			ExpiryDate:  row.item.Expiry,
		})
		row.writerRef = ref
		ref++
	}
	if len(variantWrites) > 0 {
		writeResult, err := s.ports.VariantWriter.Write(ctx, cc.OrganizationID, variantWrites)
		if err != nil {
			return fmt.Errorf("erp: variant write: %w", err)
		}
		for i := range plan.rows {
			row := &plan.rows[i]
			if !row.wantWrite {
				continue
			}
			if failure, bad := writeResult.Failures[row.writerRef]; bad {
				row.state = "failed"
				row.code = "ERP_ITEM_WRITE_FAILED"
				row.message = bilingual(row.code)
				row.wantWrite = false
				s.logItemWriteFailure(cc, "catalog", row.writerRef, failure)
				continue
			}
			// A tier-matched update keeps the variant the tier found; only an
			// insert adopts the id the catalog writer created.
			if row.variantID == 0 {
				row.variantID = writeResult.IDs[row.writerRef]
			}
		}
	}

	// ---- quotas ----------------------------------------------------------
	quotas := map[int64]*int{}
	for i := range plan.rows {
		row := &plan.rows[i]
		if !row.wantWrite || row.variantID == 0 {
			continue
		}
		if row.item.IsQuota != nil && *row.item.IsQuota && row.item.QuotaQty != nil {
			pair := *row.item.QuotaQty
			quotas[row.variantID] = &pair
		}
	}
	if len(quotas) > 0 {
		// Quotas are auxiliary metadata on top of the written variants: a
		// failure here must not lose the write, so it is contained here.
		_ = s.ports.VariantWriter.SetQuotaLimits(ctx, cc.OrganizationID, quotas)
	}

	// ---- stock -----------------------------------------------------------
	var stockRows []StockWrite
	for i := range plan.rows {
		row := &plan.rows[i]
		if !row.wantWrite || row.variantID == 0 {
			continue
		}
		stockRows = append(stockRows, StockWrite{
			Ref: row.writerRef, OrganizationID: cc.OrganizationID, WarehouseID: cc.WarehouseID,
			ProductID: row.productID, VariantID: row.variantID,
			Quantity: row.item.Stock, HasQuantity: true,
		})
	}
	if len(stockRows) > 0 {
		failures, err := s.ports.StockWriter.Replace(ctx, stockRows)
		if err != nil {
			return fmt.Errorf("erp: stock replace: %w", err)
		}
		for _, stockRow := range stockRows {
			if failure, bad := failures[stockRow.Ref]; bad {
				for i := range plan.rows {
					if plan.rows[i].writerRef == stockRow.Ref && plan.rows[i].wantWrite {
						plan.rows[i].state = "failed"
						plan.rows[i].code = "ERP_ITEM_WRITE_FAILED"
						plan.rows[i].message = bilingual(plan.rows[i].code)
						plan.rows[i].wantWrite = false
						s.logItemWriteFailure(cc, "inventory", stockRow.Ref, failure)
						break
					}
				}
			}
		}
	}

	// ---- persisted results, links, reviews, memory -----------------------
	results := make([]ItemResult, 0, len(plan.rows))
	links := make([]LinkWrite, 0, len(plan.rows))
	var reviews []ReviewWrite
	for i := range plan.rows {
		row := &plan.rows[i]
		results = append(results, ItemResult{
			ItemID: row.row.ID, State: row.state, Code: row.code,
			Message: row.message, VariantID: variantPtr(row),
		})
		if row.review != nil {
			if row.row.ID != 0 {
				row.review.BatchID = row.row.BatchID
			}
			reviews = append(reviews, *row.review)
		}
		if !row.wantWrite {
			continue
		}
		if row.linkWrite != nil {
			row.linkWrite.VariantID = row.variantID
			row.linkWrite.TaxPercent = row.item.Tax
			links = append(links, *row.linkWrite)
		} else if row.loadedLink != nil {
			links = append(links, LinkWrite{
				IntegrationID: cc.IntegrationID, VariantID: row.variantID,
				ProductID: row.loadedLink.ProductID, ExternalCode: row.item.Code,
				LinkSource: row.loadedLink.LinkSource, LastHash: row.item.Hash(cc.WarehouseID, cc.BranchID),
				TaxPercent: row.item.Tax,
			})
		} else if row.variantID != 0 {
			links = append(links, LinkWrite{
				IntegrationID: cc.IntegrationID, VariantID: row.variantID,
				ProductID: nonZeroPtr(row.productID), ExternalCode: row.item.Code,
				LinkSource: "sku_match", LastHash: row.item.Hash(cc.WarehouseID, cc.BranchID),
				TaxPercent: row.item.Tax,
			})
		}
	}
	if len(plan.remembers) > 0 {
		for i := range plan.remembers {
			if err := s.ports.MatchMemory.Remember(ctx, productmatch.Scope{
				SourceNamespace: plan.remembers[i].SourceNamespace,
				Workflow:        productmatch.WorkflowVendor,
				EntryPoint:      "erp_sync",
			}, plan.remembers[i]); err != nil {
				// The knowledge write is a cache refresh: a failure is logged
				// by the adapter and must not roll the chunk back.
				_ = err
			}
		}
	}
	return s.repo.WriteResults(ctx, cc.OrganizationID, results, links, reviews)
}

func itemUnit(i Item) string { return i.Unit }

func variantPtr(row *plannedRow) *int64 {
	if row.variantID == 0 {
		return nil
	}
	value := row.variantID
	return &value
}

func nonZeroPtr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

// logItemWriteFailure records why a single row failed in the catalog or the
// stock writer. The item itself only carries the stable code; the reason is
// the adapter's own failure text (no key material reaches this path).
func (s *Service) logItemWriteFailure(cc chunkCtx, writer string, ref int, reason string) {
	if s.log == nil {
		return
	}
	s.log.Warn("erp item write failed", "writer", writer, "ref", ref,
		"organization_id", cc.OrganizationID, "reason", reason)
}
