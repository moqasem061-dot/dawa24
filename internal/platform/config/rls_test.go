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

// Connecting the server or worker as a superuser switches row-level security
// off for every table, so production requires a role that honours it.
func TestProductionServerRequiresRLS(t *testing.T) {
	setMinimalProdEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Database.RequireRLS {
		t.Error("a production server does not require row-level security")
	}
}

// migrate runs as the schema owner, which is a superuser by design.
func TestCLIDoesNotRequireRLS(t *testing.T) {
	setMinimalProdEnv(t)
	cfg, err := LoadForCLI()
	if err != nil {
		t.Fatalf("LoadForCLI: %v", err)
	}
	if cfg.Database.RequireRLS {
		t.Error("the CLI requires row-level security, so migrate could not run as the owner")
	}
}

func TestRLSBypassCanBeAllowedExplicitly(t *testing.T) {
	setMinimalProdEnv(t)
	t.Setenv("DB_ALLOW_RLS_BYPASS", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Database.RequireRLS {
		t.Error("DB_ALLOW_RLS_BYPASS=true did not lift the requirement")
	}
}
