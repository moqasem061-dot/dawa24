package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

func (r *Repository) InsertFileRows(ctx context.Context, rows []*compare.CompareFileRow) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		cols := []string{
			"file_id", "organization_id", "row_number", "raw_name", "normalized_name", "sku",
			"price", "discount", "price_after_discount", "matched_product_id", "match_confidence", "match_method", "meta",
		}

		const chunkSize = 5000
		for i := 0; i < len(rows); i += chunkSize {
			end := i + chunkSize
			if end > len(rows) {
				end = len(rows)
			}
			chunk := rows[i:end]

			copyRows := make([][]any, len(chunk))
			for j, row := range chunk {
				metaJSON, _ := json.Marshal(row.Meta)
				dbMethod := toDBMatchMethod(row.MatchMethod)

				var skuVal any
				if strings.TrimSpace(row.SKU) != "" {
					skuVal = strings.TrimSpace(row.SKU)
				}

				copyRows[j] = []any{
					row.FileID,
					row.OrganizationID,
					row.RowNumber,
					row.RawName,
					row.NormalizedName,
					skuVal,
					row.Price.String(),
					row.Discount,
					row.PriceAfterDiscount.String(),
					row.MatchedProductID,
					row.MatchConfidence,
					dbMethod,
					metaJSON,
				}
			}

			_, err := tx.CopyFrom(
				txCtx,
				pgx.Identifier{"compare", "file_rows"},
				cols,
				pgx.CopyFromRows(copyRows),
			)
			if err != nil {
				return fmt.Errorf("CopyFrom chunk [%d:%d]: %w", i, end, err)
			}
		}
		return nil
	})
}

func (r *Repository) ListFileRows(ctx context.Context, fileID int64, limit, offset int) ([]*compare.CompareFileRow, error) {
	if limit <= 0 {
		limit = 100
	}
	var list []*compare.CompareFileRow
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + rowColumns + `
			FROM compare.file_rows
			WHERE file_id = $1
			ORDER BY row_number ASC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, fileID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var row compare.CompareFileRow
			var matchMethodStr string
			var metaBytes []byte
			if err := rows.Scan(
				&row.ID, &row.FileID, &row.OrganizationID, &row.RowNumber,
				&row.RawName, &row.NormalizedName, &row.SKU,
				&row.Price, &row.Discount, &row.PriceAfterDiscount,
				&row.MatchedProductID, &row.MatchConfidence, &matchMethodStr,
				&metaBytes, &row.CreatedAt,
			); err != nil {
				return err
			}
			row.MatchMethod = compare.MatchMethod(matchMethodStr)
			if len(metaBytes) > 0 {
				_ = json.Unmarshal(metaBytes, &row.Meta)
			}
			list = append(list, &row)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) GetFileRowsPaginated(ctx context.Context, fileID int64, page, limit int) ([]*compare.CompareFileRow, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	var total int64
	var list []*compare.CompareFileRow
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, `SELECT COUNT(*) FROM compare.file_rows WHERE file_id = $1;`, fileID).Scan(&total); err != nil {
			return err
		}
		query := `
			SELECT ` + rowColumns + `
			FROM compare.file_rows
			WHERE file_id = $1
			ORDER BY row_number ASC, id ASC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, fileID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var row compare.CompareFileRow
			var matchMethodStr string
			var metaBytes []byte
			if err := rows.Scan(
				&row.ID, &row.FileID, &row.OrganizationID, &row.RowNumber,
				&row.RawName, &row.NormalizedName, &row.SKU,
				&row.Price, &row.Discount, &row.PriceAfterDiscount,
				&row.MatchedProductID, &row.MatchConfidence, &matchMethodStr,
				&metaBytes, &row.CreatedAt,
			); err != nil {
				return err
			}
			row.MatchMethod = compare.MatchMethod(matchMethodStr)
			if len(metaBytes) > 0 {
				_ = json.Unmarshal(metaBytes, &row.Meta)
			}
			list = append(list, &row)
		}
		return rows.Err()
	})
	return list, total, err
}

func (r *Repository) DeleteFileRows(ctx context.Context, fileID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `DELETE FROM compare.file_rows WHERE file_id = $1;`, fileID)
		return err
	})
}

func (r *Repository) DeleteFileRow(ctx context.Context, rowID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var fileID int64
		err := tx.QueryRow(txCtx, `DELETE FROM compare.file_rows WHERE id = $1 RETURNING file_id;`, rowID).Scan(&fileID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(txCtx, `
			UPDATE compare.files
			SET row_count = (SELECT COUNT(*) FROM compare.file_rows WHERE file_id = $1),
			    updated_at = NOW()
			WHERE id = $1;
		`, fileID)
		return err
	})
}

func (r *Repository) DeleteFileRowOwnedBy(ctx context.Context, rowID int64, ownerUserID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var fileID int64
		err := tx.QueryRow(txCtx, `
			DELETE FROM compare.file_rows r
			USING compare.files f
			WHERE r.id = $1 AND r.file_id = f.id AND f.user_id = $2
			RETURNING r.file_id;`, rowID, ownerUserID).Scan(&fileID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("compare file row")
			}
			return err
		}
		_, err = tx.Exec(txCtx, `
			UPDATE compare.files
			SET row_count = (SELECT COUNT(*) FROM compare.file_rows WHERE file_id = $1),
			    updated_at = NOW()
			WHERE id = $1;`, fileID)
		return err
	})
}

func (r *Repository) UpdateFileRowMatch(ctx context.Context, rowID int64, matchedProductID *int64, method compare.MatchMethod, confidence float64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE compare.file_rows
			SET matched_product_id = $1,
			    match_method = $2,
			    match_confidence = $3
			WHERE id = $4;
		`
		dbMethod := toDBMatchMethod(method)
		_, err := tx.Exec(txCtx, query, matchedProductID, dbMethod, confidence, rowID)
		return err
	})
}

func (r *Repository) SaveCustomerProductMapping(ctx context.Context, orgID *int64, rawName string, productID int64, source string) error {
	normName := strings.ToLower(strings.TrimSpace(rawName))
	decKey := "manual:" + normName

	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var effectiveOrgID *int64
		if orgID != nil && *orgID > 0 {
			effectiveOrgID = orgID
		} else if actor, ok := authctx.From(ctx); ok && actor.OrganizationID > 0 {
			effectiveOrgID = &actor.OrganizationID
		}

		if effectiveOrgID == nil {
			return nil
		}

		// 1. Insert into catalog.customer_product_mappings
		query := `
			INSERT INTO catalog.customer_product_mappings (
				organization_id, customer_org_id, product_id, raw_name, source, status, is_active, created_at, updated_at
			) VALUES ($1, $1, $2, $3, $4, 'processed', true, now(), now())
			ON CONFLICT DO NOTHING;
		`
		if _, err := tx.Exec(txCtx, query, *effectiveOrgID, productID, rawName, source); err != nil {
			return err
		}

		// 2. Insert or update in catalog.match_decisions
		var userID *int64
		if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 {
			userID = &actor.UserID
		}

		if normName != "" && productID > 0 {
			_, err := tx.Exec(txCtx, `
				INSERT INTO catalog.match_decisions (
					organization_id, user_id, decision_key, norm_name, chosen_product_id,
					confidence, reason, prompt_version, hit_count, created_at, last_used_at
				) VALUES (
					$1, $2, $3, $4, $5,
					1.000, 'قرار تصحيح يدوي من أداة المقارنة', 'manual:v1', 1, now(), now()
				)
				ON CONFLICT (COALESCE(organization_id, 0), decision_key)
				DO UPDATE SET
					chosen_product_id = EXCLUDED.chosen_product_id,
					confidence = 1.000,
					user_id = COALESCE(EXCLUDED.user_id, catalog.match_decisions.user_id),
					hit_count = catalog.match_decisions.hit_count + 1,
					last_used_at = now();
			`, *effectiveOrgID, userID, decKey, normName, productID)
			return err
		}

		return nil
	})
}

func (r *Repository) GetSavedProductMapping(ctx context.Context, orgID *int64, rawName string) (*int64, error) {
	var targetOrgID int64
	if orgID != nil && *orgID > 0 {
		targetOrgID = *orgID
	} else if actor, ok := authctx.From(ctx); ok && actor.OrganizationID > 0 {
		targetOrgID = actor.OrganizationID
	}

	if targetOrgID <= 0 {
		return nil, nil
	}

	var productID int64
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT product_id
			FROM catalog.customer_product_mappings
			WHERE raw_name = $1 AND is_active = true AND status = 'processed'
			  AND (organization_id = $2 OR customer_org_id = $2)
			ORDER BY updated_at DESC, id DESC
			LIMIT 1;
		`
		return tx.QueryRow(txCtx, query, rawName, targetOrgID).Scan(&productID)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &productID, nil
}
