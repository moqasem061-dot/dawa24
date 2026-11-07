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
	// An enabled API in any non-development environment must have a real
	// pepper: keys minted with a derivable default would be forgeable by
	// anyone who reads this repository. Disabled deployments need no pepper.
	if cfg.Enabled && len(cfg.KeyPepper) < 32 && !env.IsDev() {
		fail("ERP_KEY_PEPPER must be at least 32 bytes when ERP API is enabled outside development")
	}
	if !env.IsProd() && len(cfg.KeyPepper) < 32 {
		cfg.KeyPepper = "dawa24-development-erp-pepper-not-for-production"
	}
	return cfg
}
