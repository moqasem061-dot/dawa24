package config

import "testing"

func setMinimalProdEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "prod")
	t.Setenv("APP_BASE_URL", "https://dawa24.com")
	t.Setenv("DATABASE_URL", "postgres://dawa24_app:pw@localhost:5432/dawa24_store?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("SESSION_SECURE", "true")
	t.Setenv("TURNSTILE_ENABLED", "false")
}

// A production server refuses a role that bypasses row-level security unless
// DB_REQUIRE_RLS=false explicitly opts out.
func TestProductionServerRequiresRLSByDefault(t *testing.T) {
	setMinimalProdEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Database.RequireRLS {
		t.Error("a production server does not require RLS by default")
	}
	t.Setenv("DB_REQUIRE_RLS", "false")
	if cfg, err = Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Database.RequireRLS {
		t.Error("DB_REQUIRE_RLS=false did not opt out")
	}
}

// migrate runs as the schema owner, which is a superuser by design.
func TestCLIDoesNotRequireRLS(t *testing.T) {
	setMinimalProdEnv(t)
	t.Setenv("DB_REQUIRE_RLS", "true")
	cfg, err := LoadForCLI()
	if err != nil {
		t.Fatalf("LoadForCLI: %v", err)
	}
	if cfg.Database.RequireRLS {
		t.Error("the CLI requires row-level security, so migrate could not run as the owner")
	}
}

// Production refuses to start without SESSION_SECRET; development generates one.
func TestMissingSessionSecret(t *testing.T) {
	setMinimalProdEnv(t)
	t.Setenv("SESSION_SECRET", "")
	if _, err := Load(); err == nil {
		t.Error("production started without SESSION_SECRET")
	}
	t.Setenv("APP_ENV", "dev")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load (dev): %v", err)
	}
	if !cfg.Session.SecretGenerated || len(cfg.Session.Secret) < 32 {
		t.Errorf("dev secret %d bytes, generated=%v", len(cfg.Session.Secret), cfg.Session.SecretGenerated)
	}
}
