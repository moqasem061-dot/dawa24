package whatsapp

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

const (
	userA  int64 = 10
	userB  int64 = 20
	waA          = "201111111111"
	waB          = "201222222222"
	orgOne int64 = 100
)

const proposalID = "0b9f7a52-3c1e-4d7a-9a51-2f6b1c0d8e41"

var nextMessage = 0

func textHook(from, body string) Webhook {
	nextMessage++
	m := InboundMessage{ID: "wamid.text" + strconv.Itoa(nextMessage), From: from, Type: "text"}
	m.Text = &struct {
		Body string `json:"body"`
	}{Body: body}
	return Webhook{WebhookValue: WebhookValue{
		Contacts: []Contact{{WAID: from}},
		Messages: []InboundMessage{m},
	}}
}

func buttonHook(from, id string) Webhook {
	nextMessage++
	raw := `{"entry":[{"changes":[{"field":"messages","value":{"messages":[{"id":"wamid.button` +
		strconv.Itoa(nextMessage) + `","from":"` + from + `","type":"interactive",` +
		`"interactive":{"type":"button_reply","button_reply":{"id":"` + id + `","title":"x"}}}]}}]}]}`
	var w Webhook
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		panic(err)
	}
	return w
}

func activeLink(repo *fakeRepo, userID int64, waID string, org *int64) *Link {
	now := time.Now()
	l := &Link{ID: int64(len(repo.links) + 1), UserID: userID, WAID: waID, Status: LinkActive, ActiveOrgID: org, ConfirmedAt: &now}
	repo.links = append(repo.links, l)
	return l
}

func int64p(v int64) *int64 { return &v }

func handle(t *testing.T, s *Service, w Webhook) Reply {
	t.Helper()
	r, err := s.HandleWebhook(context.Background(), w)
	if err != nil {
		t.Fatalf("HandleWebhook: %v", err)
	}
	return r
}

func bodies(r Reply) string {
	var b strings.Builder
	for _, m := range r.Messages {
		switch {
		case m.Payload != nil && m.Payload.Text != nil:
			b.WriteString(m.Payload.Text.Body + "\n")
		case m.Payload != nil && m.Payload.Interactive != nil:
			b.WriteString(m.Payload.Interactive.Body.Text + "\n")
		}
	}
	return b.String()
}

func pharmacist() (fakeGrants, *fakeAssistant) {
	grants := fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "pharmacy.assistant.use")}
	return grants, &fakeAssistant{gate: "pharmacy.assistant.use", answer: chatbridge.Answer{Markdown: "**3** طلبات", ConversationID: 42}}
}

func TestGroupAndMalformedMessagesAreIgnored(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := pharmacist()
	activeLink(repo, userA, waA, int64p(orgOne))
	s := newTestService(repo, grants, asst, "")

	group := textHook(waA, "كم طلب؟")
	group.Messages[0].GroupID = "120363000000000000"
	for name, w := range map[string]Webhook{"group": group, "bad sender": textHook("+20 111", "hi"), "no messages": {}} {
		if r := handle(t, s, w); len(r.Messages) != 0 {
			t.Errorf("%s: replied %q", name, bodies(r))
		}
	}
	if asst.calls != 0 {
		t.Fatal("assistant reached from an ignored message")
	}
}

func TestUnlinkedNumberNeverReachesTheAssistant(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := pharmacist()
	s := newTestService(repo, grants, asst, "")

	r := handle(t, s, textHook(waA, "كم طلب عندي؟"))
	if asst.calls != 0 || !strings.Contains(bodies(r), "اربط رقمك") {
		t.Fatalf("calls=%d reply=%q", asst.calls, bodies(r))
	}
	if r.Messages[0].To != waA || r.Messages[0].Payload.To != waA || r.Messages[0].Payload.MessagingProduct != "whatsapp" {
		t.Fatalf("reply addressed wrongly: %+v", r.Messages[0])
	}
}

func TestLinkingRequiresBrowserConfirmationAndRevealsNothing(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := pharmacist()
	s := newTestService(repo, grants, asst, "")
	ctx := context.Background()

	code, err := s.StartLink(ctx, authctx.Actor{UserID: userA, OrgID: orgOne})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(code.DeepLink)
	if err != nil || u.Host != "wa.me" || u.Path != "/201000000000" {
		t.Fatalf("deep link %q", code.DeepLink)
	}
	prefilled := u.Query().Get("text")

	r := handle(t, s, textHook(waA, prefilled))
	if got := bodies(r); !strings.Contains(got, "تم استلام طلب الربط") || strings.Contains(got, "@") {
		t.Fatalf("link reply %q", got)
	}
	// Pending: nothing is answered yet.
	handle(t, s, textHook(waA, "كم طلب؟"))
	if asst.calls != 0 {
		t.Fatal("pending link reached the assistant")
	}
	// The same code cannot be spent twice.
	if got := bodies(handle(t, s, textHook(waB, prefilled))); !strings.Contains(got, "غير صالح") {
		t.Fatalf("reused code: %q", got)
	}

	link, _ := s.CurrentLink(ctx, userA)
	if link == nil || link.Status != LinkPending || link.WAID != waA {
		t.Fatalf("pending link %+v", link)
	}
	if _, err := s.ConfirmLink(ctx, authctx.Actor{UserID: userB}, link.PublicID); err == nil {
		t.Fatal("another user confirmed the link")
	}
	if _, err := s.ConfirmLink(ctx, authctx.Actor{UserID: userA}, link.PublicID); err != nil {
		t.Fatal(err)
	}
	if len(repo.system) != 1 {
		t.Fatalf("welcome not queued: %v", repo.system)
	}
	handle(t, s, textHook(waA, "كم طلب؟"))
	if asst.calls != 1 {
		t.Fatal("confirmed link did not reach the assistant")
	}
}

func TestNumberCannotBeTakenOverByAnotherUsersCode(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := pharmacist()
	activeLink(repo, userA, waA, int64p(orgOne))
	s := newTestService(repo, grants, asst, "")

	code, _ := s.StartLink(context.Background(), authctx.Actor{UserID: userB})
	u, _ := url.Parse(code.DeepLink)
	if got := bodies(handle(t, s, textHook(waA, u.Query().Get("text")))); !strings.Contains(got, "مربوط بحساب آخر") {
		t.Fatalf("reply %q", got)
	}
}

func TestQuestionIsAnsweredWithTheLiveActorAndTenant(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := pharmacist()
	activeLink(repo, userA, waA, int64p(orgOne))
	s := newTestService(repo, grants, asst, "")

	r := handle(t, s, textHook(waA, "كم طلب عندي؟"))
	if asst.lastActor.UserID != userA || asst.lastActor.OrgID != orgOne {
		t.Fatalf("actor %+v", asst.lastActor)
	}
	if tenant, ok := database.TenantFrom(asst.lastCtx); !ok || tenant != orgOne {
		t.Fatalf("tenant %d %v", tenant, ok)
	}
	if got := bodies(r); !strings.Contains(got, "*3* طلبات") {
		t.Fatalf("markdown not rendered for WhatsApp: %q", got)
	}
	if id := repo.byID(1).ConversationID; id == nil || *id != 42 {
		t.Fatal("conversation not remembered")
	}
}

func TestRefusalsMirrorTheBrowserGates(t *testing.T) {
	repo := newFakeRepo()
	grants := fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "pharmacy.orders.view")}
	asst := &fakeAssistant{gate: "pharmacy.assistant.use"}
	activeLink(repo, userA, waA, int64p(orgOne))
	s := newTestService(repo, grants, asst, "")

	if got := bodies(handle(t, s, textHook(waA, "سؤال"))); !strings.Contains(got, "غير مفعّل لدورك") || asst.calls != 0 {
		t.Fatalf("calls=%d reply=%q", asst.calls, got)
	}
	g := grants[[2]int64{userA, orgOne}]
	g.OrgStatus = "suspended"
	g.Keys = append(g.Keys, "pharmacy.assistant.use")
	grants[[2]int64{userA, orgOne}] = g
	if got := bodies(handle(t, s, textHook(waA, "سؤال"))); !strings.Contains(got, "موقوفة") || asst.calls != 0 {
		t.Fatalf("suspended org: calls=%d reply=%q", asst.calls, got)
	}
}

func TestDuplicateMessagesAreProcessedOnce(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := pharmacist()
	activeLink(repo, userA, waA, int64p(orgOne))
	s := newTestService(repo, grants, asst, "")

	hook := textHook(waA, "سؤال")
	handle(t, s, hook)
	if r := handle(t, s, hook); len(r.Messages) != 0 || asst.calls != 1 {
		t.Fatalf("redelivered webhook answered again: calls=%d", asst.calls)
	}
}

func TestProposalButtonsDecideForTheLinkedNumberOnly(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := pharmacist()
	asst.answer.Proposals = []chatbridge.AnswerProposal{{ID: proposalID, Title: strings.Repeat("طلب ", 400)}}
	asst.decideResult = chatbridge.ActionReply{Message: "تم التنفيذ.", URL: "/orders/5"}
	activeLink(repo, userA, waA, int64p(orgOne))
	s := newTestService(repo, grants, asst, "")

	r := handle(t, s, textHook(waA, "أنشئ الطلب"))
	card := r.Messages[len(r.Messages)-1].Payload
	if card == nil || card.Interactive == nil || len(card.Interactive.Action.Buttons) != 2 {
		t.Fatalf("no button card: %+v", r.Messages)
	}
	if n := utf8.RuneCountInString(card.Interactive.Body.Text); n > maxInteractiveBody {
		t.Fatalf("interactive body %d runes", n)
	}
	for _, b := range card.Interactive.Action.Buttons {
		if utf8.RuneCountInString(b.Reply.Title) > 20 || len(b.Reply.ID) > 256 {
			t.Fatalf("button over limits: %+v", b)
		}
	}
	confirmID := card.Interactive.Action.Buttons[0].Reply.ID

	// A number that is not linked presses the same button: nothing happens.
	handle(t, s, buttonHook(waB, confirmID))
	// A forged payload decides nothing.
	handle(t, s, buttonHook(waA, "act:c:not-a-uuid"))
	if len(asst.decisions) != 0 {
		t.Fatalf("decided from the wrong place: %v", asst.decisions)
	}

	got := bodies(handle(t, s, buttonHook(waA, confirmID)))
	if len(asst.decisions) != 1 || asst.decisions[0] != "true:"+proposalID {
		t.Fatalf("decisions %v", asst.decisions)
	}
	if !strings.Contains(got, "https://dawa24.test/orders/5") {
		t.Fatalf("decision reply %q", got)
	}
}

func TestExportsAreServedOnlyToLinkedUsers(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := pharmacist()
	asst.answer.Files = []chatbridge.AnswerFile{{Name: "طلباتي.xlsx", Token: "tok"}}
	asst.exports = map[string]*chatbridge.ExportFile{"tok": {UserID: userA, Filename: "طلباتي.xlsx"}, "other": {UserID: userB}}
	activeLink(repo, userA, waA, int64p(orgOne))
	s := newTestService(repo, grants, asst, "")

	r := handle(t, s, textHook(waA, "صدّر طلباتي"))
	doc := r.Messages[len(r.Messages)-1].Document
	if doc == nil || doc.URL != "https://dawa24.test"+ExportPath+"tok" {
		t.Fatalf("document %+v", r.Messages)
	}
	if f, _ := s.Export(context.Background(), "tok"); f == nil {
		t.Fatal("linked user's export refused")
	}
	if f, _ := s.Export(context.Background(), "other"); f != nil {
		t.Fatal("export served for a user with no link")
	}
}

func TestOrgCommandsUseTheSharedFlow(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := pharmacist()
	repo.memberships[userA] = []chatbridge.Membership{
		{OrgID: orgOne, Name: "صيدلية الشفاء", Type: "customer", Status: "approved"},
		{OrgID: 300, Name: "صيدلية النور", Type: "customer", Status: "approved"},
	}
	activeLink(repo, userA, waA, nil)
	s := newTestService(repo, grants, asst, "")

	if got := bodies(handle(t, s, textHook(waA, "سؤال"))); !strings.Contains(got, "أكثر من منشأة") {
		t.Fatalf("reply %q", got)
	}
	if got := bodies(handle(t, s, textHook(waA, "/org"))); !strings.Contains(got, "*اختر المنشأة") || !strings.Contains(got, "`/org 1`") {
		t.Fatalf("org list %q", got)
	}
	handle(t, s, textHook(waA, "/org 1"))
	if l := repo.byID(1); l.ActiveOrgID == nil || *l.ActiveOrgID != orgOne {
		t.Fatal("org not switched")
	}
	handle(t, s, textHook(waA, "سؤال"))
	if asst.calls != 1 {
		t.Fatal("question after switching was not answered")
	}
}
