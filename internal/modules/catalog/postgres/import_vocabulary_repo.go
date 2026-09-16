package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ImportVocabulary reads the taxonomy an enrichment pass must choose within.
//
// Handing the model the platform's own categories and brands by id is what stops
// an import from fragmenting the taxonomy: it classifies into what exists rather
// than inventing a fifth spelling of i18n.TDefault("w4_mod.s_267_267").
func (r *Repository) ImportVocabulary(ctx context.Context, orgID int64) (catalog.EnrichVocabulary, error) {
	var vocab catalog.EnrichVocabulary

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var err error
		if vocab.Categories, err = loadCategoryOptions(txCtx, tx); err != nil {
			return err
		}
		if vocab.Brands, err = loadBrandOptions(txCtx, tx); err != nil {
			return err
		}
		vocab.DosageForms, err = loadDosageForms(txCtx, tx)
		return err
	})
	if err != nil {
		return catalog.EnrichVocabulary{}, err
	}
	return vocab, nil
}

func loadCategoryOptions(ctx context.Context, tx pgx.Tx) ([]catalog.TaxonomyOption, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, COALESCE(name->>'ar', name->>'en', '')
		FROM catalog.categories
		WHERE deleted_at IS NULL AND status = 'active'
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: load category vocabulary: %w", err)
	}
	defer rows.Close()

	var out []catalog.TaxonomyOption
	for rows.Next() {
		var opt catalog.TaxonomyOption
		if err := rows.Scan(&opt.ID, &opt.Name); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan category: %w", err)
		}
		if opt.Name != "" {
			out = append(out, opt)
		}
	}
	return out, rows.Err()
}

// loadBrandOptions orders brands by how much of the catalogue already uses
// them, so the truncation that keeps the prompt bounded drops the long tail
// rather than an arbitrary slice.
func loadBrandOptions(ctx context.Context, tx pgx.Tx) ([]catalog.TaxonomyOption, error) {
	rows, err := tx.Query(ctx, `
		SELECT b.id, COALESCE(b.name->>'ar', b.name->>'en', ''), count(p.id) AS uses
		FROM catalog.brands b
		LEFT JOIN catalog.products p ON p.brand_id = b.id AND p.deleted_at IS NULL
		WHERE b.deleted_at IS NULL
		GROUP BY b.id, b.name
		ORDER BY uses DESC, b.id
	`)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: load brand vocabulary: %w", err)
	}
	defer rows.Close()

	var out []catalog.TaxonomyOption
	for rows.Next() {
		var opt catalog.TaxonomyOption
		var uses int64
		if err := rows.Scan(&opt.ID, &opt.Name, &uses); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan brand: %w", err)
		}
		if opt.Name != "" {
			out = append(out, opt)
		}
	}
	return out, rows.Err()
}

func loadDosageForms(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT dosage_form
		FROM catalog.products
		WHERE deleted_at IS NULL AND btrim(dosage_form) <> ''
		GROUP BY dosage_form
		ORDER BY count(*) DESC, dosage_form ASC
		LIMIT 100
	`)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: load dosage form vocabulary: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var form string
		if err := rows.Scan(&form); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan dosage form: %w", err)
		}
		out = append(out, form)
	}
	return out, rows.Err()
}

// ListDosageForms returns distinct pharmaceutical dosage forms stored in the catalog.
func (r *Repository) ListDosageForms(ctx context.Context) ([]string, error) {
	var out []string
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var err error
		out, err = loadDosageForms(txCtx, tx)
		return err
	})
	return out, err
}

// ListRecentImportSessions backs the import history panel.
func (r *Repository) ListRecentImportSessions(ctx context.Context, orgID int64, limit int) ([]*catalog.ImportSession, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var out []*catalog.ImportSession

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		cursor, err := tx.Query(txCtx,
			`SELECT `+sessionColumns+`
			 FROM catalog.import_sessions s
			 WHERE s.organization_id = $1
			 ORDER BY s.created_at DESC
			 LIMIT $2`, orgID, limit)
		if err != nil {
			return fmt.Errorf("catalog postgres: list import sessions: %w", err)
		}
		defer cursor.Close()

		for cursor.Next() {
			s, err := scanImportSession(cursor)
			if err != nil {
				return err
			}
			out = append(out, s)
		}
		return cursor.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface{ Scan(dest ...any) error }

func scanImportSession(row rowScanner) (*catalog.ImportSession, error) {
	var s catalog.ImportSession
	var status, mode, phase string
	var options, overrides, brands, categories, structure []byte
	var progressAt *time.Time

	err := row.Scan(
		&s.ID, &s.PublicID, &s.OrganizationID, &s.CreatedBy,
		&s.Filename, &s.FileSizeBytes, &s.SourceFormat, &s.SheetName, &s.Delimiter,
		&status, &mode, &options, &overrides,
		&s.TotalRows, &s.ParsedRows, &s.InsertRows, &s.UpdateRows, &s.SkipRows,
		&s.ErrorRows, &s.WarningRows, &s.BlockCount, &brands, &categories,
		&s.AICalls, &s.AIApplied, &s.AIMatched, &s.AINote, &s.AIFallback,
		&s.ErrorMessage, &s.CreatedAt, &s.UpdatedAt, &s.CommittedAt, &s.ExpiresAt,
		&structure, &phase, &s.Progress.Current, &s.Progress.Total, &progressAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("import_session")
	}
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: scan import session: %w", err)
	}

	s.Status = catalog.SessionStatus(status)
	s.Mode = catalog.ImportMode(mode)
	if err := json.Unmarshal(options, &s.Options); err != nil {
		return nil, fmt.Errorf("catalog postgres: decode import options: %w", err)
	}
	if err := json.Unmarshal(overrides, &s.Overrides); err != nil {
		return nil, fmt.Errorf("catalog postgres: decode layout overrides: %w", err)
	}
	if err := json.Unmarshal(brands, &s.NewBrands); err != nil {
		return nil, fmt.Errorf("catalog postgres: decode new brands: %w", err)
	}
	if err := json.Unmarshal(categories, &s.NewCategories); err != nil {
		return nil, fmt.Errorf("catalog postgres: decode new categories: %w", err)
	}
	// The structure document is advisory: a session written before the column
	// existed decodes to the zero value, and the screens fall back to
	// re-reading the file rather than refusing to render.
	if len(structure) > 0 {
		if err := json.Unmarshal(structure, &s.Structure); err != nil {
			return nil, fmt.Errorf("catalog postgres: decode import structure: %w", err)
		}
	}
	s.Progress.Phase = catalog.ImportPhase(phase)
	s.Progress.Message = s.Progress.Phase.Label()
	if progressAt != nil {
		s.Progress.UpdatedAt = *progressAt
		s.Progress.StartedAt = *progressAt
	}
	return &s, nil
}

// stagingRowSelect is the one column list every staging-row read uses.
//
// It is shared rather than repeated because it drifted the moment it was not:
// two of the three queries grew the matched-product join and the third did not,
// and the mismatch surfaced only at commit time as "number of field
// descriptions must equal number of destinations, got 11 and 13" — after the
// admin had reviewed nine thousand rows and pressed save. scanStagingRow below
// is the other half of this contract; the two change together or not at all.
//
// The matched product's name comes from the join rather than from a copy taken
// at staging time. An import is reviewed minutes or hours after it was
// prepared, and a cached name would disagree with the product the admin opens
// to check it against — which is worse than showing no name.
const stagingRowSelect = `
	SELECT r.id, r.session_id, r.source_row, r.block_index, r.action, r.included,
	       r.matched_product_id, r.match_reason, r.payload, r.issues, r.ai_changes,
	       COALESCE(NULLIF(p.name->>'ar', ''), NULLIF(p.name->>'en', ''), '') AS matched_name,
	       COALESCE(p.sku, '') AS matched_sku
	FROM catalog.import_staging_rows r
	LEFT JOIN catalog.products p ON p.id = r.matched_product_id
`

func scanStagingRow(cursor pgx.Rows) (*catalog.StagingRow, error) {
	var row catalog.StagingRow
	var action, matchReason string
	var payload, issues, changes []byte

	if err := cursor.Scan(
		&row.ID, &row.SessionID, &row.SourceRow, &row.Block, &action, &row.Included,
		&row.MatchedProductID, &matchReason, &payload, &issues, &changes,
		&row.MatchedProductName, &row.MatchedProductSKU,
	); err != nil {
		return nil, fmt.Errorf("catalog postgres: scan staging row: %w", err)
	}

	row.Action = catalog.RowAction(action)
	row.MatchReason = catalog.MatchReason(matchReason)

	var product catalog.Product
	if err := json.Unmarshal(payload, &product); err != nil {
		return nil, fmt.Errorf("catalog postgres: decode staged product (row %d): %w", row.SourceRow, err)
	}
	// A product decoded from JSON has a nil Name map when the file gave no name
	// at all; downstream code writes into it, so it must never be nil.
	if product.Name == nil {
		product.Name = i18n.Text{}
	}
	row.Product = &product

	if err := json.Unmarshal(issues, &row.Issues); err != nil {
		return nil, fmt.Errorf("catalog postgres: decode staged issues (row %d): %w", row.SourceRow, err)
	}
	if err := json.Unmarshal(changes, &row.AIChanges); err != nil {
		return nil, fmt.Errorf("catalog postgres: decode staged ai changes (row %d): %w", row.SourceRow, err)
	}
	return &row, nil
}

// qualify prefixes the staging-row filter clause with the table alias the
// listing query joins under. The clause is built from a fixed set of column
// names in ListStagingRows, never from input.
func qualify(clause string) string {
	for _, col := range []string{
		"session_id", "action", "has_error", "has_warning", "has_ai",
		"search_name", "search_code",
	} {
		clause = strings.ReplaceAll(clause, col, "r."+col)
	}
	return clause
}

// The JSONB columns are NOT NULL, so a nil Go slice must encode as [] and not
// as the JSON literal null.
func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func nonNilIssues(in []catalog.RowIssue) []catalog.RowIssue {
	if in == nil {
		return []catalog.RowIssue{}
	}
	return in
}

func nonNilChanges(in []catalog.AIChange) []catalog.AIChange {
	if in == nil {
		return []catalog.AIChange{}
	}
	return in
}
