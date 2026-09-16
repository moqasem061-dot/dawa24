package catalog

import (
	"context"
	"errors"
)

// ErrBulkVariantsUnavailable means the repository in use does not implement the
// bulk variant writer. Every production repository does; a test double need not.
var ErrBulkVariantsUnavailable = errors.New("catalog: bulk variant writing is not configured")

// Bulk variant writing for the vendor catalogue import.
//
// A vendor's price list is three hundred to nine thousand lines. Writing it
// through CreateVariant and UpdateVariant means one transaction per line, which
// on a remote database is nine thousand round trips and several minutes of a
// vendor watching a spinner. These three operations replace that with one query
// to learn what already exists and one batched write per five hundred rows.

// VariantKey is an existing variant reduced to the fields the import matches
// on. The whole of a vendor's catalogue is loaded as these before an import
// runs, which is cheap — a vendor with ten thousand variants costs a couple of
// megabytes — and turns the per-row "does this already exist" question from a
// query into a map lookup.
type VariantKey struct {
	ID          int64  `json:"id"`
	ProductID   int64  `json:"product_id"`
	SKU         string `json:"sku"`
	Barcode     string `json:"barcode"`
	Unit        string `json:"unit"`
	BatchNumber string `json:"batch_number"`
	BranchID    *int64 `json:"branch_id,omitempty"`
	// Active says whether the variant is currently on sale. The replace mode's
	// preview needs it: an already-inactive variant is not one the run is about
	// to take off sale, and counting it told the vendor a run would delist
	// hundreds of items it would not touch.
	Active bool `json:"active"`
}

// VariantWriteRow is one variant to write, carrying the caller's own reference
// so a failure can be reported against the spreadsheet row it came from.
type VariantWriteRow struct {
	// Ref is the caller's index. It is never persisted; it exists so a database
	// error can be traced back to a row number in the vendor's own file.
	Ref int
	// Variant carries a non-zero ID for an update and zero for an insert.
	Variant *ProductVariant
}

// VariantWriteFailure records one row the database refused.
type VariantWriteFailure struct {
	Ref     int    `json:"ref"`
	Message string `json:"message"`
}

// VariantWriteResult accounts for a batched write.
type VariantWriteResult struct {
	Inserted int                   `json:"inserted"`
	Updated  int                   `json:"updated"`
	Failures []VariantWriteFailure `json:"failures,omitempty"`
	// IDs maps the caller's Ref onto the variant id that was written, so the
	// stock rows can be attached to variants that did not exist a moment ago.
	IDs map[int]int64 `json:"-"`
	// InsertedRefs marks the refs that created a variant rather than updating
	// one. The caller cannot infer this from what it asked for: a row sent as
	// an update whose variant has since been deleted is inserted instead, and
	// reporting that as "updated" would tell the vendor their catalogue was
	// refreshed when it was in fact extended.
	InsertedRefs map[int]bool `json:"-"`
}

// VariantWriter is the persistence the vendor import needs beyond the ordinary
// repository. It is separate so the import can be tested against a fake without
// standing up the whole catalogue.
type VariantWriter interface {
	ListVariantKeys(ctx context.Context, orgID int64) ([]VariantKey, error)
	BulkWriteVariants(ctx context.Context, orgID int64, rows []VariantWriteRow) (VariantWriteResult, error)
}

// ListVariantKeys loads the vendor's existing variants for import matching.
func (s *Service) ListVariantKeys(ctx context.Context, orgID int64) ([]VariantKey, error) {
	writer, ok := s.repo.(VariantWriter)
	if !ok {
		return nil, ErrBulkVariantsUnavailable
	}
	return writer.ListVariantKeys(ctx, orgID)
}

// BulkWriteVariants inserts and updates a batch of variants in one transaction.
func (s *Service) BulkWriteVariants(
	ctx context.Context, orgID int64, rows []VariantWriteRow,
) (VariantWriteResult, error) {
	writer, ok := s.repo.(VariantWriter)
	if !ok {
		return VariantWriteResult{}, ErrBulkVariantsUnavailable
	}
	if len(rows) == 0 {
		return VariantWriteResult{IDs: map[int]int64{}}, nil
	}
	for _, row := range rows {
		if row.Variant == nil {
			continue
		}
		if row.Variant.OrganizationID == 0 {
			row.Variant.OrganizationID = orgID
		}
		// An empty status on an UPDATE means "leave the column alone", which is
		// how an import refreshes prices without republishing a variant the
		// vendor took off sale. Defaulting it here would have quietly
		// reactivated every matched variant of every import — so it is only a
		// default for the inserts, which genuinely need one.
		if row.Variant.ID == 0 && row.Variant.Status == "" {
			row.Variant.Status = StatusActive
		}
		if row.Variant.MinOrderQty <= 0 {
			row.Variant.MinOrderQty = 1
		}
	}
	return writer.BulkWriteVariants(ctx, orgID, rows)
}
