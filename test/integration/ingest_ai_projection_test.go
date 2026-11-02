package integration_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestIngestAIUpdatesImportLinkAndCandidateProjection(t *testing.T) {
	// getTestDB, not connectTestDB: the first assertion below is a cross-tenant
	// write, and the guard against it is row-level security. A superuser
	// connection bypasses RLS unconditionally, FORCE included, so on such a DSN
	// this test reports a leak that is only a fact about the connection. The
	// helper skips with that reason instead (see rls_test.go).
	db := getTestDB(t)
	t.Cleanup(db.Close)
	system := database.AsSystem(context.Background())
	var orgID, importID, rowID, productID, runID int64
	err := db.InTx(system, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO org.organizations(name) VALUES ('{"en":"AI projection test"}') RETURNING id`).Scan(&orgID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO catalog.products(organization_id,name,concentration,dosage_form) VALUES ($1,'{"ar":"Panadol","en":"Panadol"}','500 mg','tablet') RETURNING id`, orgID).Scan(&productID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO catalog.match_runs(organization_id,workflow,entry_point,consumer_ref,policy_version,normalizer_version) VALUES ($1,'vendor','test','test','test','test') RETURNING id`, orgID).Scan(&runID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO ingest.catalog_imports(organization_id,filename,phase) VALUES ($1,'ai-test.csv','settings') RETURNING id`, orgID).Scan(&importID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO ingest.catalog_import_rows(import_id,organization_id,source_row,outcome,match_revision,human_locked) VALUES ($1,$2,1,'staged',1,false) RETURNING id`, importID, orgID).Scan(&rowID)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.InTx(system, func(ctx context.Context, tx pgx.Tx) error {
			for _, query := range []struct {
				sql string
				id  int64
			}{
				{"DELETE FROM ingest.catalog_imports WHERE id=$1", importID},
				{"DELETE FROM catalog.products WHERE id=$1", productID},
				{"DELETE FROM org.organizations WHERE id=$1", orgID},
			} {
				if _, err := tx.Exec(ctx, query.sql, query.id); err != nil {
					return err
				}
			}
			return nil
		})
	})
	ctx := database.WithTenant(context.Background(), orgID)
	applier := ingest.NewCatalogAIApplier(db)
	dec := matchflow.RowDecision{V: 1, State: matchflow.StateAccepted, Basis: matchflow.BasisAI, ProductID: &productID,
		Candidates: []matchflow.CandidateDecision{{ProductID: productID, Score: 0.91}}}
	if applied, err := applier.ApplyAI(database.WithTenant(context.Background(), orgID+1000000), productmatch.RowRef{ID: rowID}, 1, runID, dec); err != nil || applied {
		t.Fatalf("cross-tenant decision applied=%t error=%v", applied, err)
	}
	for index, state := range []matchflow.State{matchflow.StateAccepted, matchflow.StateNeedsReview} {
		dec.State = state
		applied, err := applier.ApplyAI(ctx, productmatch.RowRef{ID: rowID}, index+1, runID, dec)
		if err != nil || !applied {
			t.Fatalf("apply %s: applied=%t error=%v", state, applied, err)
		}
		err = db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var product *int64
			var score float64
			var candidates []byte
			if err := tx.QueryRow(ctx, `SELECT product_id,match_score,candidates FROM ingest.catalog_import_rows WHERE id=$1`, rowID).Scan(&product, &score, &candidates); err != nil {
				return err
			}
			if state == matchflow.StateAccepted && (product == nil || *product != productID) {
				t.Fatal("accepted decision was not importable")
			}
			if state == matchflow.StateNeedsReview && product != nil {
				t.Fatal("unsettled decision retained a stale import link")
			}
			var projected []productmatch.MatchCandidate
			if err := json.Unmarshal(candidates, &projected); err != nil {
				return err
			}
			if score != 0.91 || len(projected) != 1 || projected[0].Name != "Panadol" || projected[0].Score != score || projected[0].Concentration != "500 mg" || projected[0].DosageForm != "tablet" {
				t.Fatalf("stale score/candidates: %s", candidates)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if applied, err := applier.ApplyAI(ctx, productmatch.RowRef{ID: rowID}, 1, runID, dec); err != nil || applied {
		t.Fatalf("obsolete decision applied=%t error=%v", applied, err)
	}
	if err := db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE ingest.catalog_import_rows SET human_locked=true WHERE id=$1`, rowID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if applied, err := applier.ApplyAI(ctx, productmatch.RowRef{ID: rowID}, 3, runID, dec); err != nil || applied {
		t.Fatalf("human-locked decision applied=%t error=%v", applied, err)
	}
}
