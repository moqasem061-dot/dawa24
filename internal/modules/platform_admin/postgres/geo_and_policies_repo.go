package postgres

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ToggleCityStatus toggles the active state of a city in the database.
func (r *Repository) ToggleCityStatus(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var active bool
		err := tx.QueryRow(txCtx, `UPDATE platform_admin.cities SET is_active = NOT is_active WHERE id = $1 RETURNING is_active;`, id).Scan(&active)
		if err != nil {
			return err
		}
		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			Action:     "reference.city.toggle",
			EntityType: "city",
			EntityID:   strconv.FormatInt(id, 10),
			After:      map[string]any{"is_active": active},
		})
		return nil
	})
}

// CreateCity adds a new city / district into the database.
func (r *Repository) CreateCity(ctx context.Context, c *platformadmin.City) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if c.CountryID == 0 {
			c.CountryID = 1
		}
		query := `
			INSERT INTO platform_admin.cities (country_id, governorate_id, name, latitude, longitude, is_active, is_capital, coverage_radius_meters, time_zone)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'Africa/Cairo')
			RETURNING id;
		`
		err := tx.QueryRow(txCtx, query, c.CountryID, c.GovernorateID, c.Name, c.Latitude, c.Longitude, c.IsActive, c.IsCapital, c.NormalizedRadius()).Scan(&c.ID)
		if err != nil {
			return err
		}
		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			Action:     "reference.city.create",
			EntityType: "city",
			EntityID:   strconv.FormatInt(c.ID, 10),
			After:      map[string]any{"name": c.Name, "governorate_id": c.GovernorateID},
		})
		return nil
	})
}

// UpdateCity updates an existing city / district.
func (r *Repository) UpdateCity(ctx context.Context, c *platformadmin.City) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE platform_admin.cities
			SET governorate_id = $2, name = $3, latitude = $4, longitude = $5, is_active = $6, is_capital = $7,
			    coverage_radius_meters = $8
			WHERE id = $1;
		`
		tag, err := tx.Exec(txCtx, query, c.ID, c.GovernorateID, c.Name, c.Latitude, c.Longitude, c.IsActive, c.IsCapital, c.NormalizedRadius())
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("city")
		}
		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			Action:     "reference.city.edit",
			EntityType: "city",
			EntityID:   strconv.FormatInt(c.ID, 10),
			After:      map[string]any{"name": c.Name, "governorate_id": c.GovernorateID},
		})
		return nil
	})
}

// ListCurrencies returns supported currencies.
func (r *Repository) ListCurrencies(ctx context.Context) ([]*platformadmin.Currency, error) {
	var list []*platformadmin.Currency
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, code, name, symbol, exchange_rate_egp, is_active, is_default FROM platform_admin.currencies WHERE is_active = true ORDER BY id ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var cur platformadmin.Currency
			if err := rows.Scan(&cur.ID, &cur.Code, &cur.Name, &cur.Symbol, &cur.ExchangeRateEGP, &cur.IsActive, &cur.IsDefault); err != nil {
				return err
			}
			list = append(list, &cur)
		}
		return rows.Err()
	})
	return list, err
}

// ListLanguages returns supported languages.
func (r *Repository) ListLanguages(ctx context.Context) ([]*platformadmin.Language, error) {
	var list []*platformadmin.Language
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, code, name, dir, is_active, is_default FROM platform_admin.languages WHERE is_active = true ORDER BY id ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var l platformadmin.Language
			if err := rows.Scan(&l.ID, &l.Code, &l.Name, &l.Dir, &l.IsActive, &l.IsDefault); err != nil {
				return err
			}
			list = append(list, &l)
		}
		return rows.Err()
	})
	return list, err
}

// CreateContactMessage persists a contact inquiry.
func (r *Repository) CreateContactMessage(ctx context.Context, m *platformadmin.ContactMessage) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO platform_admin.contact_messages (name, email, phone, subject, message, status, ip, user_agent, user_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, public_id, created_at;
		`
		return tx.QueryRow(txCtx, query, m.Name, m.Email, m.Phone, m.Subject, m.Message, m.Status, m.IP, m.UserAgent, m.UserID).
			Scan(&m.ID, &m.PublicID, &m.CreatedAt)
	})
}

// ListContactMessages returns contact inquiries.
func (r *Repository) ListContactMessages(ctx context.Context, status string, limit, offset int) ([]*platformadmin.ContactMessage, error) {
	list, _, err := r.ListContactMessagesWithTotal(ctx, status, limit, offset)
	return list, err
}

// ListContactMessagesWithTotal returns contact inquiries with total count.
func (r *Repository) ListContactMessagesWithTotal(ctx context.Context, status string, limit, offset int) ([]*platformadmin.ContactMessage, int, error) {
	var list []*platformadmin.ContactMessage
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		countQ := `SELECT count(*) FROM platform_admin.contact_messages WHERE ($1 = '' OR status = $1);`
		if err := tx.QueryRow(txCtx, countQ, status).Scan(&total); err != nil {
			return err
		}

		query := `
			SELECT id, public_id, name, email, phone, subject, message, status, ip, user_agent, user_id, created_at
			FROM platform_admin.contact_messages
			WHERE ($1 = '' OR status = $1)
			ORDER BY created_at DESC, id DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, status, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m platformadmin.ContactMessage
			var phone *string
			if err := rows.Scan(&m.ID, &m.PublicID, &m.Name, &m.Email, &phone, &m.Subject, &m.Message, &m.Status, &m.IP, &m.UserAgent, &m.UserID, &m.CreatedAt); err != nil {
				return err
			}
			if phone != nil {
				m.Phone = *phone
			}
			list = append(list, &m)
		}
		return rows.Err()
	})
	return list, total, err
}

// UpdateContactMessageStatus updates the status of a contact message.
func (r *Repository) UpdateContactMessageStatus(ctx context.Context, id int64, status string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE platform_admin.contact_messages SET status = $1 WHERE id = $2;`
		_, err := tx.Exec(txCtx, query, status, id)
		return err
	})
}

// DeleteContactMessage removes a contact message.
func (r *Repository) DeleteContactMessage(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM platform_admin.contact_messages WHERE id = $1;`
		_, err := tx.Exec(txCtx, query, id)
		return err
	})
}

// ListPolicyVersions returns all versions for a policy key.
func (r *Repository) ListPolicyVersions(ctx context.Context, policyKey string) ([]*platformadmin.Policy, error) {
	var list []*platformadmin.Policy
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, policy_key, version, title, content, summary, is_published, published_at, created_by, created_at, updated_at
			FROM platform_admin.policies
			WHERE ($1 = '' OR policy_key = $1)
			ORDER BY policy_key ASC, is_published DESC, published_at DESC NULLS LAST, updated_at DESC, created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, policyKey)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var p platformadmin.Policy
			if err := rows.Scan(
				&p.ID, &p.PolicyKey, &p.Version, &p.Title, &p.Content, &p.Summary,
				&p.IsPublished, &p.PublishedAt, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &p)
		}
		return rows.Err()
	})
	return list, err
}

// GetPolicyVersion gets a specific version of a policy.
func (r *Repository) GetPolicyVersion(ctx context.Context, policyKey, version string) (*platformadmin.Policy, error) {
	var p platformadmin.Policy
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, policy_key, version, title, content, summary, is_published, published_at, created_by, created_at, updated_at
			FROM platform_admin.policies
			WHERE policy_key = $1 AND version = $2;
		`
		return tx.QueryRow(txCtx, query, policyKey, version).Scan(
			&p.ID, &p.PolicyKey, &p.Version, &p.Title, &p.Content, &p.Summary,
			&p.IsPublished, &p.PublishedAt, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("policy")
		}
		return nil, err
	}
	return &p, nil
}

// GetActivePolicy gets the currently published version of a policy.
func (r *Repository) GetActivePolicy(ctx context.Context, policyKey string) (*platformadmin.Policy, error) {
	var p platformadmin.Policy
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, policy_key, version, title, content, summary, is_published, published_at, created_by, created_at, updated_at
			FROM platform_admin.policies
			WHERE policy_key = $1 AND is_published = true
			ORDER BY published_at DESC NULLS LAST, updated_at DESC, created_at DESC
			LIMIT 1;
		`
		return tx.QueryRow(txCtx, query, policyKey).Scan(
			&p.ID, &p.PolicyKey, &p.Version, &p.Title, &p.Content, &p.Summary,
			&p.IsPublished, &p.PublishedAt, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("policy")
		}
		return nil, err
	}
	return &p, nil
}

// CreatePolicyVersion inserts a new draft or published policy version.
func (r *Repository) CreatePolicyVersion(ctx context.Context, p *platformadmin.Policy) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if p.IsPublished {
			// Unpublish previous versions for this key
			_, _ = tx.Exec(txCtx, `UPDATE platform_admin.policies SET is_published = false WHERE policy_key = $1;`, p.PolicyKey)
			if p.PublishedAt == nil {
				now := time.Now()
				p.PublishedAt = &now
			}
		}
		query := `
			INSERT INTO platform_admin.policies (
				policy_key, version, title, content, summary, is_published, published_at, created_by, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
			RETURNING id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			p.PolicyKey, p.Version, p.Title, p.Content, p.Summary, p.IsPublished, p.PublishedAt, p.CreatedBy,
		).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	})
}

// PublishPolicyVersion sets this version as published and unpublishes earlier versions.
func (r *Repository) PublishPolicyVersion(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var policyKey string
		err := tx.QueryRow(txCtx, `SELECT policy_key FROM platform_admin.policies WHERE id = $1;`, id).Scan(&policyKey)
		if err != nil {
			return err
		}

		// Unpublish previous versions
		_, err = tx.Exec(txCtx, `UPDATE platform_admin.policies SET is_published = false WHERE policy_key = $1;`, policyKey)
		if err != nil {
			return err
		}

		// Publish current version
		_, err = tx.Exec(txCtx, `
			UPDATE platform_admin.policies 
			SET is_published = true, published_at = NOW(), updated_at = NOW() 
			WHERE id = $1;
		`, id)
		return err
	})
}
