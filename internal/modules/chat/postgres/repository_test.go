package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/chat"
	"github.com/muhiya/dawa24-store/internal/modules/chat/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
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
	cfg := config.Database{URL: dbURL, MaxConns: 5, MinConns: 1, MaxConnLifetime: time.Hour, MaxConnIdleTime: 30 * time.Minute, StatementTimeout: 10 * time.Second}
	db, err := database.Open(context.Background(), cfg)
	if err != nil {
		t.Skipf("cannot connect to database: %v", err)
	}
	migrations, err := database.LoadMigrations(dbfs.Migrations, "migrations")
	if err != nil {
		t.Fatalf("failed to load migrations: %v", err)
	}
	if pending, err := db.PendingCount(context.Background(), migrations); err == nil && pending > 0 {
		t.Fatalf("%d migrations pending", pending)
	}
	return db
}

func TestConversationVisibleToBothOrgsAndUnread(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()
	repo := postgres.NewRepository(db)

	// Create two users and two organizations.
	createUser := func(email string) int64 {
		var id int64
		// Re-runnable: the previous run's rows are still there, and a plain
		// INSERT fails the second time on users_email_key. The partial unique
		// index carries WHERE deleted_at IS NULL, so the arbiter needs it too.
		err := db.Pool().QueryRow(ctx, `
			INSERT INTO identity.users (email, password_hash, name, role, status)
			VALUES ($1, 'x', '{"ar":"","en":""}', 'customer', 'active')
			ON CONFLICT (email) WHERE deleted_at IS NULL
			DO UPDATE SET updated_at = now()
			RETURNING id;`, email).Scan(&id)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		return id
	}
	createOrg := func() int64 {
		var id int64
		err := db.Pool().QueryRow(ctx, `INSERT INTO org.organizations (name, type, status) VALUES ('{"ar":"منظمة","en":"Org"}', 'supplier', 'approved') RETURNING id;`).Scan(&id)
		if err != nil {
			t.Fatalf("create org: %v", err)
		}
		return id
	}

	user1 := createUser("chat1@example.com")
	user2 := createUser("chat2@example.com")
	org1 := createOrg()
	org2 := createOrg()

	conv := &chat.Conversation{OrganizationID: org1, CounterpartyOrgID: org2, Subject: i18n.New("استفسار", "Inquiry"), ContextType: chat.ContextGeneral, Status: chat.StatusOpen, CreatedByUserID: user1}
	if err := repo.CreateConversation(ctx, conv); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	// Both sides see it.
	l1, err := repo.ListConversationsForOrg(ctx, org1, 20, 0)
	if err != nil || len(l1) == 0 {
		t.Fatalf("org1 should see the conversation: %v", err)
	}
	l2, err := repo.ListConversationsForOrg(ctx, org2, 20, 0)
	if err != nil || len(l2) == 0 {
		t.Fatalf("org2 should see the conversation: %v", err)
	}

	// Send a message from org1; org2 has one unread.
	if err := repo.SendMessage(ctx, &chat.Message{ConversationID: conv.ID, SenderUserID: user1, Body: "hello"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	unread, err := repo.CountUnread(ctx, org2)
	if err != nil || unread != 1 {
		t.Fatalf("org2 unread = %d (%v), want 1", unread, err)
	}

	// Mark read; org2 unread drops to zero.
	if err := repo.MarkConversationRead(ctx, conv.ID, org2); err != nil {
		t.Fatalf("MarkConversationRead: %v", err)
	}
	unread, _ = repo.CountUnread(ctx, org2)
	if unread != 0 {
		t.Fatalf("org2 unread = %d after mark-read, want 0", unread)
	}
	_ = user2
}
