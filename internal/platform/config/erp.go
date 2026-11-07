package config

// ERP configures the machine integration API.
type ERP struct {
	Enabled   bool
	KeyPepper string
	// DefaultRate default 600 duplicates erp.DefaultRatePerMinute; keep both, TestERPDefaultRateMatchesLimit in cmd/server/erp_wiring_test.go asserts they match.
	DefaultRate int
	// MaxBatchItems default 50000 duplicates erp.MaxBatchItems; keep both, TestERPDefaultRateMatchesLimit in cmd/server/erp_wiring_test.go asserts they match.
	MaxBatchItems int
}

func loadERP(env Env, fail func(string, ...any)) ERP {
	cfg := ERP{
		Enabled:       getBool("ERP_API_ENABLED", false),
		KeyPepper:     getStr("ERP_KEY_PEPPER", ""),
		DefaultRate:   getInt("ERP_DEFAULT_RATE_PER_MINUTE", 600),
		MaxBatchItems: getInt("ERP_MAX_BATCH_ITEMS", 50000),
	}
	if len(cfg.KeyPepper) < 32 {
		if env.IsProd() && cfg.Enabled {
			fail("ERP_KEY_PEPPER must be at least 32 bytes when ERP API is enabled in production")
		}
		if !env.IsProd() {
			cfg.KeyPepper = "dawa24-development-erp-pepper-not-for-production"
		}
	}
	return cfg
}
