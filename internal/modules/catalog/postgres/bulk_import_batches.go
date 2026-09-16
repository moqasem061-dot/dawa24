package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// insertStatus is the status a brand-new catalogue row gets: whatever the file
// stated, or active when it stated nothing. The column is NOT NULL with a CHECK,
// so an empty string would be refused.
func insertStatus(p *catalog.Product) catalog.ProductStatus {
	if p.Status == "" {
		return catalog.StatusActive
	}
	return p.Status
}

func scanInsert(br pgx.BatchResults, p *catalog.Product, res *catalog.BulkWriteResult) error {
	var id int64
	if err := br.QueryRow().Scan(&id); err != nil {
		return err
	}
	p.ID = id
	res.Inserted++
	return nil
}

func queueUpdate(batch *pgx.Batch, p *catalog.Product) {
	batch.Queue(updateProductQuery,
		p.ID, p.Name, p.Description, p.SKU, p.Barcode,
		p.Price, p.Discount, p.OldPrice, string(p.Status), p.DosageForm,
		p.ScientificName, p.Active, p.Concentration, p.Unit,
		p.ManufacturingCompanies, p.BrandID, p.Image, p.ImageLink,
	)
}

func scanUpdate(br pgx.BatchResults, _ *catalog.Product, res *catalog.BulkWriteResult) error {
	if _, err := br.Exec(); err != nil {
		return err
	}
	res.Updated++
	return nil
}

// batchWriter pipelines one kind of statement — inserts or updates — over the
// products it is handed.
//
// The two operations differ only in what they queue and how they read the
// result, so they share everything else: the chunking, the savepoint discipline,
// and the per-row diagnosis after a failure.
type batchWriter struct {
	tx    pgx.Tx
	prods []*catalog.Product
	res   *catalog.BulkWriteResult
	// queue appends one product's statement to a batch.
	queue func(*pgx.Batch, *catalog.Product)
	// scan reads that statement's result and updates the running counts.
	scan func(pgx.BatchResults, *catalog.Product, *catalog.BulkWriteResult) error
}

// write sends the given product indices in pipelined chunks.
//
// On failure the chunk is replayed one statement at a time behind a savepoint,
// which is the only way to learn which row PostgreSQL objected to: once a
// statement in a batch fails the transaction is aborted, and every later
// statement reports the same "current transaction is aborted" error regardless
// of its own content. The replay costs one extra round trip per row of a single
// failing chunk, and only on the path that was going to fail anyway.
func (w *batchWriter) write(ctx context.Context, idxs []int) error {
	for start := 0; start < len(idxs); start += chunkSize {
		chunk := idxs[start:min(start+chunkSize, len(idxs))]

		// The savepoint is what makes the per-row diagnosis possible: a failed
		// statement aborts the transaction, and only a rollback to a savepoint
		// taken before the batch can make it usable again.
		if _, err := w.tx.Exec(ctx, `SAVEPOINT bulk_chunk`); err != nil {
			return fmt.Errorf("catalog postgres: open bulk savepoint: %w", err)
		}

		batchErr := w.sendChunk(ctx, chunk)
		if batchErr == nil {
			if _, err := w.tx.Exec(ctx, `RELEASE SAVEPOINT bulk_chunk`); err != nil {
				return fmt.Errorf("catalog postgres: release bulk savepoint: %w", err)
			}
			continue
		}

		w.res.Failures = w.diagnose(ctx, chunk)
		if len(w.res.Failures) == 0 {
			// The replay found nothing, so the fault was not row-specific.
			return fmt.Errorf("catalog postgres: bulk write failed: %w", batchErr)
		}
		return errors.New(w.res.Error())
	}
	return nil
}

// sendChunk pipelines one chunk and returns the first error it produced.
func (w *batchWriter) sendChunk(ctx context.Context, chunk []int) error {
	batch := &pgx.Batch{}
	for _, i := range chunk {
		w.queue(batch, w.prods[i])
	}

	br := w.tx.SendBatch(ctx, batch)
	var firstErr error
	for _, i := range chunk {
		if err := w.scan(br, w.prods[i], w.res); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := br.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// diagnose replays a failed chunk row by row to attribute the failure.
//
// The transaction is already aborted by the time this runs, so it rolls back to
// the chunk savepoint, then tries each statement inside its own savepoint.
// Nothing it writes survives: the caller returns an error either way and the
// whole transaction is rolled back.
func (w *batchWriter) diagnose(ctx context.Context, chunk []int) []catalog.WriteFailure {
	if _, err := w.tx.Exec(ctx, `ROLLBACK TO SAVEPOINT bulk_chunk`); err != nil {
		// No savepoint to return to; the caller still reports the batch error.
		return nil
	}

	var failures []catalog.WriteFailure
	for _, i := range chunk {
		err := w.tryOne(ctx, i)
		if err == nil {
			continue
		}
		failures = append(failures, catalog.WriteFailure{
			Index:  i,
			Name:   w.prods[i].Name.Get(i18n.AR),
			SKU:    w.prods[i].SKU,
			Reason: explainWriteError(err),
		})
		// Twenty named rows is already more than an admin will act on in one
		// pass, and the total count is reported separately.
		if len(failures) >= maxNamedFailures {
			break
		}
	}
	return failures
}

// tryOne runs a single product's statement behind its own savepoint, leaving the
// transaction usable whether it succeeds or fails.
func (w *batchWriter) tryOne(ctx context.Context, idx int) error {
	if _, err := w.tx.Exec(ctx, `SAVEPOINT bulk_row`); err != nil {
		return err
	}

	batch := &pgx.Batch{}
	w.queue(batch, w.prods[idx])
	br := w.tx.SendBatch(ctx, batch)
	_, err := br.Exec()
	if closeErr := br.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		_, _ = w.tx.Exec(ctx, `ROLLBACK TO SAVEPOINT bulk_row`)
		return err
	}
	_, _ = w.tx.Exec(ctx, `RELEASE SAVEPOINT bulk_row`)
	return nil
}

// explainWriteError turns a PostgreSQL error into something an admin can act on.
func explainWriteError(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err.Error()
	}

	switch pgErr.Code {
	case "23505": // unique_violation
		return i18n.TDefault("w4_mod.s_344_344")
	case "23503": // foreign_key_violation
		return i18n.TDefault("w4_mod.w4str_117_117")
	case "23514": // check_violation
		return fmt.Sprintf(i18n.TDefault("w4_mod.s_118"), pgErr.ConstraintName)
	case "23502": // not_null_violation
		return fmt.Sprintf(i18n.TDefault("w4_mod.s_119"), pgErr.ColumnName)
	case "22001": // string_data_right_truncation
		return i18n.TDefault("w4_mod.s_345_345")
	case "22003": // numeric_value_out_of_range
		return i18n.TDefault("w4_mod.9_999_999_999_99_120")
	case "22P02": // invalid_text_representation
		return i18n.TDefault("w4_mod.s_346_346")
	default:
		return fmt.Sprintf(i18n.TDefault("w4_mod.s_s_121"), pgErr.Message, pgErr.Code)
	}
}

// MatchExistingProducts resolves which catalogue products the incoming rows
// correspond to, without writing anything.
//
// The review screen needs the same answer the commit will reach — "this row
// updates product 4,812" — before the admin approves it. Sharing the resolver
// rather than reimplementing it is what keeps the preview honest: a row shown as
// an update cannot turn into an insert at commit time.
func (r *Repository) MatchExistingProducts(
	ctx context.Context, prods []*catalog.Product,
) (map[int]catalog.ExistingMatch, error) {
	out := map[int]catalog.ExistingMatch{}
	if len(prods) == 0 {
		return out, nil
	}

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// Matching is scoped per organisation, and a freshly parsed product has
		// none yet — the write path assigns it. The same assignment has to
		// happen here or the preview would match every row against organisation
		// zero, find nothing, and promise an insert for rows the commit would
		// then update.
		defaultOrgID, err := lookupDefaultOrg(txCtx, tx)
		if err != nil {
			return err
		}
		if defaultOrgID <= 0 {
			// No organisation means an empty catalogue: nothing to match.
			return nil
		}
		for _, p := range prods {
			if p.OrganizationID <= 0 {
				p.OrganizationID = defaultOrgID
			}
		}

		matches, err := resolveExistingProducts(txCtx, tx, prods)
		if err != nil {
			return err
		}
		for idx, m := range matches {
			out[idx] = catalog.ExistingMatch{ProductID: m.id, Reason: m.reason}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DefaultCatalogOrg returns the organisation that owns the master catalogue,
// creating it on a fresh deployment that has none.
//
// The import session is tied to it at creation, so a staged row and the product
// it eventually writes are scoped to the same organisation — matching a preview
// against one organisation and committing into another would show the admin one
// answer and perform a different one.
func (r *Repository) DefaultCatalogOrg(ctx context.Context) (int64, error) {
	var orgID int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var err error
		orgID, err = resolveDefaultOrg(txCtx, tx)
		return err
	})
	return orgID, err
}
