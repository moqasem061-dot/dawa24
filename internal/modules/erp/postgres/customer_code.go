package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
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
// this supplier. Codes compare trimmed and case-insensitively (S5 D3), so C-1
// and c-1 are one code. Those codes cannot name a single buyer.
const customerConflictSQL = `
SELECT lower(trim(uo.organization_number))
FROM org.user_organizations uo
WHERE uo.vendor_org_id = $1 AND uo.deleted_at IS NULL AND uo.status IN ('pending', 'approved')
  AND lower(trim(uo.organization_number)) = ANY($2::text[])
GROUP BY lower(trim(uo.organization_number))
HAVING COUNT(DISTINCT COALESCE(uo.customer_org_id::text, 'user:' || uo.user_id::text)) > 1`

// supplierERPSQL is the one copy of "this supplier uses ERP" (plan.md E8 and the
// guard). It returns the supplier ids that have an active integration with an
// active, unrevoked, unexpired key, the same predicates authenticate.go checks.
const supplierERPSQL = `
SELECT i.organization_id
FROM erp.integrations i
WHERE i.organization_id = ANY($1::bigint[]) AND i.status = 'active'
  AND EXISTS (
    SELECT 1 FROM erp.api_keys k
    WHERE k.integration_id = i.id AND k.status = 'active' AND k.revoked_at IS NULL
      AND (k.expires_at IS NULL OR k.expires_at > now())
  )`

// codedSuppliersSQL lists the suppliers for which this buyer has a pending or
// approved link with a non-blank number. The buyer's organization link counts,
// otherwise a user-only link for the buyer, as customerCodeSQL does.
const codedSuppliersSQL = `
SELECT DISTINCT uo.vendor_org_id
FROM org.user_organizations uo
WHERE uo.vendor_org_id = ANY($1::bigint[]) AND uo.deleted_at IS NULL AND uo.status IN ('pending', 'approved')
  AND trim(uo.organization_number) <> ''
  AND (uo.customer_org_id = NULLIF($2::bigint, 0) OR (uo.customer_org_id IS NULL AND uo.user_id = $3::bigint))`

// codeLockSQL serialises every customer-code write for one supplier. The lock is
// transaction-scoped, so it is held until the guard's transaction commits.
const codeLockSQL = `SELECT pg_advisory_xact_lock(hashtextextended('erp-code:' || $1::bigint::text, 0))`

// codeTakenSQL reports whether another customer holds the code with this
// supplier among pending or approved links. The same customer (its organization,
// or for a user-only link its user) may keep the code. excludeLinkID is the link
// being edited, or 0.
const codeTakenSQL = `
SELECT EXISTS (
  SELECT 1 FROM org.user_organizations uo
  WHERE uo.vendor_org_id = $1 AND uo.deleted_at IS NULL AND uo.status IN ('pending', 'approved')
    AND uo.id <> $2
    AND lower(trim(uo.organization_number)) = lower(trim($3))
    AND NOT (CASE WHEN $4::bigint > 0
                  THEN COALESCE(uo.customer_org_id = $4::bigint, false)
                  ELSE uo.customer_org_id IS NULL AND uo.user_id = $5::bigint END))`

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
		if c := strings.ToLower(strings.TrimSpace(*l.code)); c != "" && !seen[c] {
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
	if conflicts[strings.ToLower(c)] {
		return customerCode{UnmappedReason: "code_conflict"}
	}
	return customerCode{ErpCode: c, Mapped: true}
}

// conflictingCodes returns the lower-cased codes that more than one customer uses.
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

// SupplierERPStatusAsSystem is plan.md E8. It answers, for each supplier, only
// whether it uses ERP: the rows read are the integration and key predicates in
// supplierERPSQL, and no key data is returned. It is needed because a buyer's
// tenant cannot see a supplier's erp rows.
func (r *Repository) SupplierERPStatusAsSystem(ctx context.Context, vendorOrgIDs []int64) (map[int64]bool, error) {
	if len(vendorOrgIDs) == 0 {
		return map[int64]bool{}, nil
	}
	var out map[int64]bool
	err := r.db.InReadTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = scanIDSet(ctx, tx, supplierERPSQL, vendorOrgIDs)
		return err
	})
	return out, err
}

// CodedSuppliers returns the suppliers for which the buyer has a customer code.
// It runs under the buyer's tenant; org.user_organizations has no row policy.
func (r *Repository) CodedSuppliers(ctx context.Context, buyerOrgID, buyerUserID int64, vendorOrgIDs []int64) (map[int64]bool, error) {
	var out map[int64]bool
	err := r.db.InReadTx(database.WithTenant(ctx, buyerOrgID), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = scanIDSet(ctx, tx, codedSuppliersSQL, vendorOrgIDs, buyerOrgID, buyerUserID)
		return err
	})
	return out, err
}

// GuardCustomerCode is the customer-code guard (S5 D2). In one transaction it
// takes the supplier's advisory lock, checks whether another customer holds the
// code (only for a supplier that uses ERP and a non-blank code), and when the
// code is free runs write inside that same transaction through database.JoinTx.
// The lock therefore covers both the check and the link write. The supplier's
// rows are read under its own tenant, so no AsSystem is needed here.
func (r *Repository) GuardCustomerCode(ctx context.Context, vendorOrgID int64, customerOrgID *int64, userID int64, code string, excludeLinkID int64, write func(context.Context) error) (bool, error) {
	var customerOrg int64
	if customerOrgID != nil {
		customerOrg = *customerOrgID
	}
	code = strings.TrimSpace(code)
	taken := false
	err := r.db.InTx(database.WithTenant(ctx, vendorOrgID), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, codeLockSQL, vendorOrgID); err != nil {
			return err
		}
		if code != "" {
			using, err := scanIDSet(ctx, tx, supplierERPSQL, []int64{vendorOrgID})
			if err != nil {
				return err
			}
			if using[vendorOrgID] {
				if err := tx.QueryRow(ctx, codeTakenSQL, vendorOrgID, excludeLinkID, code, customerOrg, userID).Scan(&taken); err != nil {
					return err
				}
			}
		}
		if taken {
			return nil
		}
		return database.JoinTx(ctx, tx, write)
	})
	return taken, err
}

// scanIDSet runs a query that returns one bigint column and collects the ids.
func scanIDSet(ctx context.Context, tx pgx.Tx, sql string, args ...any) (map[int64]bool, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
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

// The Postgres repository implements the customer-code port.
var _ erp.CodeRepository = (*Repository)(nil)
