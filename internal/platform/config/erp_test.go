package config

import "testing"

func TestERPConfig(t *testing.T) {
	for _, tc := range []struct {
		name, env, pepper, enabled string
		wantError                  bool
	}{
		{"defaults", "dev", "", "", false},
		{"production defaults", "prod", "", "", false},
		{"short production pepper", "prod", "short", "true", true},
		{"development fallback", "dev", "short", "true", false},
		{"disabled in production", "prod", "", "false", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setMinimalProdEnv(t)
			t.Setenv("APP_ENV", tc.env)
			t.Setenv("ERP_KEY_PEPPER", tc.pepper)
			t.Setenv("ERP_API_ENABLED", tc.enabled)
			t.Setenv("ERP_DEFAULT_RATE_PER_MINUTE", "")
			t.Setenv("ERP_MAX_BATCH_ITEMS", "")
			t.Setenv("WORKER_ERPSYNC", "")
			cfg, err := Load()
			if (err != nil) != tc.wantError {
				t.Fatalf("Load error = %v, want error %v", err, tc.wantError)
			}
			if err != nil {
				return
			}
			if cfg.ERP.Enabled != (tc.enabled == "true") || cfg.ERP.DefaultRate != 600 || cfg.ERP.MaxBatchItems != 50000 {
				t.Errorf("ERP config = %+v", cfg.ERP)
			}
			if tc.env == "dev" && len(cfg.ERP.KeyPepper) < 32 {
				t.Error("development pepper is too short")
			}
			if cfg.Worker.Queues["erpsync"] != 3 {
				t.Errorf("erpsync workers = %d", cfg.Worker.Queues["erpsync"])
			}
		})
	}
}
