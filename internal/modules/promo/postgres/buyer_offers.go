package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// offerRule renders the buyer-side offer predicates over the alias o.
//
// The listing ANDs them into its WHERE clause and the verdict selects each one
// as a column, from the same strings, so an offer the board shows is an offer
// the detail page, add-to-cart and checkout accept, and the reason one of them
// refuses is the predicate the board filtered on.
type offerRule struct {
	args []any
}

func (r *offerRule) bind(v any) string {
	r.args = append(r.args, v)
	return fmt.Sprintf("$%d", len(r.args))
}

// live is an offer a supplier is currently running and the platform approved.
func (r *offerRule) live() string {
	return `(o.deleted_at IS NULL AND o.is_active AND NOT o.is_draft
		AND o.admin_status = 'approved'
		AND (o.starts_at IS NULL OR o.starts_at <= now())
		AND (o.expires_at IS NULL OR o.expires_at >= now()))`
}

// supplier is the organisation types and status commerce.CheckAvailability
// accepts as a seller.
func (r *offerRule) supplier() string {
	return `EXISTS (SELECT 1 FROM org.organizations so
		WHERE so.id = o.organization_id AND so.deleted_at IS NULL
		  AND so.status = 'approved'
		  AND so.type IN ('vendor','supplier','company','agency'))`
}

// branch refuses an offer bound to a supplier branch that no longer ships:
// deleted, deactivated, or not the supplier's.
func (r *offerRule) branch() string {
	return `(o.branch_id IS NULL OR EXISTS (SELECT 1 FROM org.branches ob
		WHERE ob.id = o.branch_id AND ob.organization_id = o.organization_id
		  AND ob.deleted_at IS NULL AND ob.status <> 'inactive'))`
}

func (r *offerRule) own(buyerOrgID int64) string {
	return fmt.Sprintf("(o.organization_id = %s)", r.bind(buyerOrgID))
}

// institutional is the catalogue's rule: a supplier branch the offer can ship
// from holds a work connected to the buyer branch's.
func (r *offerRule) institutional(b promo.BuyerBranch) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM org.branch_institutional_works vw
		WHERE vw.institutional_work_id = ANY(%s)
		  AND ((o.branch_id IS NOT NULL AND vw.branch_id = o.branch_id)
		    OR (o.branch_id IS NULL AND vw.branch_id IN (
		          SELECT b2.id FROM org.branches b2
		           WHERE b2.organization_id = o.organization_id
		             AND b2.deleted_at IS NULL AND b2.status <> 'inactive'))))`,
		r.bind(nonNilIDs(b.AllowedWorkIDs)))
}

// coverage decides delivery. An offer with its own approved location rules is
// delivered where and on the weekday those rules say, and nowhere else; an
// offer without them follows the supplier's weekly coverage, which workflow
// resolved for today into sets.
func (r *offerRule) coverage(b promo.BuyerBranch, c promo.SupplierCoverage) string {
	rules := `SELECT 1 FROM promo.offer_location_covers l
		WHERE l.offer_id = o.id AND l.status = 'active' AND l.admin_status = 'approved'`
	day := r.bind(int(b.Weekday))
	city := r.bind(b.CityID)
	hasCoords := r.bind(b.HasCoords)
	lat, lon := r.bind(b.Lat), r.bind(b.Lon)
	orgs, branches, orgWide := r.bind(nonNilIDs(c.OrgIDs)), r.bind(nonNilIDs(c.BranchIDs)), r.bind(nonNilIDs(c.OrgWideIDs))
	return fmt.Sprintf(`(CASE WHEN EXISTS (%[1]s) THEN EXISTS (%[1]s
			AND (l.day_of_week = %[2]s OR l.day_of_week IS NULL)
			AND ((%[3]s::bigint > 0 AND l.city_id = %[3]s::bigint)
			  OR (%[4]s::boolean AND l.latitude IS NOT NULL AND l.longitude IS NOT NULL
			      AND NOT (l.latitude = 0 AND l.longitude = 0)
			      AND platform.distance_meters(l.latitude::numeric, l.longitude::numeric, %[5]s::numeric, %[6]s::numeric)
			          <= COALESCE(NULLIF(l.radius_meters, 0), 1000))))
		ELSE ((o.branch_id IS NULL AND o.organization_id = ANY(%[7]s))
		   OR (o.branch_id IS NOT NULL AND (o.branch_id = ANY(%[8]s) OR o.organization_id = ANY(%[9]s))))
		END)`, rules, day, city, hasCoords, lat, lon, orgs, branches, orgWide)
}

// nonNilIDs keeps an empty set an empty array: a nil slice binds as NULL, and
// "= ANY(NULL)" is unknown rather than false.
func nonNilIDs(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

// buyerConditions is every predicate a buyer query applies.
func (r *offerRule) buyerConditions(q promo.BuyerOfferQuery) []string {
	conds := []string{r.live(), r.supplier(), r.branch()}
	if q.BuyerOrgID > 0 {
		conds = append(conds, "NOT "+r.own(q.BuyerOrgID))
	}
	if q.Buying {
		conds = append(conds, r.institutional(q.Branch), r.coverage(q.Branch, q.Coverage))
	}
	return conds
}

const offerSponsoredExpr = `EXISTS (
	SELECT 1 FROM promo.offer_sponsorships s
	 WHERE COALESCE(s.item_id, s.offer_id) = o.id AND COALESCE(s.item_type, 'offer') = 'offer'
	   AND s.status = 'active' AND s.admin_status = 'approved' AND s.expires_at > now()
	UNION ALL
	SELECT 1 FROM promo.sponsorship_requests s
	 WHERE s.item_id = o.id AND s.item_type = 'offer'
	   AND s.status = 'active' AND s.admin_status = 'approved' AND s.expires_at > now())`

func buyerOfferOrder(sort promo.BuyerOfferSort) string {
	price := "COALESCE(NULLIF(o.total_price, 0), NULLIF(o.min_order_amount, 0))"
	discount := "CASE WHEN o.discount_type = 'percentage' THEN o.discount_value ELSE 0 END"
	switch sort {
	case promo.SortOffersDiscountDesc:
		return "sponsored DESC, " + discount + " DESC, o.discount_value DESC, o.id DESC"
	case promo.SortOffersDiscountAsc:
		return "sponsored DESC, " + discount + " ASC, o.discount_value ASC, o.id DESC"
	case promo.SortOffersPriceAsc:
		return "sponsored DESC, " + price + " ASC NULLS LAST, o.id DESC"
	case promo.SortOffersPriceDesc:
		return "sponsored DESC, " + price + " DESC NULLS LAST, o.id DESC"
	}
	return "sponsored DESC, o.starts_at DESC NULLS LAST, o.id DESC"
}

// ListBuyerOffers pages the offers a buyer may see, filtered and counted in SQL.
func (r *Repository) ListBuyerOffers(ctx context.Context, q promo.BuyerOfferQuery) ([]*promo.BuyerOffer, int, error) {
	rule := &offerRule{}
	conds := rule.buyerConditions(q)
	if term := strings.TrimSpace(q.Search); term != "" {
		t := rule.bind(term)
		conds = append(conds, fmt.Sprintf(`(platform.normalize_arabic(o.title->>'ar') ILIKE '%%' || platform.normalize_arabic(%[1]s) || '%%'
			OR o.title->>'en' ILIKE '%%' || %[1]s || '%%'
			OR platform.normalize_arabic(vo.name->>'ar') ILIKE '%%' || platform.normalize_arabic(%[1]s) || '%%'
			OR EXISTS (SELECT 1 FROM promo.offer_products sp JOIN catalog.products cp ON cp.id = sp.product_id
			           WHERE sp.offer_id = o.id AND (platform.normalize_arabic(cp.name->>'ar') ILIKE '%%' || platform.normalize_arabic(%[1]s) || '%%'
			              OR cp.name->>'en' ILIKE '%%' || %[1]s || '%%' OR cp.scientific_name ILIKE '%%' || %[1]s || '%%')))`, t))
	}
	if q.Discounts {
		conds = append(conds, "o.discount_value > 0")
	}
	limit, offset := q.Limit, q.Offset
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	if offset < 0 {
		offset = 0
	}
	query := fmt.Sprintf(`
		SELECT o.id, o.organization_id, vo.name, COALESCE(vo.legal_name, ''), o.branch_id,
		       o.title, o.description, o.discount_type, o.discount_value,
		       COALESCE(o.min_order_amount, 0), COALESCE(o.total_price, 0),
		       o.starts_at, o.expires_at,
		       (SELECT count(*) FROM promo.offer_products op WHERE op.offer_id = o.id),
		       %s AS sponsored,
		       count(*) OVER ()
		  FROM promo.offers o
		  JOIN org.organizations vo ON vo.id = o.organization_id
		 WHERE %s
		 ORDER BY %s
		 LIMIT %s OFFSET %s`,
		offerSponsoredExpr, strings.Join(conds, "\n\t\t   AND "), buyerOfferOrder(q.Sort), rule.bind(limit), rule.bind(offset))

	var (
		out   []*promo.BuyerOffer
		total int
	)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, query, rule.args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				o        promo.BuyerOffer
				discType string
			)
			if err := rows.Scan(&o.ID, &o.OrganizationID, &o.OrganizationName, &o.LegalName, &o.BranchID,
				&o.Title, &o.Description, &discType, &o.DiscountValue,
				&o.MinOrderAmount, &o.TotalPrice, &o.StartsAt, &o.ExpiresAt,
				&o.ProductCount, &o.Sponsored, &total); err != nil {
				return err
			}
			o.DiscountType = promo.DiscountType(discType)
			out = append(out, &o)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, fmt.Errorf("promo postgres: list buyer offers: %w", err)
	}
	return out, total, nil
}

// OfferVerdict evaluates one offer against each buyer predicate.
func (r *Repository) OfferVerdict(ctx context.Context, q promo.BuyerOfferQuery) (promo.OfferVerdict, error) {
	rule := &offerRule{}
	id := rule.bind(q.OfferID)
	institutional, covered := "false", "false"
	if q.Buying {
		institutional, covered = rule.institutional(q.Branch), rule.coverage(q.Branch, q.Coverage)
	}
	query := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s, %s FROM promo.offers o WHERE o.id = %s`,
		rule.live(), rule.supplier(), rule.branch(), rule.own(q.BuyerOrgID), institutional, covered, id)

	v := promo.OfferVerdict{}
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, query, rule.args...).Scan(
			&v.Live, &v.SupplierOK, &v.BranchOK, &v.OwnOffer, &v.Institutional, &v.Covered)
	})
	if database.IsNotFound(err) {
		return promo.OfferVerdict{}, nil
	}
	if err != nil {
		return promo.OfferVerdict{}, fmt.Errorf("promo postgres: offer verdict %d: %w", q.OfferID, err)
	}
	v.Found = true
	return v, nil
}

// ListRunningOffersByOrg is one supplier's own live offers with their counters.
func (r *Repository) ListRunningOffersByOrg(ctx context.Context, orgID int64, limit int) ([]*promo.Offer, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	rule := &offerRule{}
	query := `SELECT ` + offerColumnsAs("o") + ` FROM promo.offers o
		WHERE o.organization_id = $1 AND ` + rule.live() + `
		ORDER BY o.id DESC LIMIT $2`
	var out []*promo.Offer
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, query, orgID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			o, err := scanOffer(rows)
			if err != nil {
				return err
			}
			out = append(out, o)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("promo postgres: running offers for org %d: %w", orgID, err)
	}
	return out, nil
}
