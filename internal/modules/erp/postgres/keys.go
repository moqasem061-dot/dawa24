package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

const keyColumns = `id, public_id::text, organization_id, integration_id, name, key_prefix, scopes, warehouse_id, branch_id, buying_user_id, rate_limit_per_minute, max_batch_items, status, expires_at, last_used_at, last_used_ip::text, revoked_at, revoked_by, revoke_reason, rotated_from_id, created_by, created_at, updated_at`

type scanner interface{ Scan(...any) error }

func scanKey(row scanner) (erp.APIKey, error) {
	var k erp.APIKey
	err := row.Scan(&k.ID, &k.PublicID, &k.OrganizationID, &k.IntegrationID, &k.Name, &k.KeyPrefix, &k.Scopes, &k.WarehouseID, &k.BranchID, &k.BuyingUserID, &k.RateLimitPerMinute, &k.MaxBatchItems, &k.Status, &k.ExpiresAt, &k.LastUsedAt, &k.LastUsedIP, &k.RevokedAt, &k.RevokedBy, &k.RevokeReason, &k.RotatedFromID, &k.CreatedBy, &k.CreatedAt, &k.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return k, erp.ErrNotFound
	}
	return k, err
}
func (r *Repository) InsertKey(ctx context.Context, orgID int64, k erp.NewKey, audit erp.AuditRecord) (erp.APIKey, error) {
	var out erp.APIKey
	err := r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = scanKey(tx.QueryRow(ctx, `INSERT INTO erp.api_keys (organization_id,integration_id,name,key_prefix,key_hash,scopes,warehouse_id,branch_id,buying_user_id,rate_limit_per_minute,max_batch_items,expires_at,rotated_from_id,created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING `+keyColumns, orgID, k.IntegrationID, k.Name, k.KeyPrefix, k.KeyHash, k.Scopes, k.WarehouseID, k.BranchID, k.BuyingUserID, k.RateLimitPerMinute, k.MaxBatchItems, k.ExpiresAt, k.RotatedFromID, k.CreatedBy))
		if err != nil {
			return err
		}
		return writeAudit(ctx, tx, orgID, audit)
	})
	return out, err
}
func (r *Repository) GetKey(ctx context.Context, orgID, keyID int64) (erp.APIKey, error) {
	var k erp.APIKey
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		k, err = scanKey(tx.QueryRow(ctx, `SELECT `+keyColumns+` FROM erp.api_keys WHERE organization_id=$1 AND id=$2`, orgID, keyID))
		return err
	})
	return k, err
}
func (r *Repository) ListKeys(ctx context.Context, orgID int64) ([]erp.APIKey, error) {
	var keys []erp.APIKey
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+keyColumns+` FROM erp.api_keys WHERE organization_id=$1 ORDER BY id`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			k, err := scanKey(rows)
			if err != nil {
				return err
			}
			keys = append(keys, k)
		}
		return rows.Err()
	})
	return keys, err
}
func (r *Repository) updateKey(ctx context.Context, orgID, keyID int64, query string, value any, audit erp.AuditRecord) error {
	return r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, query, orgID, keyID, value)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return erp.ErrNotFound
		}
		return writeAudit(ctx, tx, orgID, audit)
	})
}
func (r *Repository) SetKeyExpiry(ctx context.Context, orgID, keyID int64, at time.Time, audit erp.AuditRecord) error {
	return r.updateKey(ctx, orgID, keyID, `UPDATE erp.api_keys SET expires_at=$3 WHERE organization_id=$1 AND id=$2`, at, audit)
}
func (r *Repository) SetKeyRateLimit(ctx context.Context, orgID, keyID int64, rate int, audit erp.AuditRecord) error {
	return r.updateKey(ctx, orgID, keyID, `UPDATE erp.api_keys SET rate_limit_per_minute=$3 WHERE organization_id=$1 AND id=$2`, rate, audit)
}
func (r *Repository) RevokeKey(ctx context.Context, orgID, keyID int64, reason string, by int64, audit erp.AuditRecord) error {
	return r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE erp.api_keys SET status='revoked', revoked_at=now(), revoked_by=NULLIF($4,0), revoke_reason=$3 WHERE organization_id=$1 AND id=$2`, orgID, keyID, reason, by)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return erp.ErrNotFound
		}
		return writeAudit(ctx, tx, orgID, audit)
	})
}
func (r *Repository) TouchLastUsed(ctx context.Context, orgID, keyID int64, ip string) error {
	return r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE erp.api_keys SET last_used_at=now(), last_used_ip=NULLIF($3,'')::inet WHERE organization_id=$1 AND id=$2 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`, orgID, keyID, ip)
		return err
	})
}

// FindKeyByPrefixAsSystem is the pre-tenant authentication lookup (plan.md E1).
func (r *Repository) FindKeyByPrefixAsSystem(ctx context.Context, prefix string) (erp.KeyAuthRow, bool, error) {
	var out erp.KeyAuthRow
	err := r.db.InReadTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT `+keyColumns+`, key_hash, (SELECT status FROM erp.integrations i WHERE i.id=erp.api_keys.integration_id), (SELECT provider FROM erp.integrations i WHERE i.id=erp.api_keys.integration_id) FROM erp.api_keys WHERE key_prefix=$1`, prefix)
		k := &out.APIKey
		return row.Scan(&k.ID, &k.PublicID, &k.OrganizationID, &k.IntegrationID, &k.Name, &k.KeyPrefix, &k.Scopes, &k.WarehouseID, &k.BranchID, &k.BuyingUserID, &k.RateLimitPerMinute, &k.MaxBatchItems, &k.Status, &k.ExpiresAt, &k.LastUsedAt, &k.LastUsedIP, &k.RevokedAt, &k.RevokedBy, &k.RevokeReason, &k.RotatedFromID, &k.CreatedBy, &k.CreatedAt, &k.UpdatedAt, &out.KeyHash, &out.IntegrationStatus, &out.Provider)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, false, nil
	}
	return out, err == nil, err
}

// ListKeysAsStaff supports staff oversight (plan.md E5).
func (r *Repository) ListKeysAsStaff(ctx context.Context) ([]erp.StaffKeyRow, error) {
	var result []erp.StaffKeyRow
	err := r.db.InReadTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+keyColumns+` FROM erp.api_keys ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			k, err := scanKey(rows)
			if err != nil {
				return err
			}
			result = append(result, erp.StaffKeyRow{Key: k})
		}
		return rows.Err()
	})
	return result, err
}

// RevokeKeyAsStaff supports staff revocation (plan.md E5).
func (r *Repository) RevokeKeyAsStaff(ctx context.Context, keyID int64, reason string, by int64, audit erp.AuditRecord) error {
	return r.db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		var orgID int64
		err := tx.QueryRow(ctx, `UPDATE erp.api_keys SET status='revoked', revoked_at=now(), revoked_by=NULLIF($3,0), revoke_reason=$2 WHERE id=$1 RETURNING organization_id`, keyID, reason, by).Scan(&orgID)
		if errors.Is(err, pgx.ErrNoRows) {
			return erp.ErrNotFound
		}
		if err != nil {
			return err
		}
		return writeAudit(ctx, tx, orgID, audit)
	})
}
