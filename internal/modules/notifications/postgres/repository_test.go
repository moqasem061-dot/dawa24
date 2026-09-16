package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
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
	testOrgID  int64 = 88600
	testUserID int64 = 88601
)

func resetFixtures(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM notifications.logs WHERE user_id = $1`, testUserID); err != nil {
			return fmt.Errorf("delete logs: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM notifications.templates WHERE slug = 'test_template'`); err != nil {
			return fmt.Errorf("delete templates: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM identity.users WHERE id = $1`, testUserID); err != nil {
			return fmt.Errorf("delete users: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organizations WHERE id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete organizations: %w", err)
		}

		if _, err := tx.Exec(txCtx,
			`INSERT INTO org.organizations (id, name) VALUES ($1, '{"ar":"مؤسسة الإشعارات","en":"Notifications Test Org"}'::jsonb)
			 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`, testOrgID); err != nil {
			return fmt.Errorf("insert org: %w", err)
		}
		if _, err := tx.Exec(txCtx,
			`INSERT INTO identity.users (id, email, password_hash, name, role)
			 VALUES ($1, 'notif-test@example.com', 'hash', '{"ar":"مستخدم","en":"Notif User"}'::jsonb, 'customer')
			 ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email`, testUserID); err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		if _, err := tx.Exec(txCtx,
			`INSERT INTO notifications.templates (slug, channel, title, body, is_active)
			 VALUES ('test_template', 'in_app', '{"ar":"مرحبا","en":"Welcome"}'::jsonb, '{"ar":"مرحبا بكم","en":"Welcome to Dawa24"}'::jsonb, true)
			 ON CONFLICT (slug) DO UPDATE SET title = EXCLUDED.title`); err != nil {
			return fmt.Errorf("insert template: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("resetFixtures: %v", err)
	}
}

func TestNotificationsRepository(t *testing.T) {
	db := getTestDB(t)
	resetFixtures(t, db)

	repo := NewRepository(db)
	ctx := context.Background()

	var notifID int64

	t.Run("Create Notification Log", func(t *testing.T) {
		orgID := testOrgID
		now := time.Now().UTC()
		log := &notifications.NotificationLog{
			UserID:         testUserID,
			OrganizationID: &orgID,
			Channel:        notifications.ChannelInApp,
			Recipient:      "notif-test@example.com",
			Title:          "Order Shipped",
			Body:           "Your order is on the way",
			Status:         notifications.StatusDelivered,
			SentAt:         &now,
		}

		if err := repo.CreateLog(ctx, log); err != nil {
			t.Fatalf("CreateLog failed: %v", err)
		}
		if log.ID <= 0 {
			t.Fatalf("expected positive notification ID, got %d", log.ID)
		}
		notifID = log.ID
	})

	t.Run("Get Template By Slug", func(t *testing.T) {
		tpl, err := repo.GetTemplateBySlug(ctx, "test_template")
		if err != nil {
			t.Fatalf("GetTemplateBySlug failed: %v", err)
		}
		if tpl.Slug != "test_template" {
			t.Errorf("got slug %q, want test_template", tpl.Slug)
		}
	})

	t.Run("List User Notifications and Unread Count", func(t *testing.T) {
		list, err := repo.ListUserNotifications(ctx, testUserID, 10, 0)
		if err != nil {
			t.Fatalf("ListUserNotifications failed: %v", err)
		}
		if len(list) == 0 {
			t.Fatal("expected at least one notification in list")
		}

		count, err := repo.GetUnreadCount(ctx, testUserID)
		if err != nil {
			t.Fatalf("GetUnreadCount failed: %v", err)
		}
		if count != 1 {
			t.Errorf("got unread count %d, want 1", count)
		}
	})

	t.Run("Mark As Read", func(t *testing.T) {
		if err := repo.MarkAsRead(ctx, notifID, testUserID); err != nil {
			t.Fatalf("MarkAsRead failed: %v", err)
		}

		count, err := repo.GetUnreadCount(ctx, testUserID)
		if err != nil {
			t.Fatalf("GetUnreadCount after mark as read failed: %v", err)
		}
		if count != 0 {
			t.Errorf("got unread count %d, want 0", count)
		}
	})
}
