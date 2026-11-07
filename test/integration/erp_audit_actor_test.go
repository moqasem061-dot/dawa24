package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

func TestAuditActorKeyRoundTrip(t *testing.T) {
	db := connectTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	defer db.Close()
	ctx := context.Background()
	keyID := time.Now().UnixNano() / 1000
	entityID := fmt.Sprint(keyID)
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		for _, entry := range []database.AuditEntry{
			{Action: "erp.test.key", EntityType: "erp_test", EntityID: entityID, ActorKind: "api_key", ActorKeyID: &keyID},
			{Action: "erp.test.default", EntityType: "erp_test", EntityID: entityID},
		} {
			if err := database.WriteAudit(ctx, tx, entry); err != nil {
				return err
			}
		}
		for _, tc := range []struct {
			action, kind string
			key          *int64
		}{
			{"erp.test.key", "api_key", &keyID}, {"erp.test.default", "user", nil},
		} {
			var kind string
			var gotKey *int64
			if err := tx.QueryRow(ctx, `SELECT actor_kind, actor_key_id FROM platform.audit_log WHERE entity_type = 'erp_test' AND entity_id = $1 AND action = $2`, entityID, tc.action).Scan(&kind, &gotKey); err != nil {
				return err
			}
			if kind != tc.kind || (tc.key == nil && gotKey != nil) || (tc.key != nil && (gotKey == nil || *gotKey != *tc.key)) {
				t.Errorf("%s: kind %q, key %v", tc.action, kind, gotKey)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
