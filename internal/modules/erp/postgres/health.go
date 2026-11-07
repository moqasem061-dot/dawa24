package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

func (r *Repository) HealthData(ctx context.Context, principal erp.Principal) (erp.HealthInfo, error) {
	var info erp.HealthInfo
	err := r.db.InReadTx(database.WithTenant(ctx, principal.OrganizationID), func(ctx context.Context, tx pgx.Tx) error {
		var name []byte
		err := tx.QueryRow(ctx, `SELECT o.name, w.name, k.expires_at, i.status
   FROM erp.api_keys k
   JOIN org.organizations o ON o.id=k.organization_id
   JOIN inventory.warehouses w ON w.id=k.warehouse_id AND w.organization_id=k.organization_id
   JOIN erp.integrations i ON i.id=k.integration_id AND i.organization_id=k.organization_id
   WHERE k.organization_id=$1 AND k.id=$2 AND o.id=$1 AND w.organization_id=$1 AND i.organization_id=$1`, principal.OrganizationID, principal.KeyID).Scan(&name, &info.WarehouseName, &info.ExpiresAt, &info.IntegrationStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			return erp.ErrNotFound
		}
		if err != nil {
			return err
		}
		return json.Unmarshal(name, &info.OrganizationName)
	})
	return info, err
}
