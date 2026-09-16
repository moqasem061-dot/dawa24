package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// ListBuyerOffers returns a paginated list of sellable supplier offers for a buyer.
// Paginates offers (catalog.product_variants) rather than master products,
// ensuring accurate totals and consistent page densities.
func (r *Repository) ListBuyerOffers(
	ctx context.Context, q catalog.BuyerOfferQuery,
) ([]*catalog.BuyerOffer, int, error) {
	// A signed-in company without a receiving branch may browse only after it
	// has selected a destination. Signed-out visitors have BuyerOrgID == 0 and
	// retain the public browsing path.
	if buyerRequiresBranch(q) {
		return nil, 0, nil
	}
	whereClauses := []string{
		"v.deleted_at IS NULL",
		"v.status = 'active'",
		"p.deleted_at IS NULL",
		"o.status = 'approved'",
		"o.type IN ('vendor','supplier','company','agency')",
	}
	args := make([]any, 0, 16)
	argNum := 1

	if q.BuyerOrgID > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("v.organization_id <> $%d", argNum))
		args = append(args, q.BuyerOrgID)
		argNum++
	}

	if q.SupplierOrgID > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("v.organization_id = $%d", argNum))
		args = append(args, q.SupplierOrgID)
		argNum++
	}

	if q.OnlyInStock {
		whereClauses = append(whereClauses, "st.qty > 0")
	}

	// Coverage, as a set the caller resolved once for this branch and weekday.
	//
	// It belongs here rather than in a pass over the returned rows because this
	// query is what the pager counts. Filtering afterwards produced a page of
	// 24 offers of which two survived, under a pager claiming 1,695 items.
	if q.ApplyCoverage {
		if len(q.CoveredVendorOrgIDs) == 0 && len(q.CoveredVendorBranchIDs) == 0 {
			// No supplier reaches this branch today. That is an empty result,
			// not an unfiltered one.
			return nil, 0, nil
		}
		whereClauses = append(whereClauses, fmt.Sprintf(`(
			(v.branch_id IS NULL AND v.organization_id = ANY($%d))
			OR v.branch_id = ANY($%d)
			OR (v.branch_id IS NOT NULL AND v.organization_id = ANY($%d))
		)`, argNum, argNum+1, argNum+2))
		args = append(args, nonNil(q.CoveredVendorOrgIDs), nonNil(q.CoveredVendorBranchIDs), nonNil(q.CoveredOrgWideVendorOrgIDs))
		argNum += 3
	}

	if q.BuyerBranchID > 0 {
		if len(q.AllowedWorkIDs) == 0 {
			// Buyer branch has no connected works; cannot buy anything.
			return nil, 0, nil
		}
		whereClauses = append(whereClauses, fmt.Sprintf(`EXISTS (
			SELECT 1
			FROM org.branch_institutional_works vw
			WHERE vw.institutional_work_id = ANY($%d)
			  AND (
			        (v.branch_id IS NOT NULL AND vw.branch_id = v.branch_id)
			     OR (v.branch_id IS NULL AND vw.branch_id IN (
			           SELECT b2.id FROM org.branches b2
			           WHERE b2.organization_id = v.organization_id
			             AND b2.deleted_at IS NULL AND b2.status <> 'inactive'))
			  )
		)`, argNum))
		args = append(args, q.AllowedWorkIDs)
		argNum++
	}

	trimmedQuery := strings.TrimSpace(q.Query)
	if trimmedQuery != "" {
		whereClauses = append(whereClauses, fmt.Sprintf(`(
			platform.normalize_arabic(p.name->>'ar') ILIKE '%%' || platform.normalize_arabic($%d) || '%%'
			OR p.name->>'en' ILIKE '%%' || $%d || '%%'
			OR platform.normalize_arabic(v.name->>'ar') ILIKE '%%' || platform.normalize_arabic($%d) || '%%'
			OR v.name->>'en' ILIKE '%%' || $%d || '%%'
			OR (p.scientific_name <> '' AND p.scientific_name ILIKE '%%' || $%d || '%%')
			OR (p.active <> '' AND p.active ILIKE '%%' || $%d || '%%')
			OR (p.manufacturing_companies <> '' AND p.manufacturing_companies ILIKE '%%' || $%d || '%%')
			OR v.sku = $%d
			OR v.barcode = $%d
			OR p.sku = $%d
			OR p.barcode = $%d
		)`, argNum, argNum, argNum, argNum, argNum, argNum, argNum, argNum, argNum, argNum, argNum))
		args = append(args, trimmedQuery)
		argNum++
	}

	if q.CategoryID != nil && *q.CategoryID > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("p.category_id = $%d", argNum))
		args = append(args, *q.CategoryID)
		argNum++
	}

	if q.BrandID != nil && *q.BrandID > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("p.brand_id = $%d", argNum))
		args = append(args, *q.BrandID)
		argNum++
	}

	if q.DosageForm != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("p.dosage_form ILIKE '%%' || $%d || '%%'", argNum))
		args = append(args, q.DosageForm)
		argNum++
	}

	if q.MinPriceMinor != nil && *q.MinPriceMinor > 0 {
		minPrice := money.FromMinor(*q.MinPriceMinor)
		whereClauses = append(whereClauses, fmt.Sprintf("v.price >= $%d", argNum))
		args = append(args, minPrice.String())
		argNum++
	}

	if q.MaxPriceMinor != nil && *q.MaxPriceMinor > 0 {
		maxPrice := money.FromMinor(*q.MaxPriceMinor)
		whereClauses = append(whereClauses, fmt.Sprintf("v.price <= $%d", argNum))
		args = append(args, maxPrice.String())
		argNum++
	}

	if q.OnlyDiscounted {
		whereClauses = append(whereClauses, "(v.discount > 0 OR (p.old_price > v.price AND p.old_price > 0))")
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	var userSort string
	switch q.Sort {
	case "price_asc":
		userSort = "v.price ASC, v.id ASC"
	case "price_desc":
		userSort = "v.price DESC, v.id ASC"
	case "newest":
		userSort = "v.created_at DESC, v.id ASC"
	case "discount":
		userSort = "v.discount DESC, v.id ASC"
	default:
		userSort = "p.sold_times DESC, v.id ASC"
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 24
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	var total int
	var offers []*catalog.BuyerOffer

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		countSQL := `
			SELECT count(*)
			FROM catalog.product_variants v
			JOIN catalog.products p ON p.id = v.product_id
			JOIN org.organizations o ON o.id = v.organization_id
			` + stockRollup + `
			WHERE ` + whereSQL

		if err := tx.QueryRow(txCtx, countSQL, args...).Scan(&total); err != nil {
			return fmt.Errorf("catalog postgres: count buyer offers: %w", err)
		}
		if total == 0 {
			return nil
		}

		selectSQL := fmt.Sprintf(`
			SELECT
				v.id,
				v.product_id,
				v.organization_id,
				COALESCE(o.name->>'ar', o.name->>'en', '') AS vendor_org_name,
				v.branch_id,
				COALESCE(b.name->>'ar', b.name->>'en', '') AS vendor_branch_name,
				COALESCE(ci.name->>'ar', ci.name->>'en', '') AS city_name,
				COALESCE(gov.name->>'ar', gov.name->>'en', '') AS governorate_name,
				COALESCE(b.latitude, ci.latitude)::double precision AS vendor_latitude,
				COALESCE(b.longitude, ci.longitude)::double precision AS vendor_longitude,
				p.name AS product_name,
				COALESCE(p.image, '') AS product_image,
				COALESCE(p.sku, '') AS product_sku,
				COALESCE(p.barcode, '') AS product_barcode,
				COALESCE(p.scientific_name, '') AS scientific_name,
				COALESCE(p.dosage_form, '') AS dosage_form,
				COALESCE(p.manufacturing_companies, '') AS manufacturing_company,
				p.brand_id,
				COALESCE(br.name->>'ar', br.name->>'en', '') AS brand_name,
				COALESCE(br.image, '') AS brand_logo,
				p.category_id,
				v.name AS variant_name,
				COALESCE(v.sku, '') AS variant_sku,
				COALESCE(v.image, '') AS variant_image,
				p.price AS public_price,
				v.price AS price,
				COALESCE(p.old_price, v.price) AS old_price,
				COALESCE(v.discount, 0) AS discount,
				COALESCE(st.qty, 0) AS available_stock,
				COALESCE(v.min_order_qty, 1) AS min_order_qty,
				v.quota_limit,
				v.expiry_date,
				v.is_featured,
				v.is_negotiable,
				COALESCE(sp.tier, 0) > 0 AS is_sponsored,
				COALESCE(sp.tier, 0) AS sponsored_tier,
				v.status
			FROM catalog.product_variants v
			JOIN catalog.products p ON p.id = v.product_id
			JOIN org.organizations o ON o.id = v.organization_id
			LEFT JOIN org.branches b ON b.id = v.branch_id AND b.deleted_at IS NULL
			LEFT JOIN platform_admin.cities ci ON ci.id = b.city_id
			LEFT JOIN platform_admin.governorates gov ON gov.id = ci.governorate_id
			LEFT JOIN catalog.brands br ON br.id = p.brand_id AND br.deleted_at IS NULL
			`+stockRollup+`
			LEFT JOIN LATERAL (
				SELECT COALESCE(MAX(op.tier_level), 0) AS tier
				FROM promo.offer_sponsorships sp
				JOIN promo.offer_packages op ON op.id = sp.package_id
				WHERE sp.status = 'active'
				  AND sp.admin_status = 'approved'
				  AND (sp.starts_at IS NULL OR sp.starts_at <= now())
				  AND (sp.expires_at IS NULL OR sp.expires_at >= now())
				  AND (
				        (sp.item_type IN ('offer', 'variant') AND COALESCE(sp.item_id, sp.offer_id) = v.id)
				     OR (sp.item_type = 'product' AND COALESCE(sp.item_id, sp.offer_id) = v.product_id)
				  )
			) sp ON true
			WHERE %s
			ORDER BY
				sp.tier DESC,
				st.qty > 0 DESC,
				%s
			LIMIT $%d OFFSET $%d
		`, whereSQL, userSort, argNum, argNum+1)

		queryArgs := append(args, limit, offset)
		rows, err := tx.Query(txCtx, selectSQL, queryArgs...)
		if err != nil {
			return fmt.Errorf("catalog postgres: select buyer offers: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var (
				off           catalog.BuyerOffer
				prodNameBytes []byte
				varNameBytes  []byte
				quotaLimitVal *int
				expiryDateVal *time.Time
			)
			err := rows.Scan(
				&off.VariantID,
				&off.ProductID,
				&off.VendorOrgID,
				&off.VendorOrgName,
				&off.VendorBranchID,
				&off.VendorBranchName,
				&off.CityName,
				&off.GovernorateName,
				&off.VendorLatitude,
				&off.VendorLongitude,
				&prodNameBytes,
				&off.ProductImage,
				&off.ProductSKU,
				&off.ProductBarcode,
				&off.ScientificName,
				&off.DosageForm,
				&off.ManufacturingCompany,
				&off.BrandID,
				&off.BrandName,
				&off.BrandLogo,
				&off.CategoryID,
				&varNameBytes,
				&off.VariantSKU,
				&off.VariantImage,
				&off.PublicPrice,
				&off.Price,
				&off.OldPrice,
				&off.Discount,
				&off.AvailableStock,
				&off.MinOrderQty,
				&quotaLimitVal,
				&expiryDateVal,
				&off.IsFeatured,
				&off.IsNegotiable,
				&off.IsSponsored,
				&off.SponsoredTier,
				&off.Status,
			)
			if err != nil {
				return fmt.Errorf("catalog postgres: scan buyer offer: %w", err)
			}

			if quotaLimitVal != nil {
				off.QuotaLimit = *quotaLimitVal
			}
			off.ExpiryDate = expiryDateVal

			if len(prodNameBytes) > 0 {
				_ = json.Unmarshal(prodNameBytes, &off.ProductName)
			}
			if len(varNameBytes) > 0 {
				_ = json.Unmarshal(varNameBytes, &off.VariantName)
			}

			variantPrice := off.Price
			if variantPrice.IsZero() {
				variantPrice = off.PublicPrice
			}
			off.PublicPrice = variantPrice
			off.OldPrice = variantPrice

			if off.Discount.IsPositive() && off.Discount.Minor() > 0 && off.Discount.Minor() < 10000 {
				off.DiscountPercent = int(off.Discount.Minor() / 100)
				off.Price = variantPrice.ApplyPercent(10000 - off.Discount.Minor())
			} else {
				off.Price = variantPrice
				off.DiscountPercent = 0
			}

			offers = append(offers, &off)
		}

		return rows.Err()
	})

	if err != nil {
		return nil, 0, err
	}
	return offers, total, nil
}

func buyerRequiresBranch(q catalog.BuyerOfferQuery) bool {
	return q.BuyerOrgID > 0 && q.BuyerBranchID <= 0
}

// nonNil binds an empty id set as an empty array rather than NULL, so
// "= ANY(...)" is false instead of unknown.
func nonNil(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}
