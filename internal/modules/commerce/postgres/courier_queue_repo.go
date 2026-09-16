package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// The dispatch board: which parcels are on which round.
//
// Every query here is scoped to one supplier by organization_id and reads
// through database.AsSystem, the same way the vendor's own shipment list does.
// Row-level security would otherwise refuse a courier who is a member of the
// supplier but whose request carries no tenant for the pharmacy that placed
// the order — and the supplier's own id in the predicate is the boundary that
// matters here, not the buyer's.

// courierQueueClause builds one tab's predicate together with its arguments.
//
// Postgres infers a parameter's type from where it is referenced, so binding
// one the predicate never mentions causes "could not determine data type of
// parameter $N" at run time. Each branch therefore returns only the arguments
// it actually references, with numbering matching its args slice.
func courierQueueClause(f commerce.CourierQueueFilter) (string, []any) {
	switch f.Queue {
	case commerce.CourierQueueCompleted:
		return `s.organization_id = $1 AND s.status IN ('delivered', 'completed') AND s.courier_user_id = $2` + courierQueueSearch,
			[]any{f.VendorOrgID, f.CourierUserID, f.Search}
	case commerce.CourierQueueFailed:
		return `s.organization_id = $1 AND s.status IN ('failed', 'returned', 'cancelled') AND s.courier_user_id = $2` + courierQueueSearch,
			[]any{f.VendorOrgID, f.CourierUserID, f.Search}
	case commerce.CourierQueueUnassigned:
		return `s.organization_id = $1 AND s.status = ANY($2) AND s.courier_user_id IS NULL` + courierQueueSearch,
			[]any{f.VendorOrgID, commerce.CourierOpenStatuses(), f.Search}
	case commerce.CourierQueueAll:
		return `s.organization_id = $1 AND s.status = ANY($2)` + courierQueueSearch,
			[]any{f.VendorOrgID, commerce.CourierOpenStatuses(), f.Search}
	default: // CourierQueueMine
		return `s.organization_id = $1 AND s.status = ANY($2) AND s.courier_user_id = $4` + courierQueueSearch,
			[]any{f.VendorOrgID, commerce.CourierOpenStatuses(), f.Search, f.CourierUserID}
	}
}

// courierQueueOrder is what "deliver this first" means for each tab.
//
// The open queues sort by assignment time ascending, so the parcel a courier
// has been holding longest is at the top of their screen — which is the whole
// point of recording an assignment date. Unassigned parcels have no assignment
// time, so they fall back to when the order arrived, the same rule applied to
// the only timestamp that exists. Completed work reads newest first, because it
// is a receipt rather than a queue.
func courierQueueOrder(q commerce.CourierQueue) string {
	switch q {
	case commerce.CourierQueueCompleted:
		return `ORDER BY COALESCE(s.delivered_at, s.updated_at) DESC, s.id DESC`
	case commerce.CourierQueueFailed:
		return `ORDER BY s.updated_at DESC, s.id DESC`
	case commerce.CourierQueueUnassigned:
		return `ORDER BY s.created_at ASC, s.id ASC`
	default:
		return `ORDER BY COALESCE(s.courier_assigned_at, s.created_at) ASC, s.id ASC`
	}
}

// courierQueueSearch matches the four references a person actually has to hand
// when they are looking for a parcel. An empty term matches everything rather
// than nothing, so one query serves the searched and the unsearched board.
const courierQueueSearch = ` AND ($3::text = '' OR s.shipment_number ILIKE '%' || $3 || '%'
	         OR s.tracking_number ILIKE '%' || $3 || '%'
	         OR ord.order_number ILIKE '%' || $3 || '%'
	         OR COALESCE(cust_org.name->>'ar', '') ILIKE '%' || $3 || '%'
	         OR COALESCE(cust_org.name->>'en', '') ILIKE '%' || $3 || '%')`

// ListCourierQueue reads one page of the dispatch board, with its total.
func (r *Repository) ListCourierQueue(ctx context.Context, f commerce.CourierQueueFilter) ([]*commerce.OrderShipment, int, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 25
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	where, args := courierQueueClause(f)
	// The page adds its own two parameters after whatever the predicate used,
	// so their numbers follow the predicate rather than being fixed.
	pageArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	pagination := fmt.Sprintf("\n\tLIMIT $%d OFFSET $%d;", len(args)+1, len(args)+2)

	var (
		list  []*commerce.OrderShipment
		total int
	)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		countSQL := `
			SELECT count(*)
			  FROM commerce.order_shipments s
			  JOIN commerce.orders ord ON ord.id = s.order_id
			  LEFT JOIN org.organizations cust_org ON cust_org.id = ord.organization_id
			 WHERE ` + where + `;`
		if err := tx.QueryRow(txCtx, countSQL, args...).Scan(&total); err != nil {
			return err
		}

		query := shipmentDetailSelect + "\n\tWHERE " + where + "\n\t" +
			courierQueueOrder(f.Queue) + pagination
		rows, err := tx.Query(txCtx, query, pageArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			s, err := scanShipmentDetail(rows)
			if err != nil {
				return err
			}
			list = append(list, s)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, fmt.Errorf("commerce postgres: list courier queue %q: %w", f.Queue, err)
	}

	ids := make([]int64, 0, len(list))
	for _, s := range list {
		ids = append(ids, s.ID)
	}
	lines, err := r.shipmentLines(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	for _, s := range list {
		s.Lines = lines[s.ID]
	}
	return list, total, nil
}

// CourierQueueCounts returns every tab badge in one round trip.
func (r *Repository) CourierQueueCounts(ctx context.Context, vendorOrgID, courierUserID int64) (commerce.CourierQueueCounts, error) {
	var c commerce.CourierQueueCounts
	open := commerce.CourierOpenStatuses()
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT
			  count(*) FILTER (WHERE s.courier_user_id = $2 AND s.status = ANY($3)),
			  count(*) FILTER (WHERE s.courier_user_id = $2 AND s.status IN ('delivered', 'completed')),
			  count(*) FILTER (WHERE s.courier_user_id = $2 AND s.status IN ('failed', 'returned', 'cancelled')),
			  count(*) FILTER (WHERE s.courier_user_id IS NULL AND s.status = ANY($3)),
			  count(*) FILTER (WHERE s.status = ANY($3)),
			  count(*) FILTER (WHERE s.courier_user_id = $2 AND s.status = ANY($3)
			                     AND s.courier_assigned_at < now() - make_interval(hours => $4))
			FROM commerce.order_shipments s
			WHERE s.organization_id = $1;
		`
		return tx.QueryRow(txCtx, query, vendorOrgID, courierUserID, open, commerce.CourierOverdueHours).
			Scan(&c.Mine, &c.Completed, &c.Failed, &c.Unassigned, &c.All, &c.Overdue)
	})
	if err != nil {
		return commerce.CourierQueueCounts{}, fmt.Errorf("commerce postgres: courier queue counts: %w", err)
	}
	return c, nil
}

// ListCourierWorkload summarises each round for the dispatcher.
//
// It reports only representatives who are actually carrying something or have
// carried something. A supplier's full roster of couriers is an org question,
// answered by the org module; this answers the commerce half — what is on each
// round — and the dispatch screen joins the two.
func (r *Repository) ListCourierWorkload(ctx context.Context, vendorOrgID int64) ([]*commerce.CourierWorkload, error) {
	var list []*commerce.CourierWorkload
	open := commerce.CourierOpenStatuses()
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT s.courier_user_id,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), COALESCE(u.email, '')),
			       COALESCE(u.phone, ''),
			       count(*) FILTER (WHERE s.status = ANY($2))  AS open_count,
			       count(*) FILTER (WHERE s.status <> ALL($2)) AS closed_count,
			       min(s.courier_assigned_at) FILTER (WHERE s.status = ANY($2)) AS oldest
			  FROM commerce.order_shipments s
			  JOIN identity.users u ON u.id = s.courier_user_id
			 WHERE s.organization_id = $1 AND s.courier_user_id IS NOT NULL
			 GROUP BY s.courier_user_id, u.name, u.email, u.phone
			 ORDER BY open_count DESC, oldest ASC NULLS LAST;
		`
		rows, err := tx.Query(txCtx, query, vendorOrgID, open)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var w commerce.CourierWorkload
			if err := rows.Scan(&w.UserID, &w.Name, &w.Phone,
				&w.OpenCount, &w.DeliveredCount, &w.OldestAssignedAt); err != nil {
				return err
			}
			list = append(list, &w)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("commerce postgres: list courier workload: %w", err)
	}
	return list, nil
}
