package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

const (
	userA  int64 = 10
	userB  int64 = 20
	tgA    int64 = 5001
	tgB    int64 = 5002
	orgOne int64 = 100
	orgTwo int64 = 200
)

var nextUpdate int64 = 1

func privateText(tg int64, text string) Update {
	nextUpdate++
	return Update{UpdateID: nextUpdate, Message: &Message{
		From: &User{ID: tg, FirstName: "Ali", Username: "ali"},
		Chat: Chat{ID: tg, Type: "private"},
		Text: text,
	}}
}

func activeLink(repo *fakeRepo, userID, tg int64, org *int64) *Link {
	now := time.Now()
	l := &Link{ID: int64(len(repo.links) + 1), UserID: userID, TelegramUserID: tg, ChatID: tg,
		Status: LinkActive, ActiveOrgID: org, ConfirmedAt: &now}
	repo.links = append(repo.links, l)
	return l
}

func int64p(v int64) *int64 { return &v }

func texts(r Reply) string {
	var b strings.Builder
	for _, m := range r.Messages {
		b.WriteString(m.Text + "\n")
	}
	return b.String()
}

func mustHandle(t *testing.T, s *Service, u Update) Reply {
	t.Helper()
	r, err := s.HandleUpdate(context.Background(), u)
	if err != nil {
		t.Fatalf("HandleUpdate: %v", err)
	}
	return r
}

func TestGroupChatsAreIgnoredEntirely(t *testing.T) {
	repo := newFakeRepo()
	activeLink(repo, userA, tgA, int64p(orgOne))
	asst := &fakeAssistant{gate: "pharmacy.assistant.use"}
	s := newTestService(repo, fakeGrants{}, asst)

	u := privateText(tgA, "كم طلب عندي؟")
	u.Message.Chat = Chat{ID: -100123, Type: "supergroup"}
	if r := mustHandle(t, s, u); len(r.Messages) != 0 || asst.calls != 0 {
		t.Fatalf("a group message must get no reply and reach no assistant; got %d messages, %d calls", len(r.Messages), asst.calls)
	}
}

func TestUnlinkedTelegramUserNeverReachesTheAssistant(t *testing.T) {
	repo := newFakeRepo()
	asst := &fakeAssistant{gate: "pharmacy.assistant.use"}
	s := newTestService(repo, fakeGrants{}, asst)

	r := mustHandle(t, s, privateText(tgA, "اعرض طلباتي"))
	if asst.calls != 0 {
		t.Fatal("an unlinked Telegram account reached the assistant")
	}
	if !strings.Contains(texts(r), "اربط حسابك") || strings.Contains(texts(r), "password") {
		t.Fatalf("expected linking instructions, got %q", texts(r))
	}
}

func TestLinkingRequiresBrowserConfirmationAndRevealsNothing(t *testing.T) {
	repo := newFakeRepo()
	grants := fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "customer", "approved", "pharmacy.assistant.use")}
	asst := &fakeAssistant{gate: "pharmacy.assistant.use", answer: chatbridge.Answer{Markdown: "ok"}}
	s := newTestService(repo, grants, asst)

	actor := authctx.FromGrant(grants[[2]int64{userA, orgOne}])
	code, err := s.StartLink(context.Background(), actor)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.TrimPrefix(code.DeepLink, "https://t.me/Dawa24Bot?start=")
	if len(raw) != 43 || strings.ContainsAny(raw, "+/=") {
		t.Fatalf("code must be 43 base64url characters, got %q", raw)
	}
	if string(repo.tokens[0].hash) == raw {
		t.Fatal("the code itself was stored instead of its hash")
	}

	// Whoever opens the code gets a pending link and no account details.
	r := mustHandle(t, s, privateText(tgB, "/start "+raw))
	if strings.Contains(texts(r), "صيدلي") || !strings.Contains(texts(r), "تأكيد الربط") {
		t.Fatalf("pending reply must ask for confirmation and reveal nothing: %q", texts(r))
	}
	mustHandle(t, s, privateText(tgB, "اعرض طلباتي"))
	if asst.calls != 0 {
		t.Fatal("a pending, unconfirmed link reached the assistant")
	}

	// The code is single-use.
	r = mustHandle(t, s, privateText(tgA, "/start "+raw))
	if !strings.Contains(texts(r), "غير صالح") {
		t.Fatalf("a used code must be refused, got %q", texts(r))
	}

	// Only the account owner, signed in, can confirm — and only their own.
	if _, err := s.ConfirmLink(context.Background(), authctx.Actor{UserID: userB}, repo.links[0].PublicID); err != ErrNoPendingLink {
		t.Fatalf("another user confirmed the link: %v", err)
	}
	if _, err := s.ConfirmLink(context.Background(), actor, repo.links[0].PublicID); err != nil {
		t.Fatal(err)
	}
	if len(repo.system) != 1 {
		t.Fatal("confirmation should queue a welcome message")
	}
	mustHandle(t, s, privateText(tgB, "اعرض طلباتي"))
	if asst.calls != 1 {
		t.Fatal("a confirmed link should reach the assistant")
	}
}

func TestLinkCodesAreRateLimited(t *testing.T) {
	repo := newFakeRepo()
	s := newTestService(repo, fakeGrants{}, &fakeAssistant{})
	actor := authctx.Actor{UserID: userA}
	for i := 0; i < maxCodesPerWindow; i++ {
		if _, err := s.StartLink(context.Background(), actor); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.StartLink(context.Background(), actor); err != ErrTooManyCodes {
		t.Fatalf("expected ErrTooManyCodes, got %v", err)
	}
}

func TestTelegramAccountCannotBeTakenOverByAnotherUsersCode(t *testing.T) {
	repo := newFakeRepo()
	activeLink(repo, userA, tgA, int64p(orgOne))
	s := newTestService(repo, fakeGrants{}, &fakeAssistant{})

	code, err := s.StartLink(context.Background(), authctx.Actor{UserID: userB})
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.TrimPrefix(code.DeepLink, "https://t.me/Dawa24Bot?start=")
	r := mustHandle(t, s, privateText(tgA, "/start "+raw))
	if !strings.Contains(texts(r), "مربوط بحساب آخر") {
		t.Fatalf("expected refusal, got %q", texts(r))
	}
	if l, _ := repo.LiveLinkByTelegramUser(context.Background(), tgA); l.UserID != userA || l.Status != LinkActive {
		t.Fatal("the existing link was disturbed")
	}
}

func TestQuestionIsAnsweredWithTheLiveActorAndTenant(t *testing.T) {
	repo := newFakeRepo()
	activeLink(repo, userA, tgA, int64p(orgOne))
	branch := int64(7)
	g := memberGrant(userA, orgOne, "customer", "approved", "pharmacy.assistant.use", "pharmacy.order.view")
	g.BranchID = &branch
	asst := &fakeAssistant{gate: "pharmacy.assistant.use", answer: chatbridge.Answer{Markdown: "**3** طلبات", ConversationID: 42}}
	s := newTestService(repo, fakeGrants{{userA, orgOne}: g}, asst)

	r := mustHandle(t, s, privateText(tgA, "كم طلب؟"))
	if asst.calls != 1 {
		t.Fatalf("assistant calls = %d", asst.calls)
	}
	a := asst.lastActr
	if a.UserID != userA || a.OrgID != orgOne || a.BranchID == nil || *a.BranchID != branch || !a.Can("pharmacy.order.view") {
		t.Fatalf("actor not rebuilt from the grant: %+v", a)
	}
	if tenant, ok := tenantOf(asst.lastCtx); !ok || tenant != orgOne {
		t.Fatalf("assistant context tenant = %d,%v; want %d", tenant, ok, orgOne)
	}
	if database.IsSystem(asst.lastCtx) {
		t.Fatal("the assistant must never run under the system context")
	}
	if ctxActor, _ := authctx.From(asst.lastCtx); ctxActor.UserID != userA {
		t.Fatal("actor not bound to the assistant context")
	}
	if !strings.Contains(texts(r), "<b>3</b>") {
		t.Fatalf("answer not rendered: %q", texts(r))
	}
	if l, _ := repo.LiveLinkByTelegramUser(context.Background(), tgA); l.ConversationID == nil || *l.ConversationID != 42 {
		t.Fatal("conversation id not remembered")
	}
	if repo.busy[1] {
		t.Fatal("busy lock not released")
	}
}

func TestRefusalsMirrorTheBrowserGates(t *testing.T) {
	suspended := memberGrant(userA, orgOne, "customer", "approved", "pharmacy.assistant.use")
	suspended.Active = false
	cases := []struct {
		name string
		g    fakeGrants
		want string
	}{
		{"suspended account", fakeGrants{{userA, orgOne}: suspended}, "غير نشط"},
		{"membership ended", fakeGrants{}, "لم تعد لديك عضوية"},
		{"organisation pending", fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "customer", "pending", "pharmacy.assistant.use")}, "قيد المراجعة"},
		{"organisation suspended", fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "vendor", "suspended", "vendor.assistant.use")}, "موقوفة"},
		{"assistant not granted to role", fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "customer", "approved", "pharmacy.order.view")}, "غير مفعّل لدورك"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			activeLink(repo, userA, tgA, int64p(orgOne))
			asst := &fakeAssistant{gate: "pharmacy.assistant.use"}
			s := newTestService(repo, tc.g, asst)
			r := mustHandle(t, s, privateText(tgA, "اعرض المبيعات"))
			if asst.calls != 0 {
				t.Fatal("refused caller reached the assistant")
			}
			if !strings.Contains(texts(r), tc.want) {
				t.Fatalf("got %q, want it to contain %q", texts(r), tc.want)
			}
		})
	}
}

func TestOneQuestionAtATimePerChat(t *testing.T) {
	repo := newFakeRepo()
	activeLink(repo, userA, tgA, int64p(orgOne))
	asst := &fakeAssistant{gate: "pharmacy.assistant.use"}
	s := newTestService(repo, fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "customer", "approved", "pharmacy.assistant.use")}, asst)
	repo.busy[1] = true

	r := mustHandle(t, s, privateText(tgA, "سؤال ثانٍ"))
	if asst.calls != 0 || !strings.Contains(texts(r), "السابق") {
		t.Fatalf("expected busy reply, got %q (calls %d)", texts(r), asst.calls)
	}
}

func TestRateLimitIsShared(t *testing.T) {
	repo := newFakeRepo()
	activeLink(repo, userA, tgA, int64p(orgOne))
	asst := &fakeAssistant{gate: "pharmacy.assistant.use", limited: true}
	s := newTestService(repo, fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "customer", "approved", "pharmacy.assistant.use")}, asst)
	mustHandle(t, s, privateText(tgA, "سؤال"))
	if asst.calls != 0 {
		t.Fatal("rate-limited question reached the assistant")
	}
}

func TestDuplicateUpdatesAreProcessedOnce(t *testing.T) {
	repo := newFakeRepo()
	activeLink(repo, userA, tgA, int64p(orgOne))
	asst := &fakeAssistant{gate: "pharmacy.assistant.use", answer: chatbridge.Answer{Markdown: "ok"}}
	s := newTestService(repo, fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "customer", "approved", "pharmacy.assistant.use")}, asst)

	u := privateText(tgA, "سؤال")
	mustHandle(t, s, u)
	if r := mustHandle(t, s, u); len(r.Messages) != 0 || asst.calls != 1 {
		t.Fatalf("a redelivered update was answered again (calls %d)", asst.calls)
	}
}

func TestOrganisationSelectionIsLimitedToLiveMemberships(t *testing.T) {
	repo := newFakeRepo()
	activeLink(repo, userA, tgA, nil)
	repo.memberships[userA] = []chatbridge.Membership{
		{OrgID: orgOne, Name: "صيدلية النور", Type: "customer", Status: "approved"},
		{OrgID: orgTwo, Name: "مورد الشفاء", Type: "vendor", Status: "approved"},
	}
	asst := &fakeAssistant{gate: "pharmacy.assistant.use"}
	grants := fakeGrants{
		{userA, orgOne}: memberGrant(userA, orgOne, "customer", "approved", "pharmacy.assistant.use"),
		{userA, orgTwo}: memberGrant(userA, orgTwo, "vendor", "approved", "vendor.order.view"),
	}
	s := newTestService(repo, grants, asst)

	r := mustHandle(t, s, privateText(tgA, "سؤال"))
	if asst.calls != 0 || !strings.Contains(texts(r), "/org") {
		t.Fatalf("with two memberships and no choice the bot must ask; got %q", texts(r))
	}
	if r = mustHandle(t, s, privateText(tgA, "/org 3")); !strings.Contains(texts(r), "غير صحيح") {
		t.Fatalf("out-of-range selection accepted: %q", texts(r))
	}
	if r = mustHandle(t, s, privateText(tgA, "/org 0")); !strings.Contains(texts(r), "غير صحيح") {
		t.Fatalf("a non-staff user selected the platform scope: %q", texts(r))
	}
	mustHandle(t, s, privateText(tgA, "/org 2"))
	if l, _ := repo.LiveLinkByTelegramUser(context.Background(), tgA); l.OrgID() != orgTwo {
		t.Fatalf("active org = %d", l.OrgID())
	}
	// In the vendor organisation this role has no assistant grant.
	r = mustHandle(t, s, privateText(tgA, "سؤال"))
	if asst.calls != 0 || !strings.Contains(texts(r), "غير مفعّل") {
		t.Fatalf("permission of the newly selected org not applied: %q", texts(r))
	}
}

func TestSingleMembershipIsSelectedAutomatically(t *testing.T) {
	repo := newFakeRepo()
	activeLink(repo, userA, tgA, nil)
	repo.memberships[userA] = []chatbridge.Membership{{OrgID: orgOne, Name: "صيدلية", Type: "customer", Status: "approved"}}
	asst := &fakeAssistant{gate: "pharmacy.assistant.use", answer: chatbridge.Answer{Markdown: "ok"}}
	s := newTestService(repo, fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "customer", "approved", "pharmacy.assistant.use")}, asst)
	mustHandle(t, s, privateText(tgA, "سؤال"))
	if asst.calls != 1 || asst.lastActr.OrgID != orgOne {
		t.Fatalf("single membership not selected (calls %d, org %d)", asst.calls, asst.lastActr.OrgID)
	}
}

func TestBlockingTheBotStopsDeliveryAndWritingResumesIt(t *testing.T) {
	repo := newFakeRepo()
	activeLink(repo, userA, tgA, int64p(orgOne))
	s := newTestService(repo, fakeGrants{}, &fakeAssistant{})

	nextUpdate++
	mustHandle(t, s, Update{UpdateID: nextUpdate, MyChatMember: &ChatMemberUpdated{
		Chat: Chat{ID: tgA, Type: "private"}, From: User{ID: tgA},
		NewChatMember: ChatMember{Status: "kicked"},
	}})
	if repo.links[0].Status != LinkBlocked {
		t.Fatalf("status = %s, want blocked", repo.links[0].Status)
	}
	mustHandle(t, s, privateText(tgA, "/help"))
	if repo.links[0].Status != LinkActive {
		t.Fatalf("status = %s, want active after the user wrote", repo.links[0].Status)
	}
}

func TestUnlinkFromChatStopsEverything(t *testing.T) {
	repo := newFakeRepo()
	activeLink(repo, userA, tgA, int64p(orgOne))
	asst := &fakeAssistant{gate: "pharmacy.assistant.use"}
	s := newTestService(repo, fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "customer", "approved", "pharmacy.assistant.use")}, asst)
	mustHandle(t, s, privateText(tgA, "/unlink"))
	mustHandle(t, s, privateText(tgA, "سؤال"))
	if asst.calls != 0 {
		t.Fatal("an unlinked chat reached the assistant")
	}
}

func TestParseCommand(t *testing.T) {
	cases := []struct{ in, cmd, arg string }{
		{"/start abc", "start", "abc"},
		{"/ORG 2", "org", "2"},
		{"/help@Dawa24Bot", "help", ""},
		{"/help@OtherBot", "", ""},
		{"hello /start", "", ""},
	}
	for _, c := range cases {
		cmd, arg := parseCommand(c.in, "dawa24bot")
		if cmd != c.cmd || arg != c.arg {
			t.Errorf("parseCommand(%q) = %q,%q want %q,%q", c.in, cmd, arg, c.cmd, c.arg)
		}
	}
}
