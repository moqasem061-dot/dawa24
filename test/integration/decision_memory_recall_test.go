package integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	matchingpg "github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// A decision the import pipeline files is a decision the next import obeys.
//
// The two halves used to be separate failures wearing one symptom. Nothing wrote
// memory at all — the pipelines were handed a read-only repository — and even if
// something had, an answer filed under the cache key could never be found again,
// because the matcher looks memory up by the question's signature. This test
// files the answer exactly as the import adapter does (vendor namespace, the
// row's own signature, human confirmation) and then asks the matcher to prepare
// the same row: it must come back accepted, from knowledge, without the engine
// deciding anything.
func TestFiledDecisionIsRecalledByTheNextImport(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(db.Close)
	system := database.AsSystem(context.Background())

	var orgID, productID int64
	err := db.InTx(system, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`INSERT INTO org.organizations(name, type, status) VALUES('{"ar":"اختبار الاسترجاع","en":"Recall test"}', 'supplier', 'approved') RETURNING id`,
		).Scan(&orgID); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`INSERT INTO catalog.products(organization_id, name, sku, concentration, dosage_form)
			 VALUES($1, '{"ar":"بانادول ٥٠٠ مجم","en":"Panadol 500 mg"}', 'RECALL-1', '500 mg', 'tablet') RETURNING id`,
			orgID,
		).Scan(&productID)
	})
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	t.Cleanup(func() {
		_ = db.InTx(system, func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `DELETE FROM catalog.match_knowledge WHERE organization_id = $1`, orgID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM catalog.match_runs WHERE organization_id = $1`, orgID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM catalog.products WHERE id = $1`, productID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `DELETE FROM org.organizations WHERE id = $1`, orgID)
			return err
		})
	})

	row := productmatch.Row{Number: 1, Name: "بانادول ٥٠٠ مجم", SKU: "RECALL-1"}
	signature := productmatch.QuestionSignature(
		productmatch.ExtractRowFacts(&row, nil), row.Name, productmatch.NormalizerVersion)

	repo := matchingpg.NewRepository(db)
	loader := matching.NewLoader(db)
	svc := matching.NewService(repo, matching.NewIndexCache(matching.NewDefaultIndexBuilder(loader)), loader)

	ctx := authctx.WithActor(database.WithTenant(context.Background(), orgID),
		authctx.Actor{UserID: 1, OrganizationID: orgID})

	// What the import adapter writes when a person confirms a row.
	if err := svc.Remember(ctx, matching.Scope{
		EffectiveOrg:    orgID,
		ActorUserID:     1,
		SourceNamespace: string(matching.WorkflowVendor),
		Workflow:        matching.WorkflowVendor,
		EntryPoint:      "ingest.ai_enhance",
	}, matching.KnowledgeWrite{
		SourceNamespace:   string(matching.WorkflowVendor),
		QuestionSignature: signature,
		Kind:              "human_confirm",
		TargetProductID:   &productID,
		PolicyVersion:     productmatch.DefaultPolicy().Version,
		NormalizerVersion: productmatch.NormalizerVersion,
		DisplayText:       row.Name,
	}); err != nil {
		t.Fatalf("Remember: %v", err)
	}

	res, err := svc.Prepare(ctx, productmatch.PrepareRequest{
		Scope: productmatch.Scope{
			EffectiveOrg:    orgID,
			ActorUserID:     1,
			SourceNamespace: string(matching.WorkflowVendor),
			Workflow:        matching.WorkflowVendor,
			EntryPoint:      "vendor_stage_run",
		},
		ConsumerRef: "test:recall",
		SourceRev:   1,
		Rows:        []productmatch.Row{row},
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(res.Decisions) != 1 {
		t.Fatalf("got %d decisions, want 1", len(res.Decisions))
	}
	dec := res.Decisions[0]
	if dec.State != matchflow.StateAccepted {
		t.Fatalf("state = %s (%v), want accepted from the remembered decision", dec.State, dec.Reasons)
	}
	if dec.ProductID == nil || *dec.ProductID != productID {
		t.Fatalf("product = %v, want the remembered %d", dec.ProductID, productID)
	}
	if dec.Basis != matchflow.BasisKnowledgeHuman {
		t.Errorf("basis = %s, want knowledge_human: the decision was re-derived instead of recalled", dec.Basis)
	}
	if res.Summary.KnowledgeReuse == 0 {
		t.Error("the run reports no knowledge reuse, so the answer was not read back")
	}
}

// An answer filed for one organisation is invisible to another.
func TestFiledDecisionIsNotSharedAcrossOrganisations(t *testing.T) {
	// getTestDB, not connectTestDB: this assertion rests on row-level security,
	// and a superuser connection bypasses it unconditionally (see rls_test.go).
	db := getTestDB(t)
	t.Cleanup(db.Close)
	system := database.AsSystem(context.Background())

	var orgA, orgB, productID int64
	err := db.InTx(system, func(ctx context.Context, tx pgx.Tx) error {
		for _, target := range []*int64{&orgA, &orgB} {
			if err := tx.QueryRow(ctx,
				`INSERT INTO org.organizations(name, type, status) VALUES('{"ar":"منشأة","en":"Org"}', 'supplier', 'approved') RETURNING id`,
			).Scan(target); err != nil {
				return err
			}
		}
		return tx.QueryRow(ctx,
			`INSERT INTO catalog.products(organization_id, name, sku) VALUES($1, '{"ar":"بانادول","en":"Panadol"}', 'RECALL-2') RETURNING id`,
			orgA,
		).Scan(&productID)
	})
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	t.Cleanup(func() {
		_ = db.InTx(system, func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `DELETE FROM catalog.match_knowledge WHERE organization_id IN ($1, $2)`, orgA, orgB); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM catalog.products WHERE id = $1`, productID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `DELETE FROM org.organizations WHERE id IN ($1, $2)`, orgA, orgB)
			return err
		})
	})

	row := productmatch.Row{Number: 1, Name: "بانادول", SKU: "RECALL-2"}
	signature := productmatch.QuestionSignature(
		productmatch.ExtractRowFacts(&row, nil), row.Name, productmatch.NormalizerVersion)

	repo := matchingpg.NewRepository(db)
	loader := matching.NewLoader(db)
	svc := matching.NewService(repo, matching.NewIndexCache(matching.NewDefaultIndexBuilder(loader)), loader)

	ctxA := authctx.WithActor(database.WithTenant(context.Background(), orgA),
		authctx.Actor{UserID: 1, OrganizationID: orgA})
	if err := svc.Remember(ctxA, matching.Scope{
		EffectiveOrg: orgA, ActorUserID: 1,
		SourceNamespace: string(matching.WorkflowVendor), Workflow: matching.WorkflowVendor,
	}, matching.KnowledgeWrite{
		SourceNamespace:   string(matching.WorkflowVendor),
		QuestionSignature: signature,
		Kind:              "human_confirm",
		TargetProductID:   &productID,
		PolicyVersion:     productmatch.DefaultPolicy().Version,
		NormalizerVersion: productmatch.NormalizerVersion,
		DisplayText:       row.Name,
	}); err != nil {
		t.Fatalf("Remember: %v", err)
	}

	// B prepared the same row: nothing of A's may leak into it.
	ctxB := authctx.WithActor(database.WithTenant(context.Background(), orgB),
		authctx.Actor{UserID: 2, OrganizationID: orgB})
	page, err := svc.ListKnowledge(ctxB, matching.Scope{EffectiveOrg: orgB, Workflow: matching.WorkflowVendor},
		matching.KnowledgeFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListKnowledge as B: %v", err)
	}
	for _, item := range page.Items {
		if item.OrganizationID != nil && *item.OrganizationID == orgA {
			t.Fatalf("organisation B can read organisation A's memory row %d", item.ID)
		}
	}
}
