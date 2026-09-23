package config

import (
	"strings"
	"testing"
)

func TestClarityProjectID(t *testing.T) {
	cases := []struct {
		name, id, want, wantErr string
	}{
		{name: "unset disables", id: ""},
		{name: "project id", id: "ymd15b26tc", want: "ymd15b26tc"},
		{name: "trimmed and lowercased", id: "  YMD15B26TC ", want: "ymd15b26tc"},
		{name: "script injection", id: `ymd15b26tc"></script><script>`, wantErr: "CLARITY_PROJECT_ID"},
		{name: "path", id: "../evil", wantErr: "CLARITY_PROJECT_ID"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("APP_ENV", "dev")
			t.Setenv("APP_BASE_URL", "https://dawa24.com")
			t.Setenv("DATABASE_URL", "postgres://u:p@localhost/db")
			t.Setenv("REDIS_URL", "redis://localhost:6379")
			t.Setenv("SESSION_SECRET", strings.Repeat("s", 32))
			t.Setenv("CLARITY_PROJECT_ID", c.id)

			cfg, err := Load()
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("want error containing %q, got %v", c.wantErr, err)
				}
				return
			}
			if err != nil && strings.Contains(err.Error(), "CLARITY") {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg != nil && cfg.Clarity.ProjectID != c.want {
				t.Fatalf("ProjectID = %q, want %q", cfg.Clarity.ProjectID, c.want)
			}
			if cfg != nil && cfg.Clarity.Enabled() != (c.want != "") {
				t.Fatalf("Enabled() = %v for %q", cfg.Clarity.Enabled(), c.want)
			}
		})
	}
}
