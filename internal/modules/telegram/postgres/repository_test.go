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
	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/modules/telegram"
	"github.com/muhiya/dawa24-store/internal/modules/telegram/postgres"
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

type fixture struct {
	db  *database.DB
	ctx context.Context
	t   *testing.T
}

func (f fixture) scalar(sql string, args ...any) int64 {
	f.t.Helper()
	var id int64
	if err := f.db.Pool().QueryRow(f.ctx, sql, args...).Scan(&id); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func (f fixture) user() int64 {
	return f.scalar(`INSERT INTO identity.users (email, password_hash, name, role, status)
		VALUES ($1, 'x', '{"ar":"مستخدم","en":"User"}', 'customer', 'active') RETURNING id;`,
		fmt.Sprintf("tg-%d@example.test", time.Now().UnixNano()))
}

func (f fixture) org(name string) int64 {
	return f.scalar(`INSERT INTO org.organizations (name, type, status, legal_name, trade_name)
		VALUES ($1, 'customer', 'approved', $2, $1) RETURNING id;`,
		fmt.Sprintf(`{"ar":%q,"en":"Org"}`, name), name)
}

func (f fixture) member(userID, orgID int64) {
	f.scalar(`INSERT INTO org.members (organization_id, user_id, role_key, status, is_active)
		VALUES ($1, $2, 'org_manager', 'active', true) RETURNING id;`, orgID, userID)
}

func (f fixture) notification(userID, orgID int64, perm, title string) int64 {
	return f.scalar(`INSERT INTO notifications.logs (user_id, organization_id, channel, recipient, title, body, status, required_permission, sent_at)
		VALUES ($1, $2, 'in_app', 'user', $3, 'body', 'sent', $4, now()) RETURNING id;`, userID, orgID, title, perm)
}

func randomHash() []byte {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b
}

func TestLinkLifecycleAgainstPostgres(t *testing.T) {
	db := testDB(t)
	ctx := database.AsSystem(context.Background())
	f := fixture{db: db, ctx: ctx, t: t}
	repo := postgres.New(db)

	owner, other := f.user(), f.user()
	org := f.org("صيدلية الاختبار")
	f.member(owner, org)
	tgOwner := time.Now().UnixNano() % 1_000_000_000
	tgOther := tgOwner + 1

	// Codes: stored as hashes, spent once, older ones retired.
	old := randomHash()
	if err := repo.CreateLinkToken(ctx, owner, &org, old, time.Now().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	fresh := randomHash()
	if err := repo.CreateLinkToken(ctx, owner, &org, fresh, time.Now().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.ConsumeLinkToken(ctx, old); !errors.Is(err, telegram.ErrTokenInvalid) {
		t.Fatalf("a superseded code was accepted: %v", err)
	}
	uid, gotOrg, err := repo.ConsumeLinkToken(ctx, fresh)
	if err != nil || uid != owner || gotOrg == nil || *gotOrg != org {
		t.Fatalf("consume = %d,%v,%v", uid, gotOrg, err)
	}
	if _, _, err := repo.ConsumeLinkToken(ctx, fresh); !errors.Is(err, telegram.ErrTokenInvalid) {
		t.Fatal("a code was spent twice")
	}
	if n, _ := repo.CountLinkTokensSince(ctx, owner, time.Now().Add(-time.Minute)); n != 2 {
		t.Fatalf("token count = %d", n)
	}

	// Pending, then confirmed only by its owner.
	link := &telegram.Link{UserID: owner, TelegramUserID: tgOwner, ChatID: tgOwner, Username: "owner", ActiveOrgID: &org}
	if err := repo.CreatePendingLink(ctx, link, time.Now().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ConfirmPendingLink(ctx, other, link.PublicID); !errors.Is(err, telegram.ErrNoPendingLink) {
		t.Fatalf("another user confirmed the link: %v", err)
	}
	confirmed, err := repo.ConfirmPendingLink(ctx, owner, link.PublicID)
	if err != nil || confirmed.Status != telegram.LinkActive || confirmed.ConfirmedAt == nil {
		t.Fatalf("confirm = %+v, %v", confirmed, err)
	}

	// The confirmed Telegram account cannot be claimed by another user's code.
	steal := &telegram.Link{UserID: other, TelegramUserID: tgOwner, ChatID: tgOwner}
	if err := repo.CreatePendingLink(ctx, steal, time.Now().Add(10*time.Minute)); !errors.Is(err, telegram.ErrLinkedElsewhere) {
		t.Fatalf("takeover not refused: %v", err)
	}

	// Re-linking the owner to a second Telegram account retires the first on confirm.
	second := &telegram.Link{UserID: owner, TelegramUserID: tgOther, ChatID: tgOther}
	if err := repo.CreatePendingLink(ctx, second, time.Now().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if cur, _ := repo.CurrentLinkForUser(ctx, owner); cur == nil || cur.ID != second.ID {
		t.Fatal("the settings page should show the pending link first")
	}
	if _, err := repo.ConfirmPendingLink(ctx, owner, second.PublicID); err != nil {
		t.Fatal(err)
	}
	if l, _ := repo.LiveLinkByTelegramUser(ctx, tgOwner); l != nil {
		t.Fatalf("the first Telegram account is still live: %+v", l)
	}

	// Busy lock and update de-duplication.
	if ok, _ := repo.AcquireBusy(ctx, second.ID, time.Now().Add(time.Minute)); !ok {
		t.Fatal("lock not acquired")
	}
	if ok, _ := repo.AcquireBusy(ctx, second.ID, time.Now().Add(time.Minute)); ok {
		t.Fatal("lock acquired twice")
	}
	if err := repo.ReleaseBusy(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	updateID := time.Now().UnixNano()
	if first, _ := repo.MarkUpdateProcessed(ctx, updateID); !first {
		t.Fatal("first update not first")
	}
	if first, _ := repo.MarkUpdateProcessed(ctx, updateID); first {
		t.Fatal("duplicate update accepted")
	}

	// Organisation switch resets the conversation.
	if err := repo.SetMutedCategories(ctx, second.ID, []string{"offers"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetActiveOrganization(ctx, second.ID, &org); err != nil {
		t.Fatal(err)
	}
	ms, err := repo.Memberships(ctx, owner)
	if err != nil || len(ms) != 1 || ms[0].OrgID != org || ms[0].Name != "صيدلية الاختبار" {
		t.Fatalf("memberships = %+v, %v", ms, err)
	}
	if on, err := repo.OffersTopicEnabled(ctx, owner); err != nil || !on {
		t.Fatalf("offers default = %v, %v", on, err)
	}

	// Outbox: candidates after confirmation only, one decision each, lease, report.
	logID := f.notification(owner, org, "pharmacy.order.view", "تحديث حالة الطلب")
	cands, err := repo.NotificationCandidates(ctx, time.Now().Add(-time.Hour), 500)
	if err != nil {
		t.Fatal(err)
	}
	var mine *chatbridge.Candidate
	for i := range cands {
		if cands[i].LogID == logID {
			mine = &cands[i]
		}
	}
	if mine == nil || mine.LinkID != second.ID || mine.OrganizationName != "صيدلية الاختبار" || len(mine.MutedCategories) != 1 {
		t.Fatalf("candidate = %+v", mine)
	}
	dec := []chatbridge.Decision{{LogID: logID, LinkID: second.ID, Category: chatbridge.CategoryOrders, Text: "<b>x</b>"}}
	if err := repo.RecordDecisions(ctx, dec); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordDecisions(ctx, dec); err != nil {
		t.Fatalf("a repeated decision must be ignored, not fail: %v", err)
	}
	if again, _ := repo.NotificationCandidates(ctx, time.Now().Add(-time.Hour), 500); containsLog(again, logID) {
		t.Fatal("a decided notification is still a candidate")
	}

	claimed, err := repo.ClaimDeliveries(ctx, 50, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	out := findOutgoing(claimed, tgOther)
	if out == nil || out.Text != "<b>x</b>" {
		t.Fatalf("claimed = %+v", claimed)
	}
	if reclaimed, _ := repo.ClaimDeliveries(ctx, 50, time.Minute); findOutgoing(reclaimed, tgOther) != nil {
		t.Fatal("a leased delivery was claimed again")
	}
	if err := repo.RetryDelivery(ctx, out.ID, "socket hang up", time.Millisecond, 5); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	retry, _ := repo.ClaimDeliveries(ctx, 50, time.Minute)
	if out = findOutgoing(retry, tgOther); out == nil {
		t.Fatal("a retried delivery was not claimable")
	}
	if err := repo.FailDeliveryAndBlockLink(ctx, out.ID, "Forbidden: bot was blocked by the user"); err != nil {
		t.Fatal(err)
	}
	if l, _ := repo.LiveLinkByTelegramUser(ctx, tgOther); l == nil || l.Status != telegram.LinkBlocked {
		t.Fatalf("link not blocked: %+v", l)
	}

	// System messages are not sent to a blocked link, and revoking drops them.
	if err := repo.EnqueueSystemMessage(ctx, second.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	if c, _ := repo.ClaimDeliveries(ctx, 50, time.Minute); findOutgoing(c, tgOther) != nil {
		t.Fatal("delivered to a blocked link")
	}
	if err := repo.RevokeLink(ctx, other, second.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := repo.LiveLinkByTelegramUser(ctx, tgOther); l == nil {
		t.Fatal("another user revoked the link")
	}
	if err := repo.RevokeLink(ctx, owner, second.ID); err != nil {
		t.Fatal(err)
	}
	var queued int
	_ = db.Pool().QueryRow(ctx, `SELECT count(*) FROM telegram.deliveries WHERE link_id = $1 AND status IN ('queued','leased')`, second.ID).Scan(&queued)
	if queued != 0 {
		t.Fatalf("%d deliveries still queued after revoke", queued)
	}
}

func containsLog(cs []chatbridge.Candidate, logID int64) bool {
	for _, c := range cs {
		if c.LogID == logID {
			return true
		}
	}
	return false
}

func findOutgoing(list []telegram.Outgoing, chatID int64) *telegram.Outgoing {
	for i := range list {
		if list[i].ChatID == chatID {
			return &list[i]
		}
	}
	return nil
}
