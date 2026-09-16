package telegram

import (
	"context"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
)

const proposalID = "3f2a9c1e-7b44-4d2a-9a51-0c6d8e2f1b7a"

func pressed(tg int64, data string) Update {
	nextUpdate++
	return Update{UpdateID: nextUpdate, CallbackQuery: &CallbackQuery{
		ID:      "cbq-1",
		From:    User{ID: tg, FirstName: "Ali"},
		Message: &Message{MessageID: 9, Chat: Chat{ID: tg, Type: "private"}},
		Data:    data,
	}}
}

func actingPharmacist(repo *fakeRepo) (fakeGrants, *fakeAssistant) {
	activeLink(repo, userA, tgA, int64p(orgOne))
	grants := fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "customer", "approved", "pharmacy.assistant.use", "pharmacy.assistant.act")}
	return grants, &fakeAssistant{gate: "pharmacy.assistant.use", decideResult: chatbridge.ActionReply{Message: "تم التنفيذ.", URL: "/orders/5"}}
}

func TestConfirmButtonDecidesForTheLinkedUser(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := actingPharmacist(repo)
	s := newTestService(repo, grants, asst)

	r := mustHandle(t, s, pressed(tgA, "act:c:"+proposalID))
	if len(asst.decisions) != 1 || asst.decisions[0] != "confirm:"+proposalID {
		t.Fatalf("decisions = %v", asst.decisions)
	}
	if asst.decideActor.UserID != userA || asst.decideActor.OrgID != orgOne {
		t.Fatalf("decided as %+v", asst.decideActor)
	}
	if tenant, ok := tenantOf(asst.decideCtx); !ok || tenant != orgOne {
		t.Fatalf("decision ran without the caller's tenant: %v %v", tenant, ok)
	}
	if r.AnswerCallback == nil || r.AnswerCallback.ID != "cbq-1" {
		t.Fatal("the button's spinner must be answered")
	}
	if !strings.Contains(texts(r), "تم التنفيذ.") || !strings.Contains(texts(r), "https://dawa24.test/orders/5") {
		t.Fatalf("reply = %s", texts(r))
	}

	// Telegram redelivering the same update must not decide twice.
	u := pressed(tgA, "act:x:"+proposalID)
	mustHandle(t, s, u)
	mustHandle(t, s, u)
	if len(asst.decisions) != 2 || asst.decisions[1] != "cancel:"+proposalID {
		t.Fatalf("decisions after redelivery = %v", asst.decisions)
	}
}

func TestButtonsFromTheWrongPlaceDecideNothing(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := actingPharmacist(repo)
	s := newTestService(repo, grants, asst)

	group := pressed(tgA, "act:c:"+proposalID)
	group.CallbackQuery.Message.Chat = Chat{ID: -1001, Type: "supergroup"}

	forwarded := pressed(tgB, "act:c:"+proposalID) // someone else's chat
	forwarded.CallbackQuery.Message.Chat = Chat{ID: tgA, Type: "private"}

	unlinked := pressed(tgB, "act:c:"+proposalID)

	for name, u := range map[string]Update{
		"group":          group,
		"another's chat": forwarded,
		"unlinked":       unlinked,
		"forged data":    pressed(tgA, "act:c:1 OR 1=1"),
		"other prefix":   pressed(tgA, "org:c:"+proposalID),
		"no message":     {UpdateID: 99999, CallbackQuery: &CallbackQuery{ID: "x", From: User{ID: tgA}, Data: "act:c:" + proposalID}},
	} {
		if _, err := s.HandleUpdate(context.Background(), u); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(asst.decisions) != 0 {
			t.Fatalf("%s reached a decision: %v", name, asst.decisions)
		}
	}
}

func TestButtonsRespectOrganisationApproval(t *testing.T) {
	repo := newFakeRepo()
	activeLink(repo, userA, tgA, int64p(orgOne))
	grants := fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "customer", "suspended", "pharmacy.assistant.use", "pharmacy.assistant.act")}
	asst := &fakeAssistant{gate: "pharmacy.assistant.use"}
	s := newTestService(repo, grants, asst)

	r := mustHandle(t, s, pressed(tgA, "act:c:"+proposalID))
	if len(asst.decisions) != 0 || !strings.Contains(texts(r), "موقوفة") {
		t.Fatalf("a suspended organisation decided: %v / %s", asst.decisions, texts(r))
	}
}

func TestProposalsAndFilesFollowTheAnswer(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := actingPharmacist(repo)
	asst.answer = chatbridge.Answer{
		Markdown: "جهّزت الطلب.",
		Proposals: []chatbridge.AnswerProposal{{
			ID: proposalID, Title: "تأكيد طلب شراء", Summary: "<b>3 أصناف</b>",
			Details: [][2]string{{"الإجمالي", "1,250 ج.م"}}, Warnings: []string{"سيُرسل فوراً"},
		}},
		Files: []chatbridge.AnswerFile{{Name: "طلباتي.xlsx", Token: "tok_abc"}},
	}
	s := newTestService(repo, grants, asst)

	r := mustHandle(t, s, privateText(tgA, "اطلب السلة"))
	if len(r.Messages) != 3 {
		t.Fatalf("want answer, proposal and document; got %d messages", len(r.Messages))
	}
	card := r.Messages[1]
	if card.ReplyMarkup == nil || len(card.ReplyMarkup.Rows[0]) != 2 {
		t.Fatal("proposal message has no confirm/cancel buttons")
	}
	if card.ReplyMarkup.Rows[0][0].CallbackData != "act:c:"+proposalID || card.ReplyMarkup.Rows[0][1].CallbackData != "act:x:"+proposalID {
		t.Fatalf("callback data = %+v", card.ReplyMarkup.Rows[0])
	}
	if strings.Contains(card.Text, "<b>3 أصناف</b>") {
		t.Fatal("preview text must be escaped, not rendered as markup")
	}
	doc := r.Messages[2].Document
	if doc == nil || doc.URL != "https://dawa24.test/api/v1/integrations/telegram/exports/tok_abc" {
		t.Fatalf("document = %+v", doc)
	}
}

// An oversized card falls back to plain text, which still travels with
// parse_mode HTML: a raw < or & from a product name would make Telegram
// refuse the message and the buttons would never arrive.
func TestOversizedProposalStaysValidHTML(t *testing.T) {
	details := make([][2]string, 12)
	for i := range details {
		details[i] = [2]string{"صنف <x> & co", strings.Repeat("ب", 400)}
	}
	msg := proposalMessage(tgA, chatbridge.AnswerProposal{ID: proposalID, Title: "a < b", Details: details})
	if units(msg.Text) > MaxMessageUnits {
		t.Fatalf("text is %d units", units(msg.Text))
	}
	if strings.ContainsAny(strings.NewReplacer("&amp;", "", "&lt;", "", "&gt;", "", "&#34;", "", "&#39;", "").Replace(msg.Text), "<>&") {
		t.Fatalf("unescaped markup in fallback text: %q", msg.Text[:80])
	}
	if msg.ReplyMarkup == nil {
		t.Fatal("fallback lost the buttons")
	}
}

func TestExportsAreServedOnlyToLinkedUsers(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := actingPharmacist(repo)
	asst.exports = map[string]*chatbridge.ExportFile{
		"mine":   {UserID: userA, Filename: "a.csv"},
		"orphan": {UserID: userB, Filename: "b.csv"},
	}
	s := newTestService(repo, grants, asst)

	if f, err := s.Export(context.Background(), "mine"); err != nil || f == nil {
		t.Fatalf("linked user's export refused: %v", err)
	}
	if f, _ := s.Export(context.Background(), "orphan"); f != nil {
		t.Fatal("an export of a user with no Telegram link was served")
	}
	if f, _ := s.Export(context.Background(), "missing"); f != nil {
		t.Fatal("an unknown token was served")
	}
}
