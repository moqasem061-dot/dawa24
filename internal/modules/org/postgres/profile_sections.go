package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Writing one section of a company's profile.
//
// The identity section writes name, trade_name **and** legal_name together, and
// that is the fix for the bug this whole feature came from. The old page wrote
// only org.organizations.name, and nothing reads it: every screen that displays
// a company —
//
//	org/postgres/admin_org_list.go        legal_name → name → trade_name
//	compare/postgres/market_discounts.go  trade_name → legal_name (never name)
//	org/postgres/admin_reviews.go         trade_name → name
//
// — prefers trade_name or legal_name. A supplier renamed themselves, the UPDATE
// succeeded, and every list on the platform went on showing the old name.
// Organisation 51 is still in that state: name.ar is 'شركة ويزر فارماالاب' and
// trade_name.ar is 'شركة ويزر فارما'.

// ReadProfileSection returns the stored values of one section, for the change
// request's "previous" snapshot and for re-rendering a form.
func (r *Repository) ReadProfileSection(
	ctx context.Context, orgID int64, section org.ProfileSection,
) (org.ProfileFields, error) {
	var fields org.ProfileFields
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var err error
		fields, err = readProfileSectionTx(txCtx, tx, orgID, section)
		return err
	})
	return fields, err
}

func readProfileSectionTx(
	ctx context.Context, tx pgx.Tx, orgID int64, section org.ProfileSection,
) (org.ProfileFields, error) {
	const query = `
		SELECT COALESCE(legal_name, ''),
		       COALESCE(trade_name->>'ar', ''), COALESCE(trade_name->>'en', ''),
		       COALESCE(commercial_register, ''), COALESCE(tax_number, ''),
		       COALESCE(min_order_price, 0), COALESCE(max_order_price, 0),
		       COALESCE(email, ''), COALESCE(phone, ''), COALESCE(address, ''),
		       COALESCE(organization_number, ''),
		       COALESCE(description->>'ar', ''), COALESCE(description->>'en', ''),
		       COALESCE(image, ''), COALESCE(coverage_image, '')
		FROM org.organizations
		WHERE id = $1 AND deleted_at IS NULL;`

	var (
		legalName, tradeAr, tradeEn, cr, tax string
		minPrice, maxPrice                   money.Amount
		email, phone, address, orgNumber     string
		descAr, descEn, image, coverImage    string
	)
	if err := tx.QueryRow(ctx, query, orgID).Scan(
		&legalName, &tradeAr, &tradeEn, &cr, &tax,
		&minPrice, &maxPrice,
		&email, &phone, &address, &orgNumber,
		&descAr, &descEn, &image, &coverImage,
	); err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("organization")
		}
		return nil, fmt.Errorf("org postgres: read profile section: %w", err)
	}

	switch section {
	case org.SectionIdentity:
		return org.ProfileFields{
			"legal_name":          legalName,
			"trade_name_ar":       tradeAr,
			"trade_name_en":       tradeEn,
			"commercial_register": cr,
			"tax_number":          tax,
		}, nil
	case org.SectionLimits:
		return org.ProfileFields{
			"min_order_price": minPrice.String(),
			"max_order_price": maxPrice.String(),
		}, nil
	case org.SectionContact:
		return org.ProfileFields{
			"email":               email,
			"phone":               phone,
			"address":             address,
			"organization_number": orgNumber,
		}, nil
	case org.SectionDescription:
		return org.ProfileFields{
			"description_ar": descAr,
			"description_en": descEn,
		}, nil
	case org.SectionMedia:
		return org.ProfileFields{
			"image":          image,
			"coverage_image": coverImage,
		}, nil
	default:
		return nil, apperr.Validation("org.profile.unknown_section", "Unknown profile section.", nil)
	}
}

// ApplyProfileSection writes one section onto the organization.
func (r *Repository) ApplyProfileSection(
	ctx context.Context, orgID int64, section org.ProfileSection, fields org.ProfileFields,
) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return applyProfileSectionTx(txCtx, tx, orgID, section, fields)
	})
}

func applyProfileSectionTx(
	ctx context.Context, tx pgx.Tx, orgID int64, section org.ProfileSection, fields org.ProfileFields,
) error {
	var (
		query string
		args  []any
	)

	switch section {
	case org.SectionIdentity:
		// name is kept in step with trade_name rather than being the only thing
		// written. It is the legacy column and some older queries still fall
		// back to it; letting the two drift is what made the old page's saves
		// invisible.
		tradeAr := strings.TrimSpace(fields["trade_name_ar"])
		tradeEn := strings.TrimSpace(fields["trade_name_en"])
		if tradeEn == "" {
			tradeEn = tradeAr
		}
		query = `
			UPDATE org.organizations
			SET legal_name          = $2,
			    trade_name          = jsonb_build_object('ar', $3::text, 'en', $4::text),
			    name                = jsonb_build_object('ar', $3::text, 'en', $4::text),
			    commercial_register = $5,
			    tax_number          = $6,
			    updated_at          = now()
			WHERE id = $1 AND deleted_at IS NULL;`
		args = []any{orgID,
			strings.TrimSpace(fields["legal_name"]), tradeAr, tradeEn,
			strings.TrimSpace(fields["commercial_register"]),
			strings.TrimSpace(fields["tax_number"]),
		}

	case org.SectionLimits:
		minPrice, err := money.Parse(strings.TrimSpace(fields["min_order_price"]))
		if err != nil {
			return apperr.Validation("org.profile.invalid_amount", "Invalid minimum order value.", nil)
		}
		maxPrice, err := money.Parse(strings.TrimSpace(fields["max_order_price"]))
		if err != nil {
			return apperr.Validation("org.profile.invalid_amount", "Invalid maximum order value.", nil)
		}
		// org_price_range enforces this in the database too; refusing here
		// turns a constraint violation into a sentence.
		if maxPrice.Minor() < minPrice.Minor() {
			return apperr.Validation("org.profile.price_range",
				"The maximum order value must not be below the minimum.", nil)
		}
		query = `
			UPDATE org.organizations
			SET min_order_price = $2, max_order_price = $3, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;`
		args = []any{orgID, minPrice, maxPrice}

	case org.SectionContact:
		query = `
			UPDATE org.organizations
			SET email = $2, phone = $3, address = $4, organization_number = $5, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;`
		args = []any{orgID,
			strings.TrimSpace(fields["email"]), strings.TrimSpace(fields["phone"]),
			strings.TrimSpace(fields["address"]), strings.TrimSpace(fields["organization_number"]),
		}

	case org.SectionDescription:
		query = `
			UPDATE org.organizations
			SET description = jsonb_build_object('ar', $2::text, 'en', $3::text), updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;`
		args = []any{orgID,
			strings.TrimSpace(fields["description_ar"]), strings.TrimSpace(fields["description_en"]),
		}

	case org.SectionMedia:
		// An empty value means "no new file was uploaded", so the stored image
		// survives. A form that omits a file must not blank the logo.
		query = `
			UPDATE org.organizations
			SET image          = CASE WHEN $2::text <> '' THEN $2::text ELSE image END,
			    coverage_image = CASE WHEN $3::text <> '' THEN $3::text ELSE coverage_image END,
			    updated_at     = now()
			WHERE id = $1 AND deleted_at IS NULL;`
		args = []any{orgID, fields["image"], fields["coverage_image"]}

	default:
		return apperr.Validation("org.profile.unknown_section", "Unknown profile section.", nil)
	}

	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("org postgres: apply profile section %s: %w", section, err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("organization")
	}
	return nil
}

// ApplyApprovedProfileChange is the callback DecideProfileChangeRequest runs
// inside its own transaction when an administrator approves.
func (r *Repository) ApplyApprovedProfileChange(
	ctx context.Context, tx pgx.Tx, req *org.ProfileChangeRequest,
) error {
	return applyProfileSectionTx(ctx, tx, req.OrganizationID, req.Section, req.Proposed)
}
