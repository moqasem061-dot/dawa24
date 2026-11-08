package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
)

// customerCodeSQL is the only copy of the customer-code rule (research D-20).
// It finds, for each order, the supplier's link to the buyer: the buyer's
// organization row first, otherwise a link with no organization for the
// ordering user; approved or pending only; the newest link wins. org.user_organizations
// has no row-level policy, so this runs in any transaction.
const customerCodeSQL = `
SELECT o.order_id, l.organization_number
FROM unnest($2::bigint[], $3::bigint[], $4::bigint[]) AS o(order_id, customer_org_id, customer_id)
LEFT JOIN LATERAL (
    SELECT uo.organization_number
    FROM org.user_organizations uo
    WHERE uo.vendor_org_id = $1 AND uo.deleted_at IS NULL AND uo.status IN ('pending', 'approved')
      AND (uo.customer_org_id = NULLIF(o.customer_org_id, 0)
           OR (uo.customer_org_id IS NULL AND uo.user_id = o.customer_id))
    ORDER BY (uo.customer_org_id IS NOT NULL) DESC, uo.updated_at DESC
    LIMIT 1
) l ON true`

// customerConflictSQL returns the codes that more than one customer uses with
// this supplier. Those codes cannot name a single buyer.
const customerConflictSQL = `
SELECT trim(uo.organization_number)
FROM org.user_organizations uo
WHERE uo.vendor_org_id = $1 AND uo.deleted_at IS NULL AND uo.status IN ('pending', 'approved')
  AND trim(uo.organization_number) = ANY($2::text[])
GROUP BY trim(uo.organization_number)
HAVING COUNT(DISTINCT COALESCE(uo.customer_org_id::text, 'user:' || uo.user_id::text)) > 1`

// orderRef identifies one order's buyer for the code lookup.
type orderRef struct {
	OrderID, CustomerOrgID, CustomerUserID int64
}

// customerCode is the resolved code for one order. An empty ErpCode means the
// order is unmapped and UnmappedReason says why.
type customerCode struct {
	ErpCode, UnmappedReason string
	Mapped                  bool
}

// resolveCustomerCodes maps each order to the customer code the supplier knows
// for its buyer. The checks run in this order: no link (no_link), a link whose
// code is blank (empty_code), a code shared by several customers (code_conflict).
// It runs inside the caller's transaction.
func resolveCustomerCodes(ctx context.Context, tx pgx.Tx, supplierOrgID int64, orders []orderRef) (map[int64]customerCode, error) {
	out := make(map[int64]customerCode, len(orders))
	if len(orders) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(orders))
	orgs := make([]int64, 0, len(orders))
	users := make([]int64, 0, len(orders))
	for _, o := range orders {
		ids = append(ids, o.OrderID)
		orgs = append(orgs, o.CustomerOrgID)
		users = append(users, o.CustomerUserID)
	}
	cursor, err := tx.Query(ctx, customerCodeSQL, supplierOrgID, ids, orgs, users)
	if err != nil {
		return nil, err
	}
	type found struct {
		orderID int64
		code    *string
	}
	var links []found
	for cursor.Next() {
		var f found
		if err := cursor.Scan(&f.orderID, &f.code); err != nil {
			cursor.Close()
			return nil, err
		}
		links = append(links, f)
	}
	cursor.Close()
	if err := cursor.Err(); err != nil {
		return nil, err
	}

	codes := []string{}
	seen := map[string]bool{}
	for _, l := range links {
		if l.code == nil {
			continue
		}
		if c := strings.TrimSpace(*l.code); c != "" && !seen[c] {
			seen[c] = true
			codes = append(codes, c)
		}
	}
	conflicts, err := conflictingCodes(ctx, tx, supplierOrgID, codes)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		out[l.orderID] = classifyCode(l.code, conflicts)
	}
	return out, nil
}

func classifyCode(code *string, conflicts map[string]bool) customerCode {
	if code == nil {
		return customerCode{UnmappedReason: "no_link"}
	}
	c := strings.TrimSpace(*code)
	if c == "" {
		return customerCode{UnmappedReason: "empty_code"}
	}
	if conflicts[c] {
		return customerCode{UnmappedReason: "code_conflict"}
	}
	return customerCode{ErpCode: c, Mapped: true}
}

func conflictingCodes(ctx context.Context, tx pgx.Tx, supplierOrgID int64, codes []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(codes) == 0 {
		return out, nil
	}
	cursor, err := tx.Query(ctx, customerConflictSQL, supplierOrgID, codes)
	if err != nil {
		return nil, err
	}
	defer cursor.Close()
	for cursor.Next() {
		var code string
		if err := cursor.Scan(&code); err != nil {
			return nil, err
		}
		out[code] = true
	}
	return out, cursor.Err()
}

// decodeBilingual reads a bilingual JSONB object. An absent or malformed value
// becomes an empty map, so a bad row shows blank names rather than failing.
func decodeBilingual(raw []byte) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}
