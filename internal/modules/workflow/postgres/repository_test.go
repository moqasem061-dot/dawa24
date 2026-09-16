package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func getTestDB(t *testing.T) *database.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database integration test in short mode")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := config.Database{
		URL:              dbURL,
		MaxConns:         5,
		MinConns:         1,
		MaxConnLifetime:  time.Hour,
		MaxConnIdleTime:  30 * time.Minute,
		StatementTimeout: 10 * time.Second,
	}

	db, err := database.Open(ctx, cfg)
	if err != nil {
		t.Skipf("cannot connect to database: %v", err)
	}

	migrations, err := database.LoadMigrations(dbfs.Migrations, "migrations")
	if err != nil {
		t.Fatalf("failed to load migrations: %v", err)
	}

	pending, err := db.PendingCount(ctx, migrations)
	if err != nil {
		t.Fatalf("cannot read migration state: %v", err)
	}
	if pending > 0 {
		t.Fatalf("%d migrations pending", pending)
	}
	return db
}

const (
	testOrgID    int64 = 88800
	testBranchID int64 = 88802
	testUserID   int64 = 88801
)

func resetFixtures(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM workflow.report_issues WHERE organization_id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete report_issues: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM workflow.weekly_coverages WHERE organization_id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete weekly_coverages: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM workflow.purchase_priority_engines WHERE organization_id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete purchase_priority_engines: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.branches WHERE id = $1`, testBranchID); err != nil {
			return fmt.Errorf("delete branches: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM identity.users WHERE id = $1`, testUserID); err != nil {
			return fmt.Errorf("delete users: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organizations WHERE id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete organizations: %w", err)
		}

		if _, err := tx.Exec(txCtx,
			`INSERT INTO org.organizations (id, name, type) VALUES ($1, '{"ar":"مؤسسة سير العمل","en":"Workflow Test Org"}'::jsonb, 'vendor')
			 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, type = EXCLUDED.type`, testOrgID); err != nil {
			return fmt.Errorf("insert org: %w", err)
		}
		if _, err := tx.Exec(txCtx,
			`INSERT INTO identity.users (id, email, password_hash, name, role)
			 VALUES ($1, 'wf-test@example.com', 'hash', '{"ar":"مستخدم","en":"Workflow User"}'::jsonb, 'user')
			 ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email, role = EXCLUDED.role`, testUserID); err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		if _, err := tx.Exec(txCtx,
			`INSERT INTO org.branches (id, organization_id, name, address)
			 VALUES ($1, $2, '{"ar":"الفرع الرئيسي","en":"Main Branch"}'::jsonb, '123 Test St')
			 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`, testBranchID, testOrgID); err != nil {
			return fmt.Errorf("insert branch: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("resetFixtures: %v", err)
	}
}

func TestWorkflowRepository(t *testing.T) {
	db := getTestDB(t)
	resetFixtures(t, db)

	repo := NewRepository(db)
	ctx := database.WithTenant(context.Background(), testOrgID)

	t.Run("Create and Get Priority Request", func(t *testing.T) {
		orgID := testOrgID
		budget, _ := money.Parse("10000.00")
		req := &workflow.PurchasePriorityRequest{
			UserID:                         testUserID,
			OrganizationID:                 &orgID,
			RequestNumber:                  "PR-2026-001",
			Status:                         "pending",
			PriorityHighestDiscount:        true,
			PriorityLowestPrice:            false,
			PriorityFastestDelivery:        true,
			PriorityPreferredSuppliersOnly: false,
			BudgetConstraint:               budget,
			Parameters:                     map[string]any{"category": "painkillers"},
			Recommendations:                map[string]any{"sku": "PAN-500", "supplier": "PharmaCorp"},
		}

		if err := repo.CreatePriorityRequest(ctx, req); err != nil {
			t.Fatalf("CreatePriorityRequest failed: %v", err)
		}
		if req.ID <= 0 {
			t.Fatalf("expected positive request ID, got %d", req.ID)
		}

		got, err := repo.GetPriorityRequestByID(ctx, req.ID)
		if err != nil {
			t.Fatalf("GetPriorityRequestByID failed: %v", err)
		}
		if got.RequestNumber != "PR-2026-001" {
			t.Errorf("got request number %q, want PR-2026-001", got.RequestNumber)
		}
	})

	t.Run("Save and List Weekly Coverage", func(t *testing.T) {
		cov := &workflow.WeeklyCoverage{
			OrganizationID: testOrgID,
			BranchID:       testBranchID,
			DayOfWeek:      1,
			CoverageFrom:   ptrStr("08:00"),
			CoverageTo:     ptrStr("16:00"),
			Address:        "Zone 1 Route",
			DistanceMeters: 5000,
			IsActive:       true,
		}

		if err := repo.SaveWeeklyCoverage(ctx, cov); err != nil {
			t.Fatalf("SaveWeeklyCoverage failed: %v", err)
		}
		if cov.ID <= 0 {
			t.Fatalf("expected positive coverage ID, got %d", cov.ID)
		}

		list, err := repo.ListWeeklyCoverage(ctx, testBranchID)
		if err != nil {
			t.Fatalf("ListWeeklyCoverage failed: %v", err)
		}
		if len(list) == 0 {
			t.Fatal("expected at least one coverage in list")
		}
	})

	t.Run("Create and Get Issue", func(t *testing.T) {
		orgID := testOrgID
		issue := &workflow.ReportIssue{
			ReportedBy:     testUserID,
			OrganizationID: &orgID,
			IssueType:      "delivery_delay",
			Description:    "Shipment delayed past expected arrival date",
			Status:         "pending",
			Priority:       "high",
		}

		if err := repo.CreateIssue(ctx, issue); err != nil {
			t.Fatalf("CreateIssue failed: %v", err)
		}
		if issue.ID <= 0 {
			t.Fatalf("expected positive issue ID, got %d", issue.ID)
		}

		got, err := repo.GetIssueByID(ctx, issue.ID)
		if err != nil {
			t.Fatalf("GetIssueByID failed: %v", err)
		}
		if got.IssueType != "delivery_delay" {
			t.Errorf("got issue type %q, want delivery_delay", got.IssueType)
		}

		list, err := repo.ListIssues(ctx, 10, 0)
		if err != nil {
			t.Fatalf("ListIssues failed: %v", err)
		}
		if len(list) == 0 {
			t.Fatal("expected at least one issue in list")
		}

		// Test ListIssuesByReporter
		userIssues, err := repo.ListIssuesByReporter(ctx, testUserID, 10, 0)
		if err != nil {
			t.Fatalf("ListIssuesByReporter failed: %v", err)
		}
		if len(userIssues) == 0 {
			t.Fatal("expected at least one issue for testUserID")
		}

		// Test ListIssuesDetailed with search
		detailed, total, err := repo.ListIssuesDetailed(ctx, workflow.ReportIssueFilter{
			Search: "delayed",
			Limit:  10,
		})
		if err != nil {
			t.Fatalf("ListIssuesDetailed failed: %v", err)
		}
		if total == 0 || len(detailed) == 0 {
			t.Fatal("expected detailed search to find the issue")
		}

		// Test GetIssueStats
		stats, err := repo.GetIssueStats(ctx)
		if err != nil {
			t.Fatalf("GetIssueStats failed: %v", err)
		}
		if stats.Total == 0 {
			t.Fatal("expected non-zero total issues in stats")
		}

		// Test UpdateIssueStatus
		if err := repo.UpdateIssueStatus(ctx, issue.ID, "resolved", "Issue resolved in test"); err != nil {
			t.Fatalf("UpdateIssueStatus failed: %v", err)
		}

		reloaded, err := repo.GetIssueByID(ctx, issue.ID)
		if err != nil {
			t.Fatalf("GetIssueByID after update failed: %v", err)
		}
		if reloaded.Status != "resolved" {
			t.Errorf("got status %q, want resolved", reloaded.Status)
		}
		if reloaded.ResponseNotes != "Issue resolved in test" {
			t.Errorf("got notes %q, want %q", reloaded.ResponseNotes, "Issue resolved in test")
		}
	})
}

// ptrStr returns a pointer to s, for the *string coverage window bounds.
func ptrStr(s string) *string { return &s }
