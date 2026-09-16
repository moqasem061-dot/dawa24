package postgres

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	"github.com/jackc/pgx/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// RecordVisitor inserts one visitor-session-day, ignoring repeats.
func (r *Repository) RecordVisitor(ctx context.Context, v *platformadmin.Visitor) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO platform_admin.visitors (visitor_key, ip, user_agent, browser, device, os, country, city, visited_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CURRENT_DATE)
			ON CONFLICT (visitor_key, visited_at) DO UPDATE 
			SET country = EXCLUDED.country, city = EXCLUDED.city, ip = EXCLUDED.ip;
		`
		_, err := tx.Exec(txCtx, query, v.VisitorKey, v.IP, v.UserAgent, v.Browser, v.Device, v.OS, v.Country, v.City)
		return err
	})
}

// groupKey renders one grouping-set label, falling back for the NULL that a
// row outside its own grouping set carries.
func groupKey(v *string) string {
	if v == nil || *v == "" {
		return i18n.TDefault("w4_ui.s_178_178")
	}
	return *v
}

// VisitorAnalytics returns the aggregate traffic and platform health view.
func (r *Repository) VisitorAnalytics(ctx context.Context, limit int) (*platformadmin.VisitorAnalytics, error) {
	return r.VisitorAnalyticsWithTotal(ctx, limit, 0)
}

// VisitorAnalyticsWithTotal returns the aggregate traffic and platform health view with pagination.
func (r *Repository) VisitorAnalyticsWithTotal(ctx context.Context, limit, offset int) (*platformadmin.VisitorAnalytics, error) {
	out := &platformadmin.VisitorAnalytics{
		ByCountry: map[string]int{},
		ByCity:    map[string]int{},
		ByDevice:  map[string]int{},
		ByOS:      map[string]int{},
		ByBrowser: map[string]int{},
		Recent:    []*platformadmin.Visitor{},
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// One pass for both figures. They used to be two full scans of the
		// largest table on the platform, run back to back.
		_ = tx.QueryRow(txCtx, `
			SELECT COUNT(*), COUNT(*) FILTER (WHERE visited_at = CURRENT_DATE)
			FROM platform_admin.visitors;`).Scan(&out.Total, &out.Today)

		// Platform business summary stats (strictly from live database tables)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM org.organizations WHERE type IN ('pharmacy', 'customer') AND deleted_at IS NULL;`).Scan(&out.TotalPharmacies)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM org.organizations WHERE type IN ('supplier', 'vendor') AND deleted_at IS NULL;`).Scan(&out.TotalSuppliers)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM catalog.products WHERE deleted_at IS NULL;`).Scan(&out.TotalProducts)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM commerce.orders;`).Scan(&out.TotalOrders)

		var totalGMVCents int64
		_ = tx.QueryRow(txCtx, `SELECT COALESCE(SUM(total_amount), 0) FROM commerce.orders WHERE status NOT IN ('cancelled', 'rejected');`).Scan(&totalGMVCents)
		out.TotalGMV = fmt.Sprintf(i18n.TDefault("w4_mod.2f_425"), float64(totalGMVCents)/100.0)

		// Five breakdowns, one scan.
		//
		// This was five separate GROUP BY statements, each an unbounded
		// sequential scan of the whole visitors table, run one after another
		// inside this transaction — on top of the two COUNT scans above and a
		// full sort below. On a table that grows with every visitor-day on the
		// platform that is most of a minute of database CPU per admin
		// dashboard load, held on one pooled connection out of twenty, which
		// is how one admin signing in came to slow down everybody else and
		// eventually time out into a 502.
		//
		// GROUPING SETS gets all five from a single pass. The results are read
		// into maps, so the ORDER BY those statements carried never survived
		// the trip and is not reproduced here.
		if err := func() error {
			rows, err := tx.Query(txCtx, `
				SELECT GROUPING(v.country), GROUPING(v.city), GROUPING(v.device),
				       GROUPING(v.os), GROUPING(v.browser),
				       v.country, v.city, v.device, v.os, v.browser, COUNT(*)
				FROM (
					SELECT
						CASE WHEN country IN ('شبكة داخلية 🖥️', 'غير محدد', '') THEN 'مصر 🇪🇬' ELSE country END AS country,
						CASE WHEN city IN ('بيئة التطوير (Local)', 'غير محدد', '') THEN 'القاهرة' ELSE city END AS city,
						COALESCE(NULLIF(device, ''), 'كمبيوتر مكتب (Desktop)') AS device,
						COALESCE(NULLIF(os, ''), 'Windows') AS os,
						COALESCE(NULLIF(browser, ''), 'Chrome') AS browser
					FROM platform_admin.visitors
				) v
				GROUP BY GROUPING SETS ((v.country), (v.city), (v.device), (v.os), (v.browser));`)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				// In a grouping set the flag is 0 for the column being grouped
				// on and 1 for every other, so exactly one of these five is the
				// dimension this row belongs to.
				var gCountry, gCity, gDevice, gOS, gBrowser int
				var country, city, device, os, browser *string
				var n int
				if err := rows.Scan(&gCountry, &gCity, &gDevice, &gOS, &gBrowser,
					&country, &city, &device, &os, &browser, &n); err != nil {
					return err
				}
				switch {
				case gCountry == 0:
					out.ByCountry[groupKey(country)] = n
				case gCity == 0:
					out.ByCity[groupKey(city)] = n
				case gDevice == 0:
					out.ByDevice[groupKey(device)] = n
				case gOS == 0:
					out.ByOS[groupKey(os)] = n
				case gBrowser == 0:
					out.ByBrowser[groupKey(browser)] = n
				}
			}
			return rows.Err()
		}(); err != nil {
			return err
		}

		rows, err := tx.Query(txCtx, `
			SELECT id, visitor_key, ip, user_agent, browser, device, os,
			       CASE WHEN country = 'شبكة داخلية 🖥️' OR country = 'غير محدد' OR country = '' THEN 'مصر 🇪🇬' ELSE country END AS country,
			       CASE WHEN city = 'بيئة التطوير (Local)' OR city = 'غير محدد' OR city = '' THEN 'القاهرة' ELSE city END AS city,
			       visited_at, created_at
			FROM platform_admin.visitors ORDER BY created_at DESC, id DESC LIMIT $1 OFFSET $2;`,
			limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v platformadmin.Visitor
			if err := rows.Scan(&v.ID, &v.VisitorKey, &v.IP, &v.UserAgent, &v.Browser, &v.Device, &v.OS, &v.Country, &v.City, &v.VisitedAt, &v.CreatedAt); err != nil {
				return err
			}
			out.Recent = append(out.Recent, &v)
		}
		return rows.Err()
	})
	return out, err
}
