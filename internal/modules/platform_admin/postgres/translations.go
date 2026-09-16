package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ListTranslations returns paginated translations matching filter criteria.
func (r *Repository) ListTranslations(ctx context.Context, filter platformadmin.TranslationFilter) ([]*platformadmin.Translation, int, error) {
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 50
	}

	where := []string{"1=1"}
	args := []any{}
	argIdx := 1

	if filter.Namespace != "" && filter.Namespace != "all" {
		where = append(where, fmt.Sprintf("namespace = $%d", argIdx))
		args = append(args, filter.Namespace)
		argIdx++
	}

	if filter.Custom != nil {
		where = append(where, fmt.Sprintf("is_custom = $%d", argIdx))
		args = append(args, *filter.Custom)
		argIdx++
	}

	if q := strings.TrimSpace(filter.Query); q != "" {
		where = append(where, fmt.Sprintf("(key ILIKE $%d OR text_ar ILIKE $%d OR text_en ILIKE $%d OR description ILIKE $%d)", argIdx, argIdx, argIdx, argIdx))
		args = append(args, "%"+q+"%")
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")
	var list []*platformadmin.Translation
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM platform.translations WHERE %s`, whereClause)
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return fmt.Errorf("platformadmin postgres: count translations: %w", err)
		}

		query := fmt.Sprintf(`
			SELECT id, key, namespace, text_ar, text_en, description, is_custom, created_at, updated_at
			FROM platform.translations
			WHERE %s
			ORDER BY namespace ASC, key ASC
			LIMIT $%d OFFSET $%d
		`, whereClause, argIdx, argIdx+1)

		pageArgs := append(append([]any{}, args...), filter.Limit, filter.Offset)
		rows, err := tx.Query(txCtx, query, pageArgs...)
		if err != nil {
			return fmt.Errorf("platformadmin postgres: list translations: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var t platformadmin.Translation
			if err := rows.Scan(
				&t.ID, &t.Key, &t.Namespace, &t.TextAR, &t.TextEN,
				&t.Description, &t.IsCustom, &t.CreatedAt, &t.UpdatedAt,
			); err != nil {
				return fmt.Errorf("platformadmin postgres: scan translation: %w", err)
			}
			list = append(list, &t)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// GetTranslationByKey returns a translation by its unique key.
func (r *Repository) GetTranslationByKey(ctx context.Context, key string) (*platformadmin.Translation, error) {
	var t platformadmin.Translation
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, key, namespace, text_ar, text_en, description, is_custom, created_at, updated_at
			FROM platform.translations
			WHERE key = $1
		`
		err := tx.QueryRow(txCtx, query, key).Scan(
			&t.ID, &t.Key, &t.Namespace, &t.TextAR, &t.TextEN,
			&t.Description, &t.IsCustom, &t.CreatedAt, &t.UpdatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("platformadmin postgres: get translation: %w", err)
	}
	if t.ID == 0 {
		return nil, nil
	}
	return &t, nil
}

// UpsertTranslation creates or updates a translation row.
func (r *Repository) UpsertTranslation(ctx context.Context, t *platformadmin.Translation) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO platform.translations (
				key, namespace, text_ar, text_en, description, is_custom, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, now())
			ON CONFLICT (key) DO UPDATE SET
				text_ar = EXCLUDED.text_ar,
				text_en = EXCLUDED.text_en,
				namespace = EXCLUDED.namespace,
				description = EXCLUDED.description,
				is_custom = EXCLUDED.is_custom,
				updated_at = now()
			RETURNING id, created_at, updated_at
		`
		return tx.QueryRow(
			txCtx, query,
			t.Key, t.Namespace, t.TextAR, t.TextEN, t.Description, t.IsCustom,
		).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	})
}

// DeleteTranslation removes a custom translation row from database.
func (r *Repository) DeleteTranslation(ctx context.Context, key string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `DELETE FROM platform.translations WHERE key = $1`, key)
		return err
	})
}

// GetTranslationStats computes total key, custom overrides, and namespace counts.
func (r *Repository) GetTranslationStats(ctx context.Context) (*platformadmin.TranslationStats, error) {
	var s platformadmin.TranslationStats
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT 
				COUNT(*),
				COUNT(*) FILTER (WHERE is_custom = true),
				COUNT(DISTINCT namespace)
			FROM platform.translations
		`
		return tx.QueryRow(txCtx, query).Scan(
			&s.TotalKeys,
			&s.CustomOverrides,
			&s.TotalNamespaces,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("platformadmin postgres: get translation stats: %w", err)
	}
	return &s, nil
}

// LoadAllCustomTranslations returns all active custom overrides for memory cache synchronization.
func (r *Repository) LoadAllCustomTranslations(ctx context.Context) (map[string]i18n.Text, error) {
	out := make(map[string]i18n.Text)
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT key, text_ar, text_en
			FROM platform.translations
			WHERE is_custom = true
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var k, ar, en string
			if err := rows.Scan(&k, &ar, &en); err != nil {
				return err
			}
			out[k] = i18n.New(ar, en)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("platformadmin postgres: load custom translations: %w", err)
	}
	return out, nil
}
