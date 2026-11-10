package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// orgVendorTypes lists the stored organization types that rbac.NormalizeOrgType
// maps to the vendor role. Only approved vendors sell.
const orgVendorTypes = `('vendor', 'supplier', 'company', 'agency')`

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// offerListSQL reads the supplier offers for one barcode: live variants of
// approved vendors with a positive balance, cheapest first. $1 buyer, $2 barcode,
// $3 limit. It returns public fields only.
const offerListSQL = `
SELECT v.id::text, o.name, p.name, v.barcode, v.price, v.discount,
       EXISTS (SELECT 1 FROM org.user_organizations uo
               WHERE uo.customer_org_id = $1 AND uo.vendor_org_id = v.organization_id
                 AND uo.status IN ('pending', 'approved') AND uo.deleted_at IS NULL) AS linked
FROM catalog.product_variants v
JOIN org.organizations o ON o.id = v.organization_id
JOIN catalog.products p ON p.id = v.product_id
WHERE v.barcode = $2 AND v.status = 'active' AND v.deleted_at IS NULL
  AND o.status IN ('approved', 'active', 'verified') AND o.type IN ` + orgVendorTypes + `
  AND v.discount >= 0 AND v.discount < 100
  AND EXISTS (SELECT 1 FROM inventory.stocks s WHERE s.product_variant_id = v.id AND s.quantity > 0)
ORDER BY v.price * (1 - v.discount / 100), v.id
LIMIT $3`

// variantOfferSelect reads one variant with the flags the purchase rules need.
// active is the purchase rule: live variant, approved vendor. The two WHERE
// forms below pick the variant by its numeric id or by its legacy public id.
const variantOfferSelect = `
SELECT v.id, v.organization_id, v.public_id::text, v.price, v.discount, o.name,
       (v.status = 'active' AND v.deleted_at IS NULL AND o.status IN ('approved', 'active', 'verified') AND o.type IN ` + orgVendorTypes + `) AS active,
       EXISTS (SELECT 1 FROM inventory.stocks s WHERE s.product_variant_id = v.id AND s.quantity > 0) AS in_stock
FROM catalog.product_variants v
JOIN org.organizations o ON o.id = v.organization_id
`

// variantOfferByIDSQL reads a variant by its numeric id. $1 is the id.
const variantOfferByIDSQL = variantOfferSelect + `WHERE v.id = $1`

// variantOfferByPublicIDSQL reads a variant by its public UUID. $1 is the UUID.
const variantOfferByPublicIDSQL = variantOfferSelect + `WHERE v.public_id = $1::uuid`

// ListOffersAsSystem reads supplier offers across tenants. plan.md E6: the
// supplier variants belong to other organizations, which the buyer cannot see
// under its own tenant. The query returns public fields only.
func (r *Repository) ListOffersAsSystem(ctx context.Context, buyerOrgID int64, barcode string, limit int) ([]erp.Offer, error) {
	var out []erp.Offer
	err := r.db.InReadTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, offerListSQL, buyerOrgID, barcode, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var o erp.Offer
			var supplier, name []byte
			if err := rows.Scan(&o.VariantID, &supplier, &name, &o.Barcode, &o.PublicPrice, &o.DiscountPercent, &o.SupplierLinked); err != nil {
				return err
			}
			if err := unmarshalNames(supplier, &o.SupplierName); err != nil {
				return err
			}
			if err := unmarshalNames(name, &o.Name); err != nil {
				return err
			}
			o.InStock = true
			out = append(out, o)
		}
		return rows.Err()
	})
	return out, err
}

// VariantOfferAsSystem resolves one variant by its id or public id across
// tenants. plan.md E6: the buyer cannot read the supplier's variant under its own
// tenant. Decimal text is the numeric id; a UUID is the legacy public id. Any
// other text, or an id that matches no variant, returns erp.ErrNotFound.
func (r *Repository) VariantOfferAsSystem(ctx context.Context, idOrPublicID string) (erp.VariantOfferRow, error) {
	sql, arg := variantOfferByPublicIDSQL, any(idOrPublicID)
	if id, ok := erp.ParseShortID(idOrPublicID); ok {
		sql, arg = variantOfferByIDSQL, id
	} else if !uuidPattern.MatchString(idOrPublicID) {
		return erp.VariantOfferRow{}, erp.ErrNotFound
	}
	var row erp.VariantOfferRow
	var supplier []byte
	err := r.db.InReadTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, sql, arg).Scan(&row.ID, &row.OrganizationID, &row.PublicID,
			&row.PublicPrice, &row.DiscountPercent, &supplier, &row.Active, &row.InStock)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return erp.VariantOfferRow{}, erp.ErrNotFound
	}
	if err != nil {
		return erp.VariantOfferRow{}, err
	}
	if err := unmarshalNames(supplier, &row.SupplierName); err != nil {
		return erp.VariantOfferRow{}, err
	}
	return row, nil
}

func unmarshalNames(raw []byte, into *map[string]string) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, into)
}
