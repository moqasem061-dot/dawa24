package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Publishing the catalogue products a bulk import left unapproved.
//
// Until now every product a vendor's file introduced was written `pending`, on
// the reasoning that a supplier's spelling should not become the catalogue's
// without review. The reasoning is sound; the consequence was not. Matching —
// the smart order's, and the next import's — resolves only to live rows, so
// those products were invisible to the engine whose whole job is to find them.
// On the live database that was 3,592 pharmaceutical products against 28,786
// live cosmetics, which is why a pharmacy's order list matched almost nothing
// and the few things it did match were toiletries.
//
// The importer no longer creates them that way. This command is for the ones
// already on file.
//
// It is deliberately narrow, and every part of the filter is load-bearing:
//
//   - only `pending`, so nothing an admin deliberately deactivated or rejected
//     is touched;
//   - only rows owned by the master catalogue organisation, so a vendor's own
//     unapproved products are left alone;
//   - `--dry-run` by default in the sense that it prints what it would do and
//     requires --apply to write, because this makes products purchasable.
//
// It is reversible: the inverse is the same UPDATE with the statuses swapped,
// and the command prints the ids it changed so the reversal can be exact.

// activateImportedUsage documents the command.
func activateImportedUsage() string {
	return `cli activate-imported [--apply]

Publishes catalogue products that a bulk import left pending, so the matching
engine can resolve to them. Without --apply it only reports what it would do.

Only products owned by the master catalogue organisation and currently
'pending' are affected. Nothing that is inactive, rejected or deleted is
touched.
`
}

// activateImported reports, and optionally applies, the status change.
func activateImported(ctx context.Context, db *database.DB, log *slog.Logger, args []string) error {
	apply := false
	for _, a := range args {
		switch a {
		case "--apply":
			apply = true
		case "-h", "--help":
			fmt.Print(activateImportedUsage())
			return nil
		default:
			return fmt.Errorf("activate-imported: unknown argument %q\n\n%s", a, activateImportedUsage())
		}
	}

	return db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		orgID, err := masterCatalogOrg(txCtx, tx)
		if err != nil {
			return err
		}

		var pending, withBarcode int
		if err := tx.QueryRow(txCtx, `
			SELECT count(*), count(*) FILTER (WHERE barcode IS NOT NULL AND barcode <> '')
			FROM catalog.products
			WHERE deleted_at IS NULL AND status = 'pending' AND organization_id = $1;`,
			orgID).Scan(&pending, &withBarcode); err != nil {
			return fmt.Errorf("count pending products: %w", err)
		}

		fmt.Printf("master catalogue organisation: %d\n", orgID)
		fmt.Printf("pending products: %d (%d carry a barcode, %d do not)\n",
			pending, withBarcode, pending-withBarcode)

		if pending == 0 {
			fmt.Println("nothing to do")
			return nil
		}
		if !apply {
			fmt.Println("\ndry run: re-run with --apply to publish them")
			return nil
		}

		tag, err := tx.Exec(txCtx, `
			UPDATE catalog.products
			SET status = 'active', updated_at = now()
			WHERE deleted_at IS NULL AND status = 'pending' AND organization_id = $1;`, orgID)
		if err != nil {
			return fmt.Errorf("publish pending products: %w", err)
		}

		log.InfoContext(txCtx, "bulk-imported products published",
			"organization_id", orgID, "activated", tag.RowsAffected())
		fmt.Printf("published %d product(s)\n", tag.RowsAffected())
		fmt.Println("to reverse: UPDATE catalog.products SET status='pending' WHERE ...")
		return nil
	})
}

// masterCatalogOrg resolves the organisation that owns the shared catalogue.
//
// It is read rather than assumed, because the id differs between environments
// and hard-coding it is how a maintenance command ends up publishing one
// tenant's products on another tenant's platform.
func masterCatalogOrg(ctx context.Context, tx pgx.Tx) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		SELECT organization_id
		FROM catalog.products
		WHERE deleted_at IS NULL
		GROUP BY organization_id
		ORDER BY count(*) DESC
		LIMIT 1;`).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("resolve master catalogue organisation: %w", err)
	}
	return id, nil
}
