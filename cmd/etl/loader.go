package main

import (
	"context"
	"database/sql"
	"fmt"
)

// Loader handles transactional data loading into PostgreSQL.
type Loader struct {
	db *sql.DB
}

// NewLoader creates a new PostgreSQL data loader.
func NewLoader(db *sql.DB) *Loader {
	return &Loader{db: db}
}

// LoadUsers inserts transformed users into identity.users.
func (l *Loader) LoadUsers(ctx context.Context, users []*TargetUser) (int, error) {
	if len(users) == 0 {
		return 0, nil
	}

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin load tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO identity.users (id, public_id, email, password_hash, name, role, status, language, phone, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			email = EXCLUDED.email,
			password_hash = EXCLUDED.password_hash,
			name = EXCLUDED.name,
			updated_at = EXCLUDED.updated_at;
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare user load stmt: %w", err)
	}
	defer stmt.Close()

	loaded := 0
	for _, u := range users {
		nameJSON := fmt.Sprintf(`{"ar":"%s","en":"%s"}`, u.Name["ar"], u.Name["en"])
		_, err := stmt.ExecContext(ctx,
			u.ID, u.PublicID, u.Email, u.PasswordHash, nameJSON,
			u.Role, u.Status, u.Language, u.Phone, u.CreatedAt, u.UpdatedAt,
		)
		if err != nil {
			return loaded, fmt.Errorf("insert user %d: %w", u.ID, err)
		}
		loaded++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit user load: %w", err)
	}
	return loaded, nil
}

// LoadOrganizations inserts transformed organizations into org.organizations.
func (l *Loader) LoadOrganizations(ctx context.Context, orgs []*TargetOrg) (int, error) {
	if len(orgs) == 0 {
		return 0, nil
	}

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin load tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO org.organizations (id, public_id, name, tax_number, phone, type, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			tax_number = EXCLUDED.tax_number,
			phone = EXCLUDED.phone,
			updated_at = EXCLUDED.updated_at;
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare org load stmt: %w", err)
	}
	defer stmt.Close()

	loaded := 0
	for _, o := range orgs {
		nameJSON := fmt.Sprintf(`{"ar":"%s","en":"%s"}`, o.LegalName["ar"], o.LegalName["en"])
		_, err := stmt.ExecContext(ctx,
			o.ID, o.PublicID, nameJSON,
			o.TaxNumber, o.Phone, o.Type, o.Status,
			o.CreatedAt, o.UpdatedAt,
		)
		if err != nil {
			return loaded, fmt.Errorf("insert org %d: %w", o.ID, err)
		}
		loaded++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit org load: %w", err)
	}
	return loaded, nil
}

// LoadProducts inserts transformed products and variants into catalog.products and catalog.product_variants.
func (l *Loader) LoadProducts(ctx context.Context, products []*TargetProduct, variants []*TargetVariant) (int, error) {
	if len(products) == 0 {
		return 0, nil
	}

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin load tx: %w", err)
	}
	defer tx.Rollback()

	prodStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO catalog.products (id, public_id, category_id, name, description, dosage_form, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, 0), $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			updated_at = EXCLUDED.updated_at;
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare product load stmt: %w", err)
	}
	defer prodStmt.Close()

	varStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO catalog.product_variants (id, public_id, product_id, sku, price, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			sku = EXCLUDED.sku,
			price = EXCLUDED.price,
			updated_at = EXCLUDED.updated_at;
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare variant load stmt: %w", err)
	}
	defer varStmt.Close()

	loaded := 0
	for i, p := range products {
		nameJSON := fmt.Sprintf(`{"ar":"%s","en":"%s"}`, p.Name["ar"], p.Name["en"])
		descJSON := fmt.Sprintf(`{"ar":"%s","en":"%s"}`, p.Description["ar"], p.Description["en"])
		_, err := prodStmt.ExecContext(ctx,
			p.ID, p.PublicID, p.CategoryID, nameJSON, p.Slug, descJSON,
			p.DosageForm, p.RequiresPrescription, p.CreatedAt, p.UpdatedAt,
		)
		if err != nil {
			return loaded, fmt.Errorf("insert product %d: %w", p.ID, err)
		}

		if i < len(variants) && variants[i] != nil {
			v := variants[i]
			_, err := varStmt.ExecContext(ctx,
				v.ID, v.PublicID, v.ProductID, v.SKU, v.Price.String(),
				v.Stock, v.CreatedAt, v.UpdatedAt,
			)
			if err != nil {
				return loaded, fmt.Errorf("insert variant %d: %w", v.ID, err)
			}
		}
		loaded++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit product load: %w", err)
	}
	return loaded, nil
}
