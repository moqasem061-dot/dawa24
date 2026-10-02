package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type backfillCounts struct {
	decisionsManualFound     int
	decisionsManualActivated int
	decisionsManualPending   int
	decisionsAIFound         int
	aliasesFound             int
	appliedTotal             int
}

func runMatchKnowledgeBackfill(ctx context.Context, db *database.DB, log *slog.Logger, args []string) error {
	apply := false
	for _, arg := range args {
		if arg == "--apply" {
			apply = true
		}
	}

	modeStr := "DRY-RUN"
	if apply {
		modeStr = "APPLY"
	}
	fmt.Printf("=== Match Knowledge Backfill [%s] ===\n", modeStr)

	// 1. Load active products into Index for gate evaluation
	loader := matching.NewLoader(db)
	products, _, err := loader.Load(ctx)
	if err != nil {
		return fmt.Errorf("loading products for gate evaluation: %w", err)
	}
	idx := productmatch.NewIndex(products)
	gate := productmatch.NewGate(idx)
	pol := productmatch.DefaultPolicy()

	var counts backfillCounts
	var knowledgeToInsert []matching.Knowledge

	// 2. Scan catalog.match_decisions
	err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT organization_id, scope, norm_name, chosen_product_id, source, prompt_version, last_used_at
			FROM catalog.match_decisions
			ORDER BY id ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var orgID *int64
			var scope string
			var normName string
			var chosenProductID *int64
			var source string
			var promptVer string
			var lastUsedAt *time.Time

			if err := rows.Scan(&orgID, &scope, &normName, &chosenProductID, &source, &promptVer, &lastUsedAt); err != nil {
				return err
			}

			normName = strings.TrimSpace(normName)
			if normName == "" {
				continue
			}

			rowObj := &productmatch.Row{Name: normName}
			rowFacts := productmatch.ExtractRowFacts(rowObj, nil)
			sig := productmatch.QuestionSignature(rowFacts, normName, productmatch.NormalizerVersion)

			var effectiveOrg *int64
			if scope != "platform" && orgID != nil && *orgID > 0 {
				effectiveOrg = orgID
			}

			switch source {
			case "manual", "admin", "import":
				counts.decisionsManualFound++
				status := "pending_revalidation"
				targetFP := ""

				if chosenProductID != nil && *chosenProductID > 0 {
					if targetProd, ok := idx.Lookup(*chosenProductID); ok && targetProd != nil {
						prop := productmatch.Proposal{
							Path:      productmatch.PathReviewer,
							ProductID: targetProd.ID,
						}
						verdict := gate.Evaluate(rowFacts, prop, pol)
						if verdict.Eligible && verdict.State == matchflow.StateAccepted {
							status = "active"
							targetFP = matching.ProductFactFingerprint(targetProd)
							counts.decisionsManualActivated++
						} else {
							counts.decisionsManualPending++
						}
					} else {
						counts.decisionsManualPending++
					}
				} else {
					// Confirmed rejection
					status = "active"
					counts.decisionsManualActivated++
				}

				kind := "human_confirm"
				if chosenProductID == nil || *chosenProductID <= 0 {
					kind = "human_reject"
				}

				k := matching.Knowledge{
					OrganizationID:        effectiveOrg,
					SourceNamespace:       "*",
					QuestionSignature:     sig,
					Kind:                  kind,
					TargetProductID:       chosenProductID,
					TargetGranularity:     "product",
					TargetFactFingerprint: targetFP,
					PolicyVersion:         pol.Version,
					NormalizerVersion:     productmatch.NormalizerVersion,
					DisplayText:           normName,
					Status:                status,
					Legacy:                true,
					CreatedBy:             "backfill",
					LastUsedAt:            lastUsedAt,
				}
				knowledgeToInsert = append(knowledgeToInsert, k)
			case "ai":
				counts.decisionsAIFound++
				targetFP := ""
				if chosenProductID != nil && *chosenProductID > 0 {
					if targetProd, ok := idx.Lookup(*chosenProductID); ok && targetProd != nil {
						targetFP = matching.ProductFactFingerprint(targetProd)
					}
				}
				promptPtr := &promptVer
				k := matching.Knowledge{
					OrganizationID:        effectiveOrg,
					SourceNamespace:       "*",
					QuestionSignature:     sig,
					Kind:                  "ai_answer",
					TargetProductID:       chosenProductID,
					TargetGranularity:     "product",
					TargetFactFingerprint: targetFP,
					PolicyVersion:         pol.Version,
					NormalizerVersion:     productmatch.NormalizerVersion,
					PromptVersion:         promptPtr,
					DisplayText:           normName,
					Status:                "active",
					Legacy:                true,
					CreatedBy:             "backfill",
					LastUsedAt:            lastUsedAt,
				}
				knowledgeToInsert = append(knowledgeToInsert, k)
			}
		}
		return rows.Err()
	})
	if err != nil {
		return fmt.Errorf("reading match_decisions: %w", err)
	}

	// 3. Scan trusted catalog.product_aliases
	err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT product_id, alias, source, created_at
			FROM catalog.product_aliases
			WHERE source IN ('manual', 'saving_product', 'import')
			ORDER BY id ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var productID int64
			var alias string
			var source string
			var createdAt time.Time

			if err := rows.Scan(&productID, &alias, &source, &createdAt); err != nil {
				return err
			}
			alias = strings.TrimSpace(alias)
			if alias == "" || productID <= 0 {
				continue
			}

			counts.aliasesFound++
			rowObj := &productmatch.Row{Name: alias}
			rowFacts := productmatch.ExtractRowFacts(rowObj, nil)
			sig := productmatch.QuestionSignature(rowFacts, alias, productmatch.NormalizerVersion)

			targetFP := ""
			if targetProd, ok := idx.Lookup(productID); ok && targetProd != nil {
				targetFP = matching.ProductFactFingerprint(targetProd)
			}

			pid := productID
			k := matching.Knowledge{
				OrganizationID:        nil,
				SourceNamespace:       "*",
				QuestionSignature:     sig,
				Kind:                  "curated_alias",
				TargetProductID:       &pid,
				TargetGranularity:     "product",
				TargetFactFingerprint: targetFP,
				PolicyVersion:         pol.Version,
				NormalizerVersion:     productmatch.NormalizerVersion,
				DisplayText:           alias,
				Status:                "active",
				Legacy:                true,
				CreatedBy:             "backfill",
				CreatedAt:             createdAt,
			}
			knowledgeToInsert = append(knowledgeToInsert, k)
		}
		return rows.Err()
	})
	if err != nil {
		return fmt.Errorf("reading product_aliases: %w", err)
	}

	fmt.Printf("Decisions (manual/admin/import): %d (pass gate now / active: %d, pending revalidation: %d)\n",
		counts.decisionsManualFound, counts.decisionsManualActivated, counts.decisionsManualPending)
	fmt.Printf("Decisions (AI):                 %d (migrated as legacy ai_answer)\n", counts.decisionsAIFound)
	fmt.Printf("Product Aliases (trusted):      %d (migrated as curated_alias)\n", counts.aliasesFound)
	fmt.Printf("Total candidate records:        %d\n", len(knowledgeToInsert))

	if !apply {
		fmt.Println("\nDry-run complete. No rows written. Use --apply to execute.")
		return nil
	}

	// 4. Apply backfill in batches
	batchSize := 200
	for i := 0; i < len(knowledgeToInsert); i += batchSize {
		end := i + batchSize
		if end > len(knowledgeToInsert) {
			end = len(knowledgeToInsert)
		}
		chunk := knowledgeToInsert[i:end]

		err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			stmt := `
				INSERT INTO catalog.match_knowledge (
					organization_id, source_namespace, question_signature, kind,
					target_product_id, target_granularity, target_fact_fingerprint,
					policy_version, normalizer_version, prompt_version, display_text,
					status, legacy, created_by, created_at, last_used_at
				) VALUES (
					$1, $2, $3, $4,
					$5, $6, $7,
					$8, $9, $10, $11,
					$12, $13, $14, COALESCE($15, now()), $16
				)
				ON CONFLICT (COALESCE(organization_id, 0), source_namespace, question_signature, kind)
					WHERE status IN ('active', 'pending_revalidation')
				DO UPDATE SET
					target_product_id = EXCLUDED.target_product_id,
					target_fact_fingerprint = EXCLUDED.target_fact_fingerprint,
					policy_version = EXCLUDED.policy_version,
					normalizer_version = EXCLUDED.normalizer_version,
					prompt_version = EXCLUDED.prompt_version,
					display_text = EXCLUDED.display_text,
					status = EXCLUDED.status,
					last_used_at = COALESCE(EXCLUDED.last_used_at, catalog.match_knowledge.last_used_at);
			`
			for _, k := range chunk {
				var createdVal any = nil
				if !k.CreatedAt.IsZero() {
					createdVal = k.CreatedAt
				}
				_, err := tx.Exec(txCtx, stmt,
					k.OrganizationID, k.SourceNamespace, k.QuestionSignature, k.Kind,
					k.TargetProductID, k.TargetGranularity, k.TargetFactFingerprint,
					k.PolicyVersion, k.NormalizerVersion, k.PromptVersion, k.DisplayText,
					k.Status, k.Legacy, k.CreatedBy, createdVal, k.LastUsedAt,
				)
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("inserting batch %d-%d: %w", i, end, err)
		}
		counts.appliedTotal += len(chunk)
	}

	fmt.Printf("\nSuccessfully applied %d match_knowledge records.\n", counts.appliedTotal)
	return nil
}
