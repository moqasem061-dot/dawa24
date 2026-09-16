package inventory

import (
	"context"
	"errors"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Bulk stock writing for the vendor catalogue import.
//
// UpsertStock exists already and is right for one adjustment from a screen. It
// is wrong for an import twice over: it is a transaction per row, and its
// ON CONFLICT clause deliberately leaves the quantity alone — which is correct
// when a warehouse screen is renaming a threshold and silently wrong when a
// supplier is uploading this morning's balances.
//
// So the import states what it means by the quantity it carries, and gets it.

// ErrBulkStockUnavailable means the repository in use does not implement the
// bulk stock writer.
var ErrBulkStockUnavailable = errors.New("inventory: bulk stock writing is not configured")

// StockMode says what an imported quantity means for a balance that already
// exists.
type StockMode string

const (
	// StockReplace sets the balance to the file's figure. This is what a
	// supplier's daily stock export means.
	StockReplace StockMode = "replace"
	// StockAdd treats the figure as a delivery to add to what is there. This is
	// what a goods-received note means.
	StockAdd StockMode = "add"
	// StockKeep leaves the balance untouched and updates only the thresholds,
	// for a file that is a price list and says nothing about stock.
	StockKeep StockMode = "keep"
)

// Label renders a mode in Arabic.
func (m StockMode) Label() string {
	switch m {
	case StockAdd:
		return i18n.TDefault("w4_mod.s_399_399")
	case StockKeep:
		return i18n.TDefault("w4_mod.s_400_400")
	default:
		return i18n.TDefault("w4_mod.s_401_401")
	}
}

// StockWriteRow is one balance to write, carrying the caller's reference so a
// failure can be reported against the spreadsheet row it came from.
type StockWriteRow struct {
	Ref   int
	Stock *Stock
	// HasQuantity is false when the file said nothing about this row's balance,
	// in which case the quantity is left alone whatever the mode says. A
	// missing cell is not a zero.
	HasQuantity bool
}

// StockWriteResult accounts for a batched write.
type StockWriteResult struct {
	Written  int            `json:"written"`
	Failures []StockFailure `json:"failures,omitempty"`
}

// StockFailure records one balance the database refused.
type StockFailure struct {
	Ref     int    `json:"ref"`
	Message string `json:"message"`
}

// StockWriter is the persistence the vendor import needs beyond the ordinary
// repository.
type StockWriter interface {
	BulkWriteStocks(ctx context.Context, mode StockMode, rows []StockWriteRow) (StockWriteResult, error)
	VariantIDsInWarehouse(ctx context.Context, warehouseID int64) (map[int64]bool, error)
}

// VariantIDsInWarehouse reports which variants already hold a balance in one
// warehouse, so an import can update the balance the vendor actually sells from
// instead of creating a second variant beside it.
func (s *Service) VariantIDsInWarehouse(
	ctx context.Context, warehouseID int64,
) (map[int64]bool, error) {
	writer, ok := s.repo.(StockWriter)
	if !ok {
		return map[int64]bool{}, nil
	}
	return writer.VariantIDsInWarehouse(ctx, warehouseID)
}

// BulkWriteStocks writes a batch of warehouse balances in one transaction.
func (s *Service) BulkWriteStocks(
	ctx context.Context, mode StockMode, rows []StockWriteRow,
) (StockWriteResult, error) {
	writer, ok := s.repo.(StockWriter)
	if !ok {
		return StockWriteResult{}, ErrBulkStockUnavailable
	}
	if len(rows) == 0 {
		return StockWriteResult{}, nil
	}
	if mode == "" {
		mode = StockReplace
	}
	return writer.BulkWriteStocks(ctx, mode, rows)
}
