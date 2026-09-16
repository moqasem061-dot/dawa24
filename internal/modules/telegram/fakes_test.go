package telegram

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// fakeRepo is an in-memory Repository with the same outcomes as postgres.
type fakeRepo struct {
	mu          sync.Mutex
	tokens      []fakeToken
	links       []*Link
	busy        map[int64]bool
	processed   map[int64]bool
	memberships map[int64][]chatbridge.Membership
	offersOff   map[int64]bool
	candidates  []chatbridge.Candidate
	decisions   []chatbridge.Decision
	system      []string
	claimed     []Outgoing
	delivered   []int64
	retried     map[int64]time.Duration
	failed      map[int64]string
	blocked     []int64
}

type fakeToken struct {
	userID   int64
	orgID    *int64
	hash     []byte
	expires  time.Time
	consumed bool
	created  time.Time
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		busy: map[int64]bool{}, processed: map[int64]bool{},
		memberships: map[int64][]chatbridge.Membership{}, offersOff: map[int64]bool{},
		retried: map[int64]time.Duration{}, failed: map[int64]string{},
	}
}

func (f *fakeRepo) CountLinkTokensSince(_ context.Context, userID int64, since time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, t := range f.tokens {
		if t.userID == userID && !t.created.Before(since) {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) CreateLinkToken(_ context.Context, userID int64, orgID *int64, hash []byte, expires time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.tokens {
		if f.tokens[i].userID == userID {
			f.tokens[i].expires = time.Now()
		}
	}
	f.tokens = append(f.tokens, fakeToken{userID: userID, orgID: orgID, hash: hash, expires: expires, created: time.Now()})
	return nil
}

func (f *fakeRepo) ConsumeLinkToken(_ context.Context, hash []byte) (int64, *int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.tokens {
		t := &f.tokens[i]
		if bytes.Equal(t.hash, hash) && !t.consumed && time.Now().Before(t.expires) {
			t.consumed = true
			return t.userID, t.orgID, nil
		}
	}
	return 0, nil, ErrTokenInvalid
}

func live(l *Link) bool { return l.Status != LinkRevoked }

func (f *fakeRepo) LiveLinkByTelegramUser(_ context.Context, tg int64) (*Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.links {
		if l.TelegramUserID == tg && live(l) {
			c := *l
			return &c, nil
		}
	}
	return nil, nil
}

func (f *fakeRepo) CurrentLinkForUser(_ context.Context, userID int64) (*Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var found *Link
	for _, l := range f.links {
		if l.UserID != userID || !live(l) {
			continue
		}
		if l.Status == LinkPending {
			c := *l
			return &c, nil
		}
		c := *l
		found = &c
	}
	return found, nil
}

func (f *fakeRepo) CreatePendingLink(_ context.Context, l *Link, confirmBy time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.links {
		if e.TelegramUserID == l.TelegramUserID && live(e) {
			if e.Status != LinkPending && e.UserID != l.UserID {
				return ErrLinkedElsewhere
			}
			if e.Status == LinkPending {
				e.Status = LinkRevoked
			}
		}
		if e.UserID == l.UserID && e.Status == LinkPending {
			e.Status = LinkRevoked
		}
	}
	l.ID = int64(len(f.links) + 1)
	l.PublicID = fmt.Sprintf("link-%d", l.ID)
	l.Status = LinkPending
	l.ConfirmExpiresAt = &confirmBy
	c := *l
	f.links = append(f.links, &c)
	return nil
}

func (f *fakeRepo) ConfirmPendingLink(_ context.Context, userID int64, publicID string) (*Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var target *Link
	for _, l := range f.links {
		if l.UserID == userID && l.PublicID == publicID && l.Status == LinkPending && time.Now().Before(*l.ConfirmExpiresAt) {
			target = l
		}
	}
	if target == nil {
		return nil, ErrNoPendingLink
	}
	for _, l := range f.links {
		if l.UserID == userID && l != target && (l.Status == LinkActive || l.Status == LinkBlocked) {
			l.Status = LinkRevoked
		}
	}
	now := time.Now()
	target.Status, target.ConfirmedAt = LinkActive, &now
	c := *target
	return &c, nil
}

func (f *fakeRepo) byID(id int64) *Link {
	for _, l := range f.links {
		if l.ID == id {
			return l
		}
	}
	return nil
}

func (f *fakeRepo) RevokeLink(_ context.Context, userID, linkID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if l := f.byID(linkID); l != nil && l.UserID == userID {
		l.Status = LinkRevoked
	}
	return nil
}

func (f *fakeRepo) SetLinkStatus(_ context.Context, linkID int64, status LinkStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if l := f.byID(linkID); l != nil && (l.Status == LinkActive || l.Status == LinkBlocked) {
		l.Status = status
	}
	return nil
}

func (f *fakeRepo) TouchLink(context.Context, int64, int64, string, string) error { return nil }

func (f *fakeRepo) SetActiveOrganization(_ context.Context, linkID int64, orgID *int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if l := f.byID(linkID); l != nil {
		l.ActiveOrgID, l.ConversationID = orgID, nil
	}
	return nil
}

func (f *fakeRepo) SetConversation(_ context.Context, linkID int64, id *int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if l := f.byID(linkID); l != nil {
		l.ConversationID = id
	}
	return nil
}

func (f *fakeRepo) SetMutedCategories(_ context.Context, linkID int64, muted []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if l := f.byID(linkID); l != nil {
		l.MutedCategories = muted
	}
	return nil
}

func (f *fakeRepo) AcquireBusy(_ context.Context, linkID int64, _ time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busy[linkID] {
		return false, nil
	}
	f.busy[linkID] = true
	return true, nil
}

func (f *fakeRepo) ReleaseBusy(_ context.Context, linkID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.busy, linkID)
	return nil
}

func (f *fakeRepo) MarkUpdateProcessed(_ context.Context, id int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.processed[id] {
		return false, nil
	}
	f.processed[id] = true
	return true, nil
}

func (f *fakeRepo) Memberships(_ context.Context, userID int64) ([]chatbridge.Membership, error) {
	return f.memberships[userID], nil
}

func (f *fakeRepo) OffersTopicEnabled(_ context.Context, userID int64) (bool, error) {
	return !f.offersOff[userID], nil
}

func (f *fakeRepo) NotificationCandidates(context.Context, time.Time, int) ([]chatbridge.Candidate, error) {
	return f.candidates, nil
}

func (f *fakeRepo) RecordDecisions(_ context.Context, d []chatbridge.Decision) error {
	f.decisions = append(f.decisions, d...)
	return nil
}

func (f *fakeRepo) EnqueueSystemMessage(_ context.Context, _ int64, text string) error {
	f.system = append(f.system, text)
	return nil
}

func (f *fakeRepo) ClaimDeliveries(context.Context, int, time.Duration) ([]Outgoing, error) {
	return f.claimed, nil
}

func (f *fakeRepo) MarkDelivered(_ context.Context, id int64) error {
	f.delivered = append(f.delivered, id)
	return nil
}

func (f *fakeRepo) RetryDelivery(_ context.Context, id int64, _ string, after time.Duration, max int) error {
	if max == 0 {
		f.failed[id] = "unretryable"
		return nil
	}
	f.retried[id] = after
	return nil
}

func (f *fakeRepo) FailDeliveryAndBlockLink(_ context.Context, id int64, errText string) error {
	f.failed[id] = errText
	f.blocked = append(f.blocked, id)
	return nil
}

func (f *fakeRepo) PurgeProcessedUpdates(context.Context, time.Time) error { return nil }

// fakeGrants answers Resolve from a table keyed by user and organisation.
type fakeGrants map[[2]int64]rbac.Grant

func (g fakeGrants) Resolve(_ context.Context, userID, orgID int64) (rbac.Grant, error) {
	if gr, ok := g[[2]int64{userID, orgID}]; ok {
		return gr, nil
	}
	// Unknown pair: an active user with no membership there, like the resolver.
	return rbac.Grant{UserID: userID, OrganizationID: orgID, Active: true}, nil
}

func memberGrant(userID, orgID int64, orgType, status string, keys ...string) rbac.Grant {
	scope, _ := rbac.TenantScopeFor(orgType)
	return rbac.Grant{
		UserID: userID, OrganizationID: orgID, Active: true, Scope: scope,
		OrgType: rbac.NormalizeOrgType(orgType), OrgStatus: status,
		Keys: keys, Permissions: rbac.NewSet(keys), Name: "صيدلي",
	}
}

// fakeAssistant records what the bridge asked and applies a gate like the
// real one: the named permission must be held.
type fakeAssistant struct {
	gate     string
	limited  bool
	answer   chatbridge.Answer
	calls    int
	lastActr authctx.Actor
	lastCtx  context.Context
	lastConv int64
	lastQ    string
	lastRefs []string

	decisions    []string
	decideActor  authctx.Actor
	decideCtx    context.Context
	decideResult chatbridge.ActionReply
	exports      map[string]*chatbridge.ExportFile
}

func (a *fakeAssistant) Decide(ctx context.Context, actor authctx.Actor, confirm bool, id string) chatbridge.ActionReply {
	verb := "cancel"
	if confirm {
		verb = "confirm"
	}
	a.decisions = append(a.decisions, verb+":"+id)
	a.decideActor, a.decideCtx = actor, ctx
	return a.decideResult
}

func (a *fakeAssistant) Export(_ context.Context, token string) (*chatbridge.ExportFile, error) {
	return a.exports[token], nil
}

func (a *fakeAssistant) Allowed(actor authctx.Actor) bool { return actor.Can(a.gate) }
func (a *fakeAssistant) AllowQuestion(int64) bool         { return !a.limited }
func (a *fakeAssistant) Ask(ctx context.Context, actor authctx.Actor, conv int64, q string, refs ...string) chatbridge.Answer {
	a.calls++
	a.lastActr, a.lastCtx, a.lastConv = actor, ctx, conv
	a.lastQ = q
	a.lastRefs = refs
	return a.answer
}

func (a *fakeAssistant) IngestAttachment(_ context.Context, _ authctx.Actor, filename string, _ []byte) (string, error) {
	if filename == "" {
		filename = "file"
	}
	return "att-" + filename, nil
}

func newTestService(repo *fakeRepo, grants fakeGrants, asst *fakeAssistant) *Service {
	return NewService(repo, grants, asst, Config{BotUsername: "Dawa24Bot", BaseURL: "https://dawa24.test"}, nil)
}

// tenantOf reports the tenant bound to a context the way the database layer
// reads it.
func tenantOf(ctx context.Context) (int64, bool) { return database.TenantFrom(ctx) }
