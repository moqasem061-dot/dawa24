package platformadmin_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
)

type mockSQLRepo struct {
	platformadmin.Repository
	executedQuery string
	actorID       *int64
	actorName     string
}

func (m *mockSQLRepo) ExecuteSQL(_ context.Context, actorID *int64, actorName, query string) (*platformadmin.SQLQueryResult, error) {
	m.executedQuery = query
	m.actorID = actorID
	m.actorName = actorName

	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return &platformadmin.SQLQueryResult{Error: "استعلام SQL فارغ."}, nil
	}

	upper := strings.ToUpper(trimmed)
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") && !strings.HasPrefix(upper, "EXPLAIN") {
		return &platformadmin.SQLQueryResult{
			Error: "عمليات التعديل أو الحذف أو الإدراج غير مسموحة في لوحة الاستعلامات. يُسمح فقط باستعلامات القراءة (SELECT / EXPLAIN).",
		}, nil
	}

	return &platformadmin.SQLQueryResult{
		Columns:      []string{"id", "name"},
		Rows:         [][]any{{"1", "Aspirin"}},
		RowsAffected: 1,
		DurationMS:   5,
	}, nil
}

func TestHardenedSQLConsolePreFilter(t *testing.T) {
	repo := &mockSQLRepo{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := platformadmin.NewService(repo, logger)
	ctx := context.Background()

	actorID := int64(99)
	actorName := "Dev Agent"

	tests := []struct {
		name      string
		query     string
		wantError bool
		errSubstr string
	}{
		{
			name:      "Valid SELECT query",
			query:     "SELECT * FROM catalog.products LIMIT 10;",
			wantError: false,
		},
		{
			name:      "Valid EXPLAIN query",
			query:     "EXPLAIN ANALYZE SELECT * FROM org.organizations;",
			wantError: false,
		},
		{
			name:      "Valid WITH CTE read query",
			query:     "WITH top_orgs AS (SELECT id, legal_name FROM org.organizations LIMIT 5) SELECT * FROM top_orgs;",
			wantError: false,
		},
		{
			name:      "Rejected INSERT statement",
			query:     "INSERT INTO platform_admin.cities (country_id, name) VALUES (1, '{\"ar\":\"طنطا\"}');",
			wantError: true,
			errSubstr: "غير مسموحة",
		},
		{
			name:      "Rejected UPDATE statement",
			query:     "UPDATE catalog.products SET is_active = true WHERE id = 1;",
			wantError: true,
			errSubstr: "غير مسموحة",
		},
		{
			name:      "Rejected DELETE statement",
			query:     "DELETE FROM identity.users WHERE id = 50;",
			wantError: true,
			errSubstr: "غير مسموحة",
		},
		{
			name:      "Rejected DROP TABLE statement",
			query:     "DROP TABLE org.branches CASCADE;",
			wantError: true,
			errSubstr: "غير مسموحة",
		},
		{
			name:      "Rejected TRUNCATE statement",
			query:     "TRUNCATE platform_admin.sql_logs;",
			wantError: true,
			errSubstr: "غير مسموحة",
		},
		{
			name:      "Rejected ALTER TABLE statement",
			query:     "ALTER TABLE identity.users ADD COLUMN hacked text;",
			wantError: true,
			errSubstr: "غير مسموحة",
		},
		{
			name:      "Empty query rejected",
			query:     "   ",
			wantError: true,
			errSubstr: "فارغ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := svc.ExecuteSQL(ctx, &actorID, actorName, tt.query)
			if err != nil {
				t.Fatalf("unexpected service error: %v", err)
			}
			if tt.wantError {
				if res.Error == "" {
					t.Errorf("expected error containing %q, got empty error", tt.errSubstr)
				} else if !strings.Contains(res.Error, tt.errSubstr) {
					t.Errorf("expected error to contain %q, got %q", tt.errSubstr, res.Error)
				}
			} else {
				if res.Error != "" {
					t.Errorf("expected clean execution, got error %q", res.Error)
				}
				if len(res.Columns) == 0 || len(res.Rows) == 0 {
					t.Errorf("expected rows and columns returned, got %+v", res)
				}
			}
		})
	}
}
