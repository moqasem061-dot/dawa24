package postgres

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Repository implements org.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a new organization repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// SetExtraDevices updates the temporary extra connected devices quota and expiry for an organization.
func (r *Repository) SetExtraDevices(ctx context.Context, orgID int64, extraDevices int, expiresAt *time.Time) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE org.organizations
			SET extra_devices = $2,
			    extra_devices_expires_at = $3,
			    updated_at = now()
			WHERE id = $1;
		`
		tag, err := tx.Exec(txCtx, query, orgID, extraDevices, expiresAt)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("organization")
		}
		return nil
	})
}

// CreateOrganization inserts a new organization record.
func (r *Repository) CreateOrganization(ctx context.Context, o *org.Organization) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.organizations (
				name, legal_name, trade_name, tax_number, commercial_register, type, status, credit_limit, payment_terms_days
			) VALUES (
				jsonb_build_object('ar', $1::text, 'en', $1::text),
				$1, COALESCE($2, jsonb_build_object('ar', $1::text, 'en', $1::text)),
				$3, $4, $5, $6, $7, $8
			)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			o.LegalName, o.TradeName, o.TaxNumber, o.CommercialRegister,
			string(o.Type), string(o.Status), o.CreditLimit, o.PaymentTermsDays,
		).Scan(&o.ID, &o.PublicID, &o.CreatedAt, &o.UpdatedAt)
	})
}

// GetOrganizationByID retrieves an organization by ID.
func (r *Repository) GetOrganizationByID(ctx context.Context, id int64) (*org.Organization, error) {
	var o org.Organization
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, legal_name, trade_name, tax_number, commercial_register,
			       COALESCE(pharmacist_license, ''),
			       COALESCE(verification_notes, ''), COALESCE(rejection_reason, ''),
			       COALESCE(owner_id, 0), approved_at, approved_by,
			       COALESCE(ai_virtual_key, ''), COALESCE(ai_user_id, ''),
			       COALESCE(name, '{}'::jsonb), COALESCE(email, ''),
			       COALESCE(phone, ''), COALESCE(address, ''),
			       type, status, credit_limit, payment_terms_days, created_at, updated_at,
			       COALESCE(extra_devices, 0), extra_devices_expires_at
			FROM org.organizations
			WHERE id = $1;
		`
		var typeStr, statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&o.ID, &o.PublicID, &o.LegalName, &o.TradeName, &o.TaxNumber, &o.CommercialRegister,
			&o.PharmacistLicense, &o.VerificationNotes, &o.RejectionReason,
			&o.OwnerID, &o.ApprovedAt, &o.ApprovedBy,
			&o.AIVirtualKey, &o.AIUserID,
			&o.Name, &o.Email, &o.Phone, &o.Address,
			&typeStr, &statusStr, &o.CreditLimit, &o.PaymentTermsDays, &o.CreatedAt, &o.UpdatedAt,
			&o.ExtraDevices, &o.ExtraDevicesExpiresAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("organization")
			}
			return err
		}
		o.Type = org.OrganizationType(typeStr)
		o.Status = org.OrganizationStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetSupplierProfile retrieves full commercial details of a supplier/vendor organization.
func (r *Repository) GetSupplierProfile(ctx context.Context, id int64) (*org.SupplierOrgProfile, error) {
	var p org.SupplierOrgProfile
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT 
				id, public_id,
				COALESCE(name->>'ar', name->>'en', ''),
				COALESCE(name->>'en', name->>'ar', ''),
				COALESCE(type, 'company'),
				COALESCE(min_order_price, 10.00),
				COALESCE(max_order_price, 50.00),
				COALESCE(organization_number, ''),
				COALESCE(email, ''),
				COALESCE(phone, ''),
				COALESCE(tax_number, ''),
				COALESCE(address, ''),
				COALESCE(description->>'ar', ''),
				COALESCE(description->>'en', ''),
				COALESCE(image, ''),
				COALESCE(coverage_image, ''),
				COALESCE(status, 'approved'),
				COALESCE(rating, 5),
				created_at, updated_at
			FROM org.organizations
			WHERE id = $1 AND deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&p.ID, &p.PublicID, &p.NameAr, &p.NameEn, &p.Type,
			&p.MinOrderPrice, &p.MaxOrderPrice,
			&p.OrganizationNumber, &p.Email, &p.Phone, &p.TaxNumber, &p.Address,
			&p.DescriptionAr, &p.DescriptionEn,
			&p.Image, &p.CoverageImage,
			&p.Status, &p.Rating,
			&p.CreatedAt, &p.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("organization")
		}
		return nil, err
	}
	return &p, nil
}

// UpdateSupplierProfile updates the commercial details and order price limits of a supplier/vendor organization.
func (r *Repository) UpdateSupplierProfile(ctx context.Context, p *org.SupplierOrgProfile) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE org.organizations
			SET 
				name = jsonb_build_object('ar', $1::text, 'en', $2::text),
				type = $3,
				min_order_price = $4,
				max_order_price = $5,
				organization_number = $6,
				email = $7,
				phone = $8,
				tax_number = $9,
				address = $10,
				description = jsonb_build_object('ar', $11::text, 'en', $12::text),
				image = CASE WHEN $13::text <> '' THEN $13::text ELSE image END,
				coverage_image = CASE WHEN $14::text <> '' THEN $14::text ELSE coverage_image END,
				updated_at = now()
			WHERE id = $15 AND deleted_at IS NULL;
		`
		tag, err := tx.Exec(txCtx, query,
			p.NameAr, p.NameEn, p.Type,
			p.MinOrderPrice, p.MaxOrderPrice,
			p.OrganizationNumber, p.Email, p.Phone, p.TaxNumber, p.Address,
			p.DescriptionAr, p.DescriptionEn,
			p.Image, p.CoverageImage,
			p.ID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("organization")
		}
		return nil
	})
}

// UpdateOrganizationStatus modifies organization approval state.
func (r *Repository) UpdateOrganizationStatus(ctx context.Context, id int64, status org.OrganizationStatus) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var beforeStatus string
		_ = tx.QueryRow(txCtx, `SELECT status FROM org.organizations WHERE id = $1;`, id).Scan(&beforeStatus)

		query := `UPDATE org.organizations SET status = $1, approved_at = CASE WHEN $1 = 'approved' THEN NOW() ELSE approved_at END, updated_at = now() WHERE id = $2;`
		tag, err := tx.Exec(txCtx, query, string(status), id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("organization")
		}

		if status == org.StatusApproved {
			_, _ = tx.Exec(txCtx, `
				UPDATE identity.users u
				SET status = 'active', updated_at = now()
				FROM org.members m
				WHERE m.user_id = u.id AND m.organization_id = $1 AND u.status != 'active';
			`, id)

			_, _ = tx.Exec(txCtx, `
				UPDATE org.members
				SET status = 'active', is_active = true, updated_at = now()
				WHERE organization_id = $1 AND (status != 'active' OR is_active = false);
			`, id)

			_, _ = tx.Exec(txCtx, `
				UPDATE platform_admin.documents
				SET status = 'verified', updated_at = now()
				WHERE organization_id = $1 AND status = 'pending';
			`, id)

			_, _ = tx.Exec(txCtx, `SELECT identity.bump_rbac_version($1);`, fmt.Sprintf("org:%d", id))
		}

		action := "org." + string(status)
		if status == org.StatusApproved {
			if beforeStatus == string(org.StatusSuspended) {
				action = "org.reactivate"
			} else {
				action = "org.approve"
			}
		} else if status == org.StatusRejected {
			action = "org.reject"
		} else if status == org.StatusSuspended {
			action = "org.suspend"
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &id,
			Action:         action,
			EntityType:     "organization",
			EntityID:       strconv.FormatInt(id, 10),
			Before:         map[string]any{"status": beforeStatus},
			After:          map[string]any{"status": string(status)},
		})

		return nil
	})
}

// UpdateOrganizationAICredentials saves the AI Gateway user ID and virtual API key for an organization.
func (r *Repository) UpdateOrganizationAICredentials(ctx context.Context, id int64, aiUserID, aiVirtualKey string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE org.organizations SET ai_user_id = $1, ai_virtual_key = $2, updated_at = now() WHERE id = $3;`
		tag, err := tx.Exec(txCtx, query, aiUserID, aiVirtualKey, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("organization")
		}
		return nil
	})
}

// ReviewOrganization updates the approval status, admin notes, rejection reason, and audit info.
func (r *Repository) ReviewOrganization(ctx context.Context, id int64, status org.OrganizationStatus, notes, rejectionReason string, adminID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var beforeStatus string
		_ = tx.QueryRow(txCtx, `SELECT status FROM org.organizations WHERE id = $1;`, id).Scan(&beforeStatus)

		var approvedAt *string
		var adminPtr *int64
		if adminID > 0 {
			adminPtr = &adminID
		}
		query := `
			UPDATE org.organizations 
			SET status = $1, 
			    verification_notes = $2, 
			    rejection_reason = $3,
			    approved_at = CASE WHEN $1 = 'approved' THEN NOW() ELSE approved_at END,
			    approved_by = CASE WHEN $1 = 'approved' THEN $4 ELSE approved_by END,
			    updated_at = NOW()
			WHERE id = $5;
		`
		_ = approvedAt
		tag, err := tx.Exec(txCtx, query, string(status), notes, rejectionReason, adminPtr, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("organization")
		}

		if status == org.StatusApproved {
			_, _ = tx.Exec(txCtx, `
				UPDATE identity.users u
				SET status = 'active', updated_at = now()
				FROM org.members m
				WHERE m.user_id = u.id AND m.organization_id = $1 AND u.status != 'active';
			`, id)

			_, _ = tx.Exec(txCtx, `
				UPDATE org.members
				SET status = 'active', is_active = true, updated_at = now()
				WHERE organization_id = $1 AND (status != 'active' OR is_active = false);
			`, id)

			_, _ = tx.Exec(txCtx, `
				UPDATE platform_admin.documents
				SET status = 'verified', updated_at = now()
				WHERE organization_id = $1 AND status = 'pending';
			`, id)

			_, _ = tx.Exec(txCtx, `SELECT identity.bump_rbac_version($1);`, fmt.Sprintf("org:%d", id))
		}

		action := "org." + string(status)
		if status == org.StatusApproved {
			if beforeStatus == string(org.StatusSuspended) {
				action = "org.reactivate"
			} else {
				action = "org.approve"
			}
		} else if status == org.StatusRejected {
			action = "org.reject"
		} else if status == org.StatusSuspended {
			action = "org.suspend"
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &id,
			ActorUserID:    adminID,
			Action:         action,
			EntityType:     "organization",
			EntityID:       strconv.FormatInt(id, 10),
			Before:         map[string]any{"status": beforeStatus},
			After: map[string]any{
				"status":           string(status),
				"notes":            notes,
				"rejection_reason": rejectionReason,
			},
		})

		return nil
	})
}
