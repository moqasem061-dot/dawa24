package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// TestSQLConsoleRunsWithoutSuperuserOrSecrets holds the console to its role:
// PostgreSQL itself refuses superuser functions and credential columns, so a
// query the denylist does not anticipate still cannot reach them.
func TestSQLConsoleRunsWithoutSuperuserOrSecrets(t *testing.T) {
	db := getTestDB(t)
	repo := NewRepository(db)
	ctx := database.AsSystem(context.Background())

	refused := map[string]string{
		"superuser-only function":      "SELECT pg_ls_waldir()",
		"password hashes via SELECT *": "SELECT * FROM identity.users LIMIT 1",
		"stored gateway and AI keys":   "SELECT key, value FROM platform_admin.system_settings",
		"MFA secrets":                  "SELECT * FROM identity.user_mfa LIMIT 1",
		"payout account details":       "SELECT destination_details FROM billing.wallet_withdrawals LIMIT 1",
		"write inside a SELECT":        "WITH x AS (DELETE FROM platform_admin.error_logs RETURNING 1) SELECT count(*) FROM x",
	}
	for name, q := range refused {
		res, err := repo.ExecuteSQL(ctx, nil, "test", q)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.Error == "" {
			t.Errorf("%s was not refused: %q", name, q)
		}
	}

	allowed := []string{
		"SELECT id, email, status FROM identity.users LIMIT 1",
		"SELECT count(*) FROM commerce.orders",
		"SELECT id, status, amount FROM billing.wallet_withdrawals LIMIT 1",
	}
	for _, q := range allowed {
		res, err := repo.ExecuteSQL(ctx, nil, "test", q)
		if err != nil || res.Error != "" {
			t.Errorf("%q should run: %v %s", q, err, res.Error)
		}
		if res.Error != "" && strings.Contains(res.Error, "permission denied") {
			t.Errorf("%q hit a missing grant: %s", q, res.Error)
		}
	}
}
