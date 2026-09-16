package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// UpdateOrganization updates organization profile fields.
func (r *Repository) UpdateOrganization(ctx context.Context, o *org.Organization) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// Ensure trade_name and name are never nil/empty in DB
		tradeName := o.TradeName
		if tradeName.IsEmpty() {
			tradeName = i18n.New(o.LegalName, o.LegalName)
		}
		name := o.Name
		if name.IsEmpty() {
			name = tradeName
		}

		// Ensure price range constraint is respected
		minPrice := o.MinOrderPrice
		maxPrice := o.MaxOrderPrice
		if maxPrice.Minor() < minPrice.Minor() {
			maxPrice = minPrice
		}

		orgType := string(o.Type)
		if orgType != string(org.TypeCustomer) && orgType != string(org.TypeVendor) {
			orgType = string(org.TypeVendor)
		}

		orgStatus := string(o.Status)
		if orgStatus == "" {
			orgStatus = string(org.StatusPending)
		}

		query := `
			UPDATE org.organizations
			SET legal_name = $1,
			    trade_name = $2,
			    name = $3,
			    type = $4,
			    status = $5,
			    tax_number = $6,
			    commercial_register = $7,
			    pharmacist_license = $8,
			    phone = $9,
			    email = $10,
			    address = $11,
			    credit_limit = $12,
			    payment_terms_days = $13,
			    min_order_price = $14,
			    max_order_price = $15,
			    verification_notes = $16,
			    updated_at = now()
			WHERE id = $17;
		`
		tag, err := tx.Exec(txCtx, query,
			o.LegalName, tradeName, name,
			orgType, orgStatus,
			o.TaxNumber, o.CommercialRegister, o.PharmacistLicense,
			o.Phone, o.Email, o.Address,
			o.CreditLimit, o.PaymentTermsDays,
			minPrice, maxPrice,
			o.VerificationNotes,
			o.ID,
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

// DeleteOrganization soft-deletes or suspends an organization.
func (r *Repository) DeleteOrganization(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `UPDATE org.organizations SET status = 'suspended', updated_at = now() WHERE id = $1;`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("organization")
		}
		return nil
	})
}

// UpdateMemberRole changes a member's role in the organization.

func (r *Repository) UpdateMemberRole(ctx context.Context, orgID, userID int64, role string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `UPDATE org.members SET role_key = $1 WHERE organization_id = $2 AND user_id = $3;`, role, orgID, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("org_member")
		}
		return nil
	})
}
