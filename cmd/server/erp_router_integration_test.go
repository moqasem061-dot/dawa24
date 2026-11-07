package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/platform/cache"
	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

func TestRouterERPRealWiring(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("database URL not set")
	}
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Fatal("TEST_REDIS_URL is required for real wiring test")
	}
	t.Setenv("APP_ENV", "dev")
	t.Setenv("ERP_API_ENABLED", "true")
	t.Setenv("ERP_KEY_PEPPER", "erp-test-pepper-at-least-32-bytes-long")
	t.Setenv("DATABASE_URL", dbURL)
	t.Setenv("REDIS_URL", redisURL)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	cacheHandle, err := cache.Open(ctx, cfg.Redis, cfg.Env)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cacheHandle.Close() })
	orgID := time.Now().UnixNano() / 1000
	full, prefix, hash, err := erp.GenerateKey("test", []byte(cfg.ERP.KeyPepper))
	if err != nil {
		t.Fatal(err)
	}
	var keyID int64
	err = db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id,name,type,status) VALUES ($1,'{"ar":"اختبار","en":"Test"}','supplier','approved')`, orgID); err != nil {
			return err
		}
		var branch, warehouse, integration int64
		if err := tx.QueryRow(ctx, `INSERT INTO org.branches (organization_id,name) VALUES ($1,'{"ar":"فرع","en":"Branch"}') RETURNING id`, orgID).Scan(&branch); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO inventory.warehouses (organization_id,name) VALUES ($1,'Warehouse') RETURNING id`, orgID).Scan(&warehouse); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO erp.integrations (organization_id,provider) VALUES ($1,$2) RETURNING id`, orgID, provider.DefaultProvider).Scan(&integration); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO erp.api_keys (organization_id,integration_id,name,key_prefix,key_hash,scopes,warehouse_id,branch_id) VALUES ($1,$2,'test',$3,$4,ARRAY['erp.orders.read'],$5,$6) RETURNING id`, orgID, integration, prefix, hash, warehouse, branch).Scan(&keyID)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.InTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM org.organizations WHERE id=$1`, orgID)
			return err
		}); err != nil {
			t.Error(err)
		}
	})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deps := &dependencies{ctx: ctx, db: db, cache: cacheHandle}
	router := newRouter(cfg, log, deps, nil, nil, nil, []database.Migration{})
	request := func(h http.Handler, token, ip string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/erp/v1/health", nil)
		r.RemoteAddr = ip + ":1234"
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	ip := "198.18." + strconv.Itoa(int(orgID>>8&255)) + "." + strconv.Itoa(int(orgID&255))
	if w := request(router, full, ip); w.Code != 200 {
		t.Fatalf("valid health status=%d", w.Code)
	}
	err = db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE erp.api_keys SET status='revoked', revoked_at=now() WHERE organization_id=$1 AND id=$2`, orgID, keyID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if w := request(router, full, ip); w.Code != 401 {
		t.Fatalf("revoked health status=%d", w.Code)
	}
	// The secret may contain '_', so derive it from the parser like
	// handler_test.go does instead of splitting on underscores.
	_, secret, ok := erp.ParseKey(full)
	if !ok {
		t.Fatal("generated key does not parse")
	}
	mutated := []byte(secret)
	if mutated[0] == 'A' {
		mutated[0] = 'B'
	} else {
		mutated[0] = 'A'
	}
	wrong := full[:len(full)-len(secret)] + string(mutated)
	for i := 1; i < erp.AuthFailuresPerIPPerMinute; i++ {
		if w := request(router, wrong, ip); w.Code != 401 {
			t.Fatalf("wrong attempt %d status=%d", i, w.Code)
		}
	}
	if w := request(router, wrong, ip); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("blocked status=%d", w.Code)
	}
	disabled := *cfg
	disabled.ERP.Enabled = false
	disabledRouter := newRouter(&disabled, log, &dependencies{ctx: ctx, db: db, cache: cacheHandle}, nil, nil, nil, []database.Migration{})
	if w := request(disabledRouter, full, ip); w.Code != 404 {
		t.Fatalf("disabled health status=%d", w.Code)
	}
	website := httptest.NewRecorder()
	disabledRouter.ServeHTTP(website, httptest.NewRequest("GET", "/", nil))
	if website.Code == 404 {
		t.Fatal("website route is missing when ERP disabled")
	}
}
func TestERPDefaultRateMatchesLimit(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("ERP_DEFAULT_RATE_PER_MINUTE", "")
	t.Setenv("ERP_MAX_BATCH_ITEMS", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ERP.DefaultRate != erp.DefaultRatePerMinute || cfg.ERP.MaxBatchItems != erp.MaxBatchItems {
		t.Error("ERP defaults do not match module limits")
	}
}
