package main

import (
	"context"
	"fmt"
	"log/slog"

	ingestPG "github.com/muhiya/dawa24-store/internal/modules/ingest/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Releasing imports wedged in the processing phase.
//
// A staging run lives in the web process, so a deploy or a crash can leave a
// session in 'processing' with nobody working on it — a progress bar that polls
// for ever. The same state was produced for a while by a defect: the run wrote
// its outcome through SaveDraft, whose phase predicate excludes 'processing',
// so the write matched zero rows and the session wedged with the matching
// complete and nothing the vendor could reach.
//
// The defect is fixed. This exists because the sessions it produced are still
// in the database, and because a crash can still produce them.
func importsRecover(ctx context.Context, db *database.DB, log *slog.Logger, apply bool) error {
	// Import sessions belong to tenants, but this sweeps every tenant's, so it
	// runs as the system deliberately and greppably.
	sysCtx := database.AsSystem(ctx)
	repo := ingestPG.NewRepository(db)

	if !apply {
		wedged, err := repo.CountWedgedRuns(sysCtx)
		if err != nil {
			return err
		}
		fmt.Printf("%d import session(s) are wedged in 'processing'.\n", wedged)
		if wedged > 0 {
			fmt.Println("Re-run with --apply to release them:")
			fmt.Println("  sessions whose rows are staged are promoted to review, with counters recomputed")
			fmt.Println("  sessions with no rows are failed, so the vendor is told to upload again")
		}
		return nil
	}

	recovered, err := repo.RecoverStaleRuns(sysCtx)
	if err != nil {
		return err
	}
	log.InfoContext(ctx, "released wedged import runs", "count", recovered)
	fmt.Printf("released %d wedged import session(s)\n", recovered)
	return nil
}
