package postgres_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/whatsapp"
	"github.com/muhiya/dawa24-store/internal/modules/whatsapp/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// These tests write fixture rows, so they read TEST_DATABASE_URL only — never
// DATABASE_URL, which on a developer machine is too often a shared database.
func testDB(t *testing.T) *database.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("database integration test")
	}
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := database.Open(context.Background(), config.Database{
		URL: url, MaxConns: 5, MinConns: 1, MaxConnLifetime: time.Hour,
		MaxConnIdleTime: time.Minute, StatementTimeout: 20 * time.Second,
	})
	if err != nil {
		t.Skipf("cannot connect: %v", err)
	}
	migrations, err := database.LoadMigrations(dbfs.Migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if pending, err := db.PendingCount(context.Background(), migrations); err != nil || pending > 0 {
		t.Fatalf("migrations pending: %d (%v)", pending, err)
	}
	return db
}

func scalar(t *testing.T, db *database.DB, ctx context.Context, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := db.Pool().QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func newUser(t *testing.T, db *database.DB, ctx context.Context) int64 {
	return scalar(t, db, ctx, `INSERT INTO identity.users (email, password_hash, name, role, status)
		VALUES ($1, 'x', '{"ar":"مستخدم","en":"User"}', 'customer', 'active') RETURNING id;`,
		fmt.Sprintf("wa-%d@example.test", time.Now().UnixNano()))
}

func randomHash() []byte {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b
}

func find(list []whatsapp.LeasedDelivery, waID string) *whatsapp.LeasedDelivery {
	for i := range list {
		if list[i].WAID == waID {
			return &list[i]
		}
	}
	return nil
}

func TestWhatsAppLinkAndWindowAgainstPostgres(t *testing.T) {
	db := testDB(t)
	ctx := database.AsSystem(context.Background())
	repo := postgres.New(db)

	owner, other := newUser(t, db, ctx), newUser(t, db, ctx)
	waID := fmt.Sprintf("2010%08d", time.Now().UnixNano()%100_000_000)

	hash := randomHash()
	if err := repo.CreateLinkToken(ctx, owner, nil, hash, time.Now().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if uid, _, err := repo.ConsumeLinkToken(ctx, hash); err != nil || uid != owner {
		t.Fatalf("consume = %d, %v", uid, err)
	}
	if _, _, err := repo.ConsumeLinkToken(ctx, hash); !errors.Is(err, whatsapp.ErrTokenInvalid) {
		t.Fatal("a code was spent twice")
	}

	link := &whatsapp.Link{UserID: owner, WAID: waID, DisplayName: "Ali"}
	if err := repo.CreatePendingLink(ctx, link, time.Now().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ConfirmPendingLink(ctx, other, link.PublicID); !errors.Is(err, whatsapp.ErrNoPendingLink) {
		t.Fatalf("another user confirmed the link: %v", err)
	}
	if l, err := repo.ConfirmPendingLink(ctx, owner, link.PublicID); err != nil || l.Status != whatsapp.LinkActive || l.WAID != waID {
		t.Fatalf("confirm = %+v, %v", l, err)
	}
	steal := &whatsapp.Link{UserID: other, WAID: waID}
	if err := repo.CreatePendingLink(ctx, steal, time.Now().Add(10*time.Minute)); !errors.Is(err, whatsapp.ErrLinkedElsewhere) {
		t.Fatalf("takeover not refused: %v", err)
	}

	msgID := fmt.Sprintf("wamid.test.%d", time.Now().UnixNano())
	if first, _ := repo.MarkMessageProcessed(ctx, msgID); !first {
		t.Fatal("first message not first")
	}
	if first, _ := repo.MarkMessageProcessed(ctx, msgID); first {
		t.Fatal("duplicate message accepted")
	}

	// The message that carried the code opened the window.
	if err := repo.EnqueueSystemMessage(ctx, link.ID, "مرحباً"); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimDeliveries(ctx, 50, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	d := find(claimed, waID)
	if d == nil || !d.WindowOpen || d.Text != "مرحباً" {
		t.Fatalf("claimed = %+v", claimed)
	}

	// WhatsApp says the window closed: the retry is claimed as closed.
	if err := repo.CloseWindowAndRetry(ctx, d.ID, "(#131047) Re-engagement message", 5); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	claimed, _ = repo.ClaimDeliveries(ctx, 50, time.Minute)
	if d = find(claimed, waID); d == nil || d.WindowOpen {
		t.Fatalf("after window closed = %+v", d)
	}
	if err := repo.DropDelivery(ctx, d.ID, "window_closed"); err != nil {
		t.Fatal(err)
	}

	// A new inbound message reopens it.
	if err := repo.TouchLink(ctx, link.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.EnqueueSystemMessage(ctx, link.ID, "ثانية"); err != nil {
		t.Fatal(err)
	}
	claimed, _ = repo.ClaimDeliveries(ctx, 50, time.Minute)
	if d = find(claimed, waID); d == nil || !d.WindowOpen {
		t.Fatalf("after touch = %+v", d)
	}
	if l, _ := repo.LiveLinkByWAID(ctx, waID); l == nil || l.DisplayName != "Ali" {
		t.Fatalf("an empty profile name overwrote the stored one: %+v", l)
	}

	if err := repo.FailDeliveryAndBlockLink(ctx, d.ID, "(#131026) Message Undeliverable"); err != nil {
		t.Fatal(err)
	}
	if l, _ := repo.LiveLinkByWAID(ctx, waID); l == nil || l.Status != whatsapp.LinkBlocked {
		t.Fatalf("link not blocked: %+v", l)
	}

	var dropped int
	_ = db.Pool().QueryRow(ctx, `SELECT count(*) FROM whatsapp.deliveries WHERE link_id = $1 AND drop_reason = 'window_closed'`, link.ID).Scan(&dropped)
	if dropped != 1 {
		t.Fatalf("window_closed drops = %d", dropped)
	}
	if err := repo.RevokeLink(ctx, owner, link.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := repo.LiveLinkByWAID(ctx, waID); l != nil {
		t.Fatal("revoked link still live")
	}
}
