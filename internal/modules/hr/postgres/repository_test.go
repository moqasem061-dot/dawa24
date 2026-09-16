package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/hr"
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
	testOrgID  int64 = 88400
	testUserID int64 = 88401
)

func resetFixtures(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM hr.work_times WHERE organization_id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete work_times: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.members WHERE organization_id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete members: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM identity.users WHERE id = $1`, testUserID); err != nil {
			return fmt.Errorf("delete users: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organizations WHERE id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete organizations: %w", err)
		}

		if _, err := tx.Exec(txCtx,
			`INSERT INTO org.organizations (id, name) VALUES ($1, '{"ar":"مؤسسة الموارد","en":"HR Test Org"}'::jsonb)
			 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`, testOrgID); err != nil {
			return fmt.Errorf("insert org: %w", err)
		}
		if _, err := tx.Exec(txCtx,
			`INSERT INTO identity.users (id, email, password_hash, name, role)
			 VALUES ($1, 'hr-test@example.com', 'hash', '{"ar":"موظف","en":"HR User"}'::jsonb, 'customer')
			 ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email`, testUserID); err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("resetFixtures: %v", err)
	}
}

func TestHRRepository(t *testing.T) {
	db := getTestDB(t)
	resetFixtures(t, db)

	repo := NewRepository(db)
	ctx := database.WithTenant(context.Background(), testOrgID)

	t.Run("Create and Get Employee", func(t *testing.T) {
		hiredAt := time.Now().UTC().Truncate(time.Second)
		baseSalary, _ := money.Parse("7500.50")
		varSalary, _ := money.Parse("1250.25")

		emp := &hr.Employee{
			OrganizationID: testOrgID,
			UserID:         testUserID,
			EmployeeCode:   "EMP-001",
			JobTitle:       "Pharmacist",
			BaseSalary:     baseSalary,
			VariableSalary: varSalary,
			Status:         "active",
			HiredAt:        &hiredAt,
		}

		if err := repo.CreateEmployee(ctx, emp); err != nil {
			t.Fatalf("CreateEmployee failed: %v", err)
		}
		if emp.ID <= 0 {
			t.Fatalf("expected positive employee ID, got %d", emp.ID)
		}

		got, err := repo.GetEmployeeByID(ctx, emp.ID)
		if err != nil {
			t.Fatalf("GetEmployeeByID failed: %v", err)
		}
		if got.EmployeeCode != "EMP-001" {
			t.Errorf("got code %q, want EMP-001", got.EmployeeCode)
		}
		if got.BaseSalary != baseSalary {
			t.Errorf("got base salary %s, want %s", got.BaseSalary, baseSalary)
		}
		if got.VariableSalary != varSalary {
			t.Errorf("got variable salary %s, want %s", got.VariableSalary, varSalary)
		}
	})

	t.Run("List Employees", func(t *testing.T) {
		list, err := repo.ListEmployees(ctx, 10, 0)
		if err != nil {
			t.Fatalf("ListEmployees failed: %v", err)
		}
		if len(list) == 0 {
			t.Fatal("expected at least one employee in list")
		}
	})

	t.Run("Save and List Work Times", func(t *testing.T) {
		times := []*hr.WorkTime{
			{
				OrganizationID: testOrgID,
				DayNameAr:      "الأحد",
				DayNameEn:      "Sunday",
				OpenTime:       "09:00",
				CloseTime:      "17:00",
				IsClosed:       false,
				SortOrder:      1,
			},
			{
				OrganizationID: testOrgID,
				DayNameAr:      "الجمعة",
				DayNameEn:      "Friday",
				OpenTime:       "00:00",
				CloseTime:      "00:00",
				IsClosed:       true,
				SortOrder:      6,
			},
		}

		if err := repo.SaveWorkTimes(ctx, times); err != nil {
			t.Fatalf("SaveWorkTimes failed: %v", err)
		}

		list, err := repo.ListWorkTimes(ctx)
		if err != nil {
			t.Fatalf("ListWorkTimes failed: %v", err)
		}
		if len(list) < 2 {
			t.Fatalf("expected at least 2 work times, got %d", len(list))
		}
	})
}
