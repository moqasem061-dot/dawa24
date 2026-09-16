package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
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
	testOrgID  int64 = 88500
	testUserID int64 = 88501
)

func resetFixtures(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM ingest.import_rows WHERE organization_id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete import_rows: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM ingest.import_sessions WHERE organization_id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete import_sessions: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM platform_admin.documents WHERE organization_id = $1 AND document_type = 'import_file'`, testOrgID); err != nil {
			return fmt.Errorf("delete file_uploads: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM identity.users WHERE id = $1`, testUserID); err != nil {
			return fmt.Errorf("delete users: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organizations WHERE id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete organizations: %w", err)
		}

		if _, err := tx.Exec(txCtx,
			`INSERT INTO org.organizations (id, name) VALUES ($1, '{"ar":"مؤسسة الاستيراد","en":"Ingest Test Org"}'::jsonb)
			 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`, testOrgID); err != nil {
			return fmt.Errorf("insert org: %w", err)
		}
		if _, err := tx.Exec(txCtx,
			`INSERT INTO identity.users (id, email, password_hash, name, role)
			 VALUES ($1, 'ingest-test@example.com', 'hash', '{"ar":"مستورد","en":"Ingest User"}'::jsonb, 'customer')
			 ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email`, testUserID); err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("resetFixtures: %v", err)
	}
}

func TestIngestRepository(t *testing.T) {
	db := getTestDB(t)
	resetFixtures(t, db)

	repo := NewRepository(db)
	ctx := database.WithTenant(context.Background(), testOrgID)

	var uploadID int64
	var sessionID int64

	t.Run("Create and Get File Upload", func(t *testing.T) {
		upload := &ingest.FileUpload{
			OrganizationID: testOrgID,
			UserID:         testUserID,
			Filename:       "catalog_import.xlsx",
			StorageKey:     "uploads/2026/08/catalog_import.xlsx",
			FileSizeBytes:  1048576,
			MimeType:       "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		}

		if err := repo.CreateFileUpload(ctx, upload); err != nil {
			t.Fatalf("CreateFileUpload failed: %v", err)
		}
		if upload.ID <= 0 {
			t.Fatalf("expected positive upload ID, got %d", upload.ID)
		}
		uploadID = upload.ID

		got, err := repo.GetFileUploadByID(ctx, upload.ID)
		if err != nil {
			t.Fatalf("GetFileUploadByID failed: %v", err)
		}
		if got.Filename != upload.Filename {
			t.Errorf("got filename %q, want %q", got.Filename, upload.Filename)
		}
	})

	t.Run("Create and Get Import Session", func(t *testing.T) {
		session := &ingest.ImportSession{
			OrganizationID:     testOrgID,
			FileUploadID:       uploadID,
			Status:             ingest.StatusPending,
			ColumnMapping:      map[string]string{"name": "col_a", "price": "col_b"},
			MinSimilarityScore: 0.85,
			TotalRows:          100,
		}

		if err := repo.CreateImportSession(ctx, session); err != nil {
			t.Fatalf("CreateImportSession failed: %v", err)
		}
		if session.ID <= 0 {
			t.Fatalf("expected positive session ID, got %d", session.ID)
		}
		sessionID = session.ID

		got, err := repo.GetImportSessionByID(ctx, session.ID)
		if err != nil {
			t.Fatalf("GetImportSessionByID failed: %v", err)
		}
		if got.TotalRows != 100 {
			t.Errorf("got total rows %d, want 100", got.TotalRows)
		}
	})

	t.Run("Update Import Session Progress", func(t *testing.T) {
		err := repo.UpdateImportSessionProgress(ctx, sessionID, 50, 45, ingest.StatusProcessing, "")
		if err != nil {
			t.Fatalf("UpdateImportSessionProgress failed: %v", err)
		}

		got, err := repo.GetImportSessionByID(ctx, sessionID)
		if err != nil {
			t.Fatalf("GetImportSessionByID failed: %v", err)
		}
		if got.ProcessedRows != 50 || got.MatchedRows != 45 {
			t.Errorf("got processed=%d matched=%d, want 50/45", got.ProcessedRows, got.MatchedRows)
		}
	})

	t.Run("List Import Sessions", func(t *testing.T) {
		list, err := repo.ListImportSessions(ctx, testOrgID, 10, 0)
		if err != nil {
			t.Fatalf("ListImportSessions failed: %v", err)
		}
		if len(list) == 0 {
			t.Fatal("expected at least one import session in list")
		}
	})

	t.Run("Insert and List Import Rows", func(t *testing.T) {
		rows := []*ingest.ImportRow{
			{
				SessionID:      sessionID,
				OrganizationID: testOrgID,
				RowNumber:      1,
				RawData:        map[string]any{"name": "Panadol 500mg", "price": 50},
				NormalizedName: "panadol 500mg",
				Status:         "pending",
			},
		}

		if err := repo.InsertImportRows(ctx, rows); err != nil {
			t.Fatalf("InsertImportRows failed: %v", err)
		}

		list, err := repo.ListImportRows(ctx, sessionID, "", 10, 0)
		if err != nil {
			t.Fatalf("ListImportRows failed: %v", err)
		}
		if len(list) == 0 {
			t.Fatal("expected at least one import row in list")
		}

		gotRow, err := repo.GetImportRowByID(ctx, list[0].ID)
		if err != nil {
			t.Fatalf("GetImportRowByID failed: %v", err)
		}
		if gotRow.RowNumber != 1 {
			t.Errorf("got row number %d, want 1", gotRow.RowNumber)
		}

		err = repo.UpdateImportRowMatch(ctx, list[0].ID, nil, 0.95, "matched")
		if err != nil {
			t.Fatalf("UpdateImportRowMatch failed: %v", err)
		}
	})
}
