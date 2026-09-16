package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ListPlatformPaymentMethods returns all platform configured payment channels.
func (r *Repository) ListPlatformPaymentMethods(ctx context.Context, onlyActive bool) ([]*billing.PlatformPaymentMethod, error) {
	var methods []*billing.PlatformPaymentMethod
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var query string
		if onlyActive {
			query = `
				SELECT id, name_ar, name_en, provider_type, description_ar, description_en,
				       account_name, bank_name, account_number, iban, swift_code, branch_name,
				       instapay_handle, phone_number, is_active, is_deposit_enabled, is_checkout_enabled,
				       display_order, created_at, updated_at
				FROM billing.platform_payment_methods
				WHERE is_active = true
				ORDER BY display_order ASC, id ASC;
			`
		} else {
			query = `
				SELECT id, name_ar, name_en, provider_type, description_ar, description_en,
				       account_name, bank_name, account_number, iban, swift_code, branch_name,
				       instapay_handle, phone_number, is_active, is_deposit_enabled, is_checkout_enabled,
				       display_order, created_at, updated_at
				FROM billing.platform_payment_methods
				ORDER BY display_order ASC, id ASC;
			`
		}

		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m billing.PlatformPaymentMethod
			var nameAr, nameEn, descAr, descEn string

			if err := rows.Scan(
				&m.ID, &nameAr, &nameEn, &m.ProviderType, &descAr, &descEn,
				&m.AccountName, &m.BankName, &m.AccountNumber, &m.IBAN, &m.SwiftCode, &m.BranchName,
				&m.InstaPayHandle, &m.PhoneNumber, &m.IsActive, &m.IsDepositEnabled, &m.IsCheckoutEnabled,
				&m.DisplayOrder, &m.CreatedAt, &m.UpdatedAt,
			); err != nil {
				return err
			}

			m.Name = i18n.New(nameAr, nameEn)
			m.Description = i18n.New(descAr, descEn)

			methods = append(methods, &m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("billing postgres: list platform payment methods: %w", err)
	}
	return methods, nil
}

// GetPlatformPaymentMethod retrieves a single platform payment method by its unique ID.
func (r *Repository) GetPlatformPaymentMethod(ctx context.Context, id string) (*billing.PlatformPaymentMethod, error) {
	var m billing.PlatformPaymentMethod
	var nameAr, nameEn, descAr, descEn string

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, name_ar, name_en, provider_type, description_ar, description_en,
			       account_name, bank_name, account_number, iban, swift_code, branch_name,
			       instapay_handle, phone_number, is_active, is_deposit_enabled, is_checkout_enabled,
			       display_order, created_at, updated_at
			FROM billing.platform_payment_methods
			WHERE id = $1;
		`
		if err := tx.QueryRow(txCtx, query, id).Scan(
			&m.ID, &nameAr, &nameEn, &m.ProviderType, &descAr, &descEn,
			&m.AccountName, &m.BankName, &m.AccountNumber, &m.IBAN, &m.SwiftCode, &m.BranchName,
			&m.InstaPayHandle, &m.PhoneNumber, &m.IsActive, &m.IsDepositEnabled, &m.IsCheckoutEnabled,
			&m.DisplayOrder, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("platform_payment_method")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	m.Name = i18n.New(nameAr, nameEn)
	m.Description = i18n.New(descAr, descEn)

	return &m, nil
}

// SavePlatformPaymentMethod inserts or updates a platform payment channel.
func (r *Repository) SavePlatformPaymentMethod(ctx context.Context, pm *billing.PlatformPaymentMethod) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO billing.platform_payment_methods (
				id, name_ar, name_en, provider_type, description_ar, description_en,
				account_name, bank_name, account_number, iban, swift_code, branch_name,
				instapay_handle, phone_number, is_active, is_deposit_enabled, is_checkout_enabled,
				display_order, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, now())
			ON CONFLICT (id) DO UPDATE SET
				name_ar = EXCLUDED.name_ar,
				name_en = EXCLUDED.name_en,
				provider_type = EXCLUDED.provider_type,
				description_ar = EXCLUDED.description_ar,
				description_en = EXCLUDED.description_en,
				account_name = EXCLUDED.account_name,
				bank_name = EXCLUDED.bank_name,
				account_number = EXCLUDED.account_number,
				iban = EXCLUDED.iban,
				swift_code = EXCLUDED.swift_code,
				branch_name = EXCLUDED.branch_name,
				instapay_handle = EXCLUDED.instapay_handle,
				phone_number = EXCLUDED.phone_number,
				is_active = EXCLUDED.is_active,
				is_deposit_enabled = EXCLUDED.is_deposit_enabled,
				is_checkout_enabled = EXCLUDED.is_checkout_enabled,
				display_order = EXCLUDED.display_order,
				updated_at = now();
		`
		nameAr := pm.Name.Get("ar")
		nameEn := pm.Name.Get("en")
		descAr := pm.Description.Get("ar")
		descEn := pm.Description.Get("en")

		_, err := tx.Exec(
			txCtx, query,
			pm.ID, nameAr, nameEn, pm.ProviderType, descAr, descEn,
			pm.AccountName, pm.BankName, pm.AccountNumber, pm.IBAN, pm.SwiftCode, pm.BranchName,
			pm.InstaPayHandle, pm.PhoneNumber, pm.IsActive, pm.IsDepositEnabled, pm.IsCheckoutEnabled,
			pm.DisplayOrder,
		)
		return err
	})
}

// TogglePlatformPaymentMethod toggles the active state of a payment channel.
func (r *Repository) TogglePlatformPaymentMethod(ctx context.Context, id string, active bool) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE billing.platform_payment_methods
			SET is_active = $2, updated_at = now()
			WHERE id = $1;
		`
		cmd, err := tx.Exec(txCtx, query, id, active)
		if err != nil {
			return err
		}
		if cmd.RowsAffected() == 0 {
			return apperr.NotFound("platform_payment_method")
		}
		return nil
	})
}

// TogglePlatformPaymentMethodCheckout toggles whether a payment channel is enabled for checkout orders.
func (r *Repository) TogglePlatformPaymentMethodCheckout(ctx context.Context, id string, enabled bool) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE billing.platform_payment_methods
			SET is_checkout_enabled = $2, updated_at = now()
			WHERE id = $1;
		`
		cmd, err := tx.Exec(txCtx, query, id, enabled)
		if err != nil {
			return err
		}
		if cmd.RowsAffected() == 0 {
			return apperr.NotFound("platform_payment_method")
		}
		return nil
	})
}

// DeletePlatformPaymentMethod deletes a platform payment channel.
func (r *Repository) DeletePlatformPaymentMethod(ctx context.Context, id string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `DELETE FROM billing.platform_payment_methods WHERE id = $1;`
		cmd, err := tx.Exec(txCtx, query, id)
		if err != nil {
			return err
		}
		if cmd.RowsAffected() == 0 {
			return apperr.NotFound("platform_payment_method")
		}
		return nil
	})
}
