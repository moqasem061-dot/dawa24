package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Extractor handles streaming data extraction from legacy MariaDB.
type Extractor struct {
	db *sql.DB
}

// NewExtractor creates a new MariaDB data extractor.
func NewExtractor(db *sql.DB) *Extractor {
	return &Extractor{db: db}
}

// ExtractUsers streams user rows from MariaDB.
func (e *Extractor) ExtractUsers(ctx context.Context, batchSize int, offset int) ([]*SourceUser, error) {
	query := `SELECT id, name, email, password, COALESCE(phone, ''), COALESCE(role, 'customer'), created_at FROM users ORDER BY id ASC LIMIT ? OFFSET ?;`
	rows, err := e.db.QueryContext(ctx, query, batchSize, offset)
	if err != nil {
		return nil, fmt.Errorf("extract users query: %w", err)
	}
	defer rows.Close()

	var list []*SourceUser
	for rows.Next() {
		var u SourceUser
		var createdAt *time.Time
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Password, &u.Phone, &u.Role, &createdAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		if createdAt != nil {
			u.CreatedAt = *createdAt
		} else {
			u.CreatedAt = time.Now().UTC()
		}
		list = append(list, &u)
	}
	return list, rows.Err()
}

// ExtractProducts streams product rows from MariaDB.
func (e *Extractor) ExtractProducts(ctx context.Context, batchSize int, offset int) ([]*SourceProduct, error) {
	query := `SELECT id, COALESCE(name_ar, name, ''), COALESCE(name_en, name, ''), COALESCE(slug, ''), COALESCE(description, ''), COALESCE(category_id, 0), COALESCE(price, 0), COALESCE(stock, 0), COALESCE(supplier_id, 0), created_at FROM products ORDER BY id ASC LIMIT ? OFFSET ?;`
	rows, err := e.db.QueryContext(ctx, query, batchSize, offset)
	if err != nil {
		return nil, fmt.Errorf("extract products query: %w", err)
	}
	defer rows.Close()

	var list []*SourceProduct
	for rows.Next() {
		var p SourceProduct
		var createdAt *time.Time
		if err := rows.Scan(&p.ID, &p.NameAr, &p.NameEn, &p.Slug, &p.Description, &p.CategoryID, &p.Price, &p.Stock, &p.VendorOrgID, &createdAt); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		if createdAt != nil {
			p.CreatedAt = *createdAt
		} else {
			p.CreatedAt = time.Now().UTC()
		}
		list = append(list, &p)
	}
	return list, rows.Err()
}

// ExtractOrganizations streams suppliers/organizations from MariaDB.
func (e *Extractor) ExtractOrganizations(ctx context.Context, batchSize int, offset int) ([]*SourceOrg, error) {
	query := `SELECT id, name, COALESCE(tax_number, ''), COALESCE(commercial_register, ''), COALESCE(phone, ''), COALESCE(status, 'active'), created_at FROM suppliers ORDER BY id ASC LIMIT ? OFFSET ?;`
	rows, err := e.db.QueryContext(ctx, query, batchSize, offset)
	if err != nil {
		// Fallback to organizations table if suppliers doesn't exist
		return nil, err
	}
	defer rows.Close()

	var list []*SourceOrg
	for rows.Next() {
		var o SourceOrg
		var createdAt *time.Time
		if err := rows.Scan(&o.ID, &o.Name, &o.TaxNumber, &o.CommercialRegister, &o.Phone, &o.Status, &createdAt); err != nil {
			return nil, fmt.Errorf("scan supplier: %w", err)
		}
		if createdAt != nil {
			o.CreatedAt = *createdAt
		} else {
			o.CreatedAt = time.Now().UTC()
		}
		list = append(list, &o)
	}
	return list, rows.Err()
}
