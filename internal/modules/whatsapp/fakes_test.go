package whatsapp

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// fakeRepo is an in-memory Repository with the same outcomes as postgres.
type fakeRepo struct {
	mu          sync.Mutex
	tokens      []fakeToken
	links       []*Link
	busy        map[int64]bool
	processed   map[string]bool
	memberships map[int64][]chatbridge.Membership
	candidates  []chatbridge.Candidate
	decisions   []chatbridge.Decision
	system      []string
	leased      []LeasedDelivery
	dropped     map[int64]string
	delivered   []int64
	retried     map[int64]time.Duration
	failed      map[int64]string
	blocked     []int64
	windowShut  []int64
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
		busy: map[int64]bool{}, processed: map[string]bool{},
		memberships: map[int64][]chatbridge.Membership{}, dropped: map[int64]string{},
		retried: map[int64]time.Duration{}, failed: map[int64]string{},
	}
}

func (f *fakeRepo) CountLinkTokensSince(_ context.Context, userID int64, since time.Time) (int, error) {
	n := 0
	for _, t := range f.tokens {
		if t.userID == userID && !t.created.Before(since) {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) CreateLinkToken(_ context.Context, userID int64, orgID *int64, hash []byte, expires time.Time) error {
	f.tokens = append(f.tokens, fakeToken{userID: userID, orgID: orgID, hash: hash, expires: expires, created: time.Now()})
	return nil
}

func (f *fakeRepo) ConsumeLinkToken(_ context.Context, hash []byte) (int64, *int64, error) {
	for i := range f.tokens {
		t := &f.tokens[i]
		if bytes.Equal(t.hash, hash) && !t.consumed && time.Now().Before(t.expires) {
			t.consumed = true
			return t.userID, t.orgID, nil
		}
	}
	return 0, nil, ErrTokenInvalid
}

func (f *fakeRepo) LiveLinkByWAID(_ context.Context, waID string) (*Link, error) {
	for _, l := range f.links {
		if l.WAID == waID && l.Status != LinkRevoked {
			c := *l
			return &c, nil
		}
	}
	return nil, nil
}

func (f *fakeRepo) CurrentLinkForUser(_ context.Context, userID int64) (*Link, error) {
	var found *Link
	for _, l := range f.links {
		if l.UserID != userID || l.Status == LinkRevoked {
			continue
		}
		c := *l
		if l.Status == LinkPending {
			return &c, nil
		}
		found = &c
	}
	return found, nil
}

func (f *fakeRepo) CreatePendingLink(_ context.Context, l *Link, confirmBy time.Time) error {
	for _, e := range f.links {
		if e.WAID == l.WAID && e.Status != LinkRevoked {
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
	for _, l := range f.links {
		if l.UserID == userID && l.PublicID == publicID && l.Status == LinkPending && time.Now().Before(*l.ConfirmExpiresAt) {
			now := time.Now()
			l.Status, l.ConfirmedAt = LinkActive, &now
			c := *l
			return &c, nil
		}
	}
	return nil, ErrNoPendingLink
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
	if l := f.byID(linkID); l != nil && l.UserID == userID {
		l.Status = LinkRevoked
	}
	return nil
}

func (f *fakeRepo) SetLinkStatus(_ context.Context, linkID int64, status LinkStatus) error {
	if l := f.byID(linkID); l != nil && (l.Status == LinkActive || l.Status == LinkBlocked) {
		l.Status = status
	}
	return nil
}

func (f *fakeRepo) TouchLink(context.Context, int64, string) error { return nil }

func (f *fakeRepo) MarkMessageProcessed(_ context.Context, id string) (bool, error) {
	if f.processed[id] {
		return false, nil
	}
	f.processed[id] = true
	return true, nil
}

func (f *fakeRepo) SetActiveOrganization(_ context.Context, linkID int64, orgID *int64) error {
	if l := f.byID(linkID); l != nil {
		l.ActiveOrgID, l.ConversationID = orgID, nil
	}
	return nil
}

func (f *fakeRepo) SetConversation(_ context.Context, linkID int64, id *int64) error {
	if l := f.byID(linkID); l != nil {
		l.ConversationID = id
	}
	return nil
}

func (f *fakeRepo) SetMutedCategories(_ context.Context, linkID int64, muted []string) error {
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

func (f *fakeRepo) Memberships(_ context.Context, userID int64) ([]chatbridge.Membership, error) {
	return f.memberships[userID], nil
}

func (f *fakeRepo) OffersTopicEnabled(context.Context, int64) (bool, error) { return true, nil }

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

func (f *fakeRepo) ClaimDeliveries(context.Context, int, time.Duration) ([]LeasedDelivery, error) {
	return f.leased, nil
}

func (f *fakeRepo) DropDelivery(_ context.Context, id int64, reason string) error {
	f.dropped[id] = reason
	return nil
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

func (f *fakeRepo) CloseWindowAndRetry(_ context.Context, id int64, _ string, _ int) error {
	f.windowShut = append(f.windowShut, id)
	return nil
}

func (f *fakeRepo) FailDeliveryAndBlockLink(_ context.Context, id int64, errText string) error {
	f.failed[id] = errText
	f.blocked = append(f.blocked, id)
	return nil
}

func (f *fakeRepo) PurgeProcessedMessages(context.Context, time.Time) error { return nil }

// fakeGrants answers Resolve from a table keyed by user and organisation.
type fakeGrants map[[2]int64]rbac.Grant

func (g fakeGrants) Resolve(_ context.Context, userID, orgID int64) (rbac.Grant, error) {
	if gr, ok := g[[2]int64{userID, orgID}]; ok {
		return gr, nil
	}
	return rbac.Grant{UserID: userID, OrganizationID: orgID, Active: true}, nil
}

func memberGrant(userID, orgID int64, keys ...string) rbac.Grant {
	scope, _ := rbac.TenantScopeFor("customer")
	return rbac.Grant{
		UserID: userID, OrganizationID: orgID, Active: true, Scope: scope,
		OrgType: rbac.NormalizeOrgType("customer"), OrgStatus: "approved",
		Keys: keys, Permissions: rbac.NewSet(keys), Name: "صيدلي",
	}
}

// fakeAssistant records what the bridge asked and gates on one permission.
type fakeAssistant struct {
	gate         string
	answer       chatbridge.Answer
	calls        int
	lastActor    authctx.Actor
	lastCtx      context.Context
	decisions    []string
	decideResult chatbridge.ActionReply
	exports      map[string]*chatbridge.ExportFile
}

func (a *fakeAssistant) Allowed(actor authctx.Actor) bool { return actor.Can(a.gate) }
func (a *fakeAssistant) AllowQuestion(int64) bool         { return true }

func (a *fakeAssistant) Ask(ctx context.Context, actor authctx.Actor, _ int64, _ string, _ ...string) chatbridge.Answer {
	a.calls++
	a.lastActor, a.lastCtx = actor, ctx
	return a.answer
}

func (a *fakeAssistant) IngestAttachment(_ context.Context, _ authctx.Actor, _ string, _ []byte) (string, error) {
	return "fake-att-ref", nil
}

func (a *fakeAssistant) Decide(_ context.Context, _ authctx.Actor, confirm bool, id string) chatbridge.ActionReply {
	a.decisions = append(a.decisions, fmt.Sprintf("%t:%s", confirm, id))
	return a.decideResult
}

func (a *fakeAssistant) Export(_ context.Context, token string) (*chatbridge.ExportFile, error) {
	return a.exports[token], nil
}

func newTestService(repo *fakeRepo, grants fakeGrants, asst *fakeAssistant, template string) *Service {
	return NewService(repo, grants, asst, Config{
		BusinessNumber: "201000000000", BaseURL: "https://dawa24.test", NotificationTemplate: template,
	}, nil)
}
