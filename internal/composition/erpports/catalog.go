package erpports

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// NewVariantWriter adapts the catalog service to the ERP's VariantWriter port.
// Field conversion only (ADR-ERP-1).
func NewVariantWriter(svc *catalog.Service) erp.VariantWriter {
	return catalogVariantWriter{svc: svc}
}

type catalogVariantWriter struct {
	svc *catalog.Service
}

// Write converts erp.VariantWrite rows into catalog.VariantWriteRow rows and
// calls BulkWriteVariants: VariantID 0 inserts, otherwise updates; the ERP
// names are {ar: ArName, en: ArName}; new variants carry the active status
// through the catalog service's own defaults.
func (w catalogVariantWriter) Write(ctx context.Context, orgID int64, rows []erp.VariantWrite) (erp.VariantWriteResult, error) {
	if len(rows) == 0 {
		return erp.VariantWriteResult{IDs: map[int]int64{}, InsertedRefs: map[int]bool{}, Failures: map[int]string{}}, nil
	}
	out := make([]catalog.VariantWriteRow, 0, len(rows))
	for _, row := range rows {
		name := i18n.New(row.Name["ar"], row.Name["en"])
		variant := &catalog.ProductVariant{
			ID:             row.VariantID,
			OrganizationID: orgID,
			ProductID:      row.ProductID,
			Name:           name,
			SKU:            row.SKU,
			Barcode:        row.Barcode,
			Price:          row.Price,
			Discount:       row.Discount,
			Unit:           row.Unit,
			BatchNumber:    row.BatchNumber,
			ExpiryDate:     row.ExpiryDate,
		}
		out = append(out, catalog.VariantWriteRow{Ref: row.Ref, Variant: variant})
	}
	result, err := w.svc.BulkWriteVariants(ctx, orgID, out)
	if err != nil {
		return erp.VariantWriteResult{}, err
	}
	return erp.VariantWriteResult{
		Inserted:     result.Inserted,
		Updated:      result.Updated,
		Failures:     failureText(result.Failures),
		IDs:          result.IDs,
		InsertedRefs: result.InsertedRefs,
	}, nil
}

// SetQuotaLimits delegates the ERP sync's per-branch caps to the catalog's
// bulk quota writer.
func (w catalogVariantWriter) SetQuotaLimits(ctx context.Context, orgID int64, limits map[int64]*int) error {
	return w.svc.SetVariantQuotaLimits(ctx, orgID, limits)
}

// failureText converts catalog's failure list into the ERP's map keyed by Ref.
func failureText(failures []catalog.VariantWriteFailure) map[int]string {
	out := map[int]string{}
	for _, failure := range failures {
		out[failure.Ref] = failure.Message
	}
	return out
}

var _ erp.VariantWriter = catalogVariantWriter{}
var _ = money.Zero
