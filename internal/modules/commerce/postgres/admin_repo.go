package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// AdminSearchOrders finds orders across every tenant.
func (r *Repository) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*commerce.Order, error) {
	var list []*commerce.Order
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const sql = `
			SELECT ` + orderColumns + `
			FROM commerce.orders
			WHERE deleted_at IS NULL
			  AND ($1::text = '' OR order_number ILIKE '%' || $1 || '%')
			ORDER BY created_at DESC, id DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		if offset < 0 {
			offset = 0
		}

		rows, err := tx.Query(txCtx, sql, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			o, err := scanOrder(rows)
			if err != nil {
				return err
			}
			list = append(list, o)
		}
		return rows.Err()
	})
	return list, err
}

// AdminSearchOrdersWithTotal finds orders across every tenant with pagination and optional tab filtering.
func (r *Repository) AdminSearchOrdersWithTotal(ctx context.Context, query, tab string, limit, offset int) ([]*commerce.Order, int, error) {
	var list []*commerce.Order
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"deleted_at IS NULL"}
		var args []any

		if query != "" {
			args = append(args, "%"+query+"%")
			p := "$" + strconv.Itoa(len(args))
			where = append(where, "order_number ILIKE "+p)
		}

		if tab == "direct" {
			where = append(where, "is_negotiation = false")
		} else if tab == "negotiations" {
			where = append(where, "is_negotiation = true")
		}

		clause := strings.Join(where, " AND ")

		countSQL := "SELECT count(*) FROM commerce.orders WHERE " + clause + ";"
		if err := tx.QueryRow(txCtx, countSQL, args...).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		args = append(args, limit, offset)
		limParam := "$" + strconv.Itoa(len(args)-1)
		offParam := "$" + strconv.Itoa(len(args))

		sql := `
			SELECT ` + orderColumns + `
			FROM commerce.orders
			WHERE ` + clause + `
			ORDER BY created_at DESC, id DESC
			LIMIT ` + limParam + ` OFFSET ` + offParam + `;
		`

		rows, err := tx.Query(txCtx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			o, err := scanOrder(rows)
			if err != nil {
				return err
			}
			list = append(list, o)
		}
		return rows.Err()
	})
	return list, total, err
}

// AdminOrderStats returns count of all, direct, and negotiation orders.
func (r *Repository) AdminOrderStats(ctx context.Context) (allCount, directCount, negotiationCount int, err error) {
	err = r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT
				COUNT(*) AS all_orders,
				COUNT(*) FILTER (WHERE is_negotiation = false) AS direct_orders,
				COUNT(*) FILTER (WHERE is_negotiation = true) AS negotiation_orders
			FROM commerce.orders
			WHERE deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, query).Scan(&allCount, &directCount, &negotiationCount)
	})
	return allCount, directCount, negotiationCount, err
}

// AdminOrderKPIs returns aggregated metrics for the admin orders dashboard.
func (r *Repository) AdminOrderKPIs(ctx context.Context) (commerce.AdminOrderKPIs, error) {
	var kpi commerce.AdminOrderKPIs
	var volMinor int64
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT
				COUNT(*),
				COUNT(*) FILTER (WHERE is_negotiation = false),
				COUNT(*) FILTER (WHERE is_negotiation = true),
				COUNT(*) FILTER (WHERE status IN ('pending', 'processing', 'on_hold')),
				COUNT(*) FILTER (WHERE status IN ('confirmed', 'shipped', 'in_transit', 'out_for_delivery')),
				COUNT(*) FILTER (WHERE status IN ('delivered', 'completed')),
				COUNT(*) FILTER (WHERE status IN ('cancelled', 'failed', 'returned', 'refunded')),
				COALESCE(ROUND(SUM(total_amount) * 100)::bigint, 0)
			FROM commerce.orders
			WHERE deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, query).Scan(
			&kpi.TotalOrders, &kpi.DirectOrders, &kpi.NegotiationOrders,
			&kpi.PendingOrders, &kpi.ShippingOrders, &kpi.CompletedOrders,
			&kpi.CancelledOrders, &volMinor,
		)
	})
	kpi.TotalVolume = money.FromMinor(volMinor)
	return kpi, err
}

// AdminSearchOrdersFiltered finds orders across every tenant with rich filters and enriched metadata.
func (r *Repository) AdminSearchOrdersFiltered(ctx context.Context, filter commerce.AdminOrderFilter) ([]*commerce.Order, int, error) {
	var list []*commerce.Order
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"ord.deleted_at IS NULL"}
		var args []any

		if filter.Query != "" {
			args = append(args, "%"+strings.ToLower(filter.Query)+"%")
			p := "$" + strconv.Itoa(len(args))
			where = append(where, fmt.Sprintf("(LOWER(ord.order_number) LIKE %s OR LOWER(COALESCE(corg.legal_name, '')) LIKE %s OR LOWER(COALESCE(vorg.legal_name, '')) LIKE %s OR LOWER(COALESCE(corg.name->>'ar', '')) LIKE %s OR LOWER(COALESCE(vorg.name->>'ar', '')) LIKE %s)", p, p, p, p, p))
		}

		if filter.Tab == "direct" {
			where = append(where, "ord.is_negotiation = false")
		} else if filter.Tab == "negotiations" {
			where = append(where, "ord.is_negotiation = true")
		}

		if filter.Status != "" && filter.Status != "all" {
			args = append(args, filter.Status)
			where = append(where, "ord.status = $"+strconv.Itoa(len(args)))
		}

		if filter.PaymentStatus != "" && filter.PaymentStatus != "all" {
			args = append(args, filter.PaymentStatus)
			where = append(where, "ord.payment_status = $"+strconv.Itoa(len(args)))
		}

		if filter.CustomerOrgID > 0 {
			args = append(args, filter.CustomerOrgID)
			where = append(where, "ord.organization_id = $"+strconv.Itoa(len(args)))
		}

		if filter.VendorOrgID > 0 {
			args = append(args, filter.VendorOrgID)
			where = append(where, "(vb.organization_id = $"+strconv.Itoa(len(args))+" OR sh_vorg.organization_id = $"+strconv.Itoa(len(args))+")")
		}

		// The buyer is whatever kind of company placed the order. A supplier
		// restocking from another supplier is an ordinary order here, and
		// filtering the picker by type is how those orders became unfindable.
		if filter.BuyerType != "" && filter.BuyerType != "all" {
			args = append(args, filter.BuyerType)
			where = append(where, "corg.type = $"+strconv.Itoa(len(args)))
		}

		if filter.DateFrom != "" {
			args = append(args, filter.DateFrom)
			where = append(where, "ord.created_at >= $"+strconv.Itoa(len(args))+"::date")
		}

		if filter.DateTo != "" {
			args = append(args, filter.DateTo)
			where = append(where, "ord.created_at <= ($"+strconv.Itoa(len(args))+"::date + INTERVAL '1 day')")
		}

		clause := strings.Join(where, " AND ")

		countSQL := `
			SELECT count(*) 
			FROM commerce.orders ord
			LEFT JOIN org.organizations corg ON corg.id = ord.organization_id
			LEFT JOIN org.branches cb ON cb.id = ord.branch_id
			LEFT JOIN org.branches vb ON vb.id = ord.vendor_branch_id
			LEFT JOIN org.organizations vorg ON vorg.id = vb.organization_id
			LEFT JOIN LATERAL (
				SELECT sh.organization_id, sh_org.name
				FROM commerce.order_shipments sh
				JOIN org.organizations sh_org ON sh_org.id = sh.organization_id
				WHERE sh.order_id = ord.id
				LIMIT 1
			) sh_vorg ON true
			WHERE ` + clause + `;`

		if err := tx.QueryRow(txCtx, countSQL, args...).Scan(&total); err != nil {
			return err
		}

		limit := filter.Limit
		if limit <= 0 || limit > 100 {
			limit = 25
		}
		offset := filter.Offset
		if offset < 0 {
			offset = 0
		}

		args = append(args, limit, offset)
		limParam := "$" + strconv.Itoa(len(args)-1)
		offParam := "$" + strconv.Itoa(len(args))

		sql := `
			SELECT 
				ord.id, ord.public_id, ord.order_number, ord.customer_id, ord.organization_id,
				ord.offer_id, ord.branch_id, ord.vendor_branch_id, ord.user_address_id, ord.status,
				ord.subtotal, ord.discount_amount, ord.total_discount, ord.shipping_fee, ord.tax_amount,
				ord.total_amount, ord.final_price, ord.payment_method, ord.payment_status, ord.notes,
				ord.is_negotiation, ord.negotiation_status, COALESCE(ord.negotiation_notes, '') AS negotiation_notes,
				ord.rating, ord.review, ord.rated_at, ord.delivered_at,
				ord.created_at, ord.updated_at, ord.deleted_at,
				COALESCE(NULLIF(corg.name, '{"ar":"","en":""}'::jsonb), NULLIF(corg.trade_name, '{"ar":"","en":""}'::jsonb), jsonb_build_object('ar', COALESCE(corg.legal_name, ''), 'en', COALESCE(corg.legal_name, ''))),
				COALESCE(cb.name, '{"ar":"","en":""}'::jsonb),
				COALESCE(ccity.name->>'ar', ''),
				COALESCE(NULLIF(vorg.name, '{"ar":"","en":""}'::jsonb), NULLIF(vorg.trade_name, '{"ar":"","en":""}'::jsonb), sh_vorg.name, jsonb_build_object('ar', COALESCE(vorg.legal_name, ''), 'en', COALESCE(vorg.legal_name, ''))),
				COALESCE(vb.name, '{"ar":"","en":""}'::jsonb),
				(SELECT COUNT(*) FROM commerce.order_lines ol WHERE ol.order_id = ord.id)
			FROM commerce.orders ord
			LEFT JOIN org.organizations corg ON corg.id = ord.organization_id
			LEFT JOIN org.branches cb ON cb.id = ord.branch_id
			LEFT JOIN platform_admin.cities ccity ON ccity.id = cb.city_id
			LEFT JOIN org.branches vb ON vb.id = ord.vendor_branch_id
			LEFT JOIN org.organizations vorg ON vorg.id = vb.organization_id
			LEFT JOIN LATERAL (
				SELECT sh.organization_id, sh_org.name
				FROM commerce.order_shipments sh
				JOIN org.organizations sh_org ON sh_org.id = sh.organization_id
				WHERE sh.order_id = ord.id
				LIMIT 1
			) sh_vorg ON true
			WHERE ` + clause + `
			ORDER BY ord.created_at DESC, ord.id DESC
			LIMIT ` + limParam + ` OFFSET ` + offParam + `;
		`

		rows, err := tx.Query(txCtx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var o commerce.Order
			var statusStr, payStatusStr string
			var offerID *int64
			if err := rows.Scan(
				&o.ID, &o.PublicID, &o.OrderNumber, &o.CustomerID, &o.OrganizationID,
				&offerID, &o.BranchID, &o.VendorBranchID, &o.UserAddressID,
				&statusStr, &o.Subtotal, &o.DiscountAmount, &o.TotalDiscount, &o.ShippingFee,
				&o.TaxAmount, &o.TotalAmount, &o.FinalPrice,
				&o.PaymentMethod, &payStatusStr, &o.Notes,
				&o.IsNegotiation, &o.NegotiationStatus, &o.NegotiationNotes,
				&o.Rating, &o.Review, &o.RatedAt, &o.DeliveredAt,
				&o.CreatedAt, &o.UpdatedAt, &o.DeletedAt,
				&o.CustomerOrgName, &o.CustomerBranchName, &o.CustomerCityName,
				&o.VendorOrgName, &o.VendorBranchName, &o.ItemsCount,
			); err != nil {
				return err
			}
			if offerID != nil {
				o.OfferID = *offerID
			}
			o.Status = commerce.OrderStatus(statusStr)
			o.PaymentStatus = commerce.PaymentStatus(payStatusStr)
			list = append(list, &o)
		}
		return rows.Err()
	})
	return list, total, err
}

// AdminOrderParties lists the organisations that actually appear on an order,
// as buyers and as sellers.
//
// Both selects used to be filled from every organisation on the platform and
// narrowed in the template by type -- the buyer select on `type == "customer"`,
// which silently excluded every supplier that had bought from another supplier.
// Deriving the lists from the orders themselves cannot exclude a real buyer,
// and it is a shorter list.
func (r *Repository) AdminOrderParties(ctx context.Context) (commerce.AdminOrderParties, error) {
	var out commerce.AdminOrderParties
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const partyName = `COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.name->>'ar', ''),
		                            NULLIF(o.trade_name->>'en', ''), NULLIF(o.name->>'en', ''),
		                            o.legal_name, '')`

		buyerRows, err := tx.Query(txCtx, `
			SELECT o.id, `+partyName+`, COALESCE(o.type, ''), count(*)
			FROM commerce.orders ord
			JOIN org.organizations o ON o.id = ord.organization_id
			WHERE ord.deleted_at IS NULL
			GROUP BY o.id, o.trade_name, o.name, o.legal_name, o.type
			ORDER BY 2;`)
		if err != nil {
			return err
		}
		for buyerRows.Next() {
			var p commerce.AdminOrderParty
			if err := buyerRows.Scan(&p.ID, &p.Name, &p.Type, &p.Count); err != nil {
				buyerRows.Close()
				return err
			}
			out.Buyers = append(out.Buyers, p)
		}
		buyerRows.Close()
		if err := buyerRows.Err(); err != nil {
			return err
		}

		// A seller appears on the shipment rather than on the order: one order
		// can be split across several suppliers.
		sellerRows, err := tx.Query(txCtx, `
			SELECT o.id, `+partyName+`, COALESCE(o.type, ''), count(DISTINCT sh.order_id)
			FROM commerce.order_shipments sh
			JOIN org.organizations o ON o.id = sh.organization_id
			JOIN commerce.orders ord ON ord.id = sh.order_id AND ord.deleted_at IS NULL
			GROUP BY o.id, o.trade_name, o.name, o.legal_name, o.type
			ORDER BY 2;`)
		if err != nil {
			return err
		}
		defer sellerRows.Close()
		for sellerRows.Next() {
			var p commerce.AdminOrderParty
			if err := sellerRows.Scan(&p.ID, &p.Name, &p.Type, &p.Count); err != nil {
				return err
			}
			out.Sellers = append(out.Sellers, p)
		}
		return sellerRows.Err()
	})
	if err != nil {
		return commerce.AdminOrderParties{}, fmt.Errorf("commerce postgres: admin order parties: %w", err)
	}
	return out, nil
}
