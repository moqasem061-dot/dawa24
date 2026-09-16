package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
)

func candidate(logID int64, perm, title string) chatbridge.Candidate {
	return chatbridge.Candidate{
		LogID: logID, LinkID: 1, LinkActiveOrgID: orgOne, UserID: userA,
		OrganizationID: orgOne, OrganizationName: "صيدلية النور",
		Title: title, Body: "تفاصيل", RequiredPermission: perm,
	}
}

func decisionFor(t *testing.T, repo *fakeRepo, logID int64) chatbridge.Decision {
	t.Helper()
	for _, d := range repo.decisions {
		if d.LogID == logID {
			return d
		}
	}
	t.Fatalf("no decision recorded for log %d", logID)
	return chatbridge.Decision{}
}

func TestNotificationEligibilityIsReResolvedAtDelivery(t *testing.T) {
	repo := newFakeRepo()
	grants := fakeGrants{
		{userA, orgOne}: memberGrant(userA, orgOne, "customer", "approved", "pharmacy.order.view"),
	}
	s := newTestService(repo, grants, &fakeAssistant{})

	offer := candidate(4, "vendor.offer.view", "عرض جديد")
	offer.UserID = userB
	offer.OrganizationID = orgTwo
	grants[[2]int64{userB, orgTwo}] = memberGrant(userB, orgTwo, "vendor", "approved", "vendor.offer.view")
	repo.offersOff[userB] = true

	muted := candidate(5, "pharmacy.order.view", "طلب")
	muted.MutedCategories = []string{"orders"}

	noOrg := candidate(6, "pharmacy.order.view", "تحديث حالة الطلب")
	noOrg.OrganizationID = 0

	repo.candidates = []chatbridge.Candidate{
		candidate(1, "pharmacy.order.view", "تحديث حالة الطلب"),
		candidate(2, "pharmacy.wallet.view", "تم شحن المحفظة"), // role lost this permission
		candidate(3, "", "رسالة عامة"),
		offer, muted, noOrg,
	}
	if _, err := s.ClaimOutbox(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if d := decisionFor(t, repo, 1); d.DropReason != "" || d.Category != chatbridge.CategoryOrders || d.Text == "" {
		t.Fatalf("permitted order update was not queued: %+v", d)
	}
	if d := decisionFor(t, repo, 2); d.DropReason != "permission_revoked" || d.Text != "" {
		t.Fatalf("revoked permission must drop without text: %+v", d)
	}
	if d := decisionFor(t, repo, 3); d.DropReason != "" {
		t.Fatalf("an unrestricted notification should be queued: %+v", d)
	}
	if d := decisionFor(t, repo, 4); d.DropReason != "offers_preference_off" {
		t.Fatalf("offers preference ignored: %+v", d)
	}
	if d := decisionFor(t, repo, 5); d.DropReason != "muted" {
		t.Fatalf("muted category delivered: %+v", d)
	}
	if d := decisionFor(t, repo, 6); d.DropReason != "" {
		t.Fatalf("no-org notification should be judged against the active org: %+v", d)
	}
}

func TestNotificationsStopWhenMembershipOrAccountEnds(t *testing.T) {
	repo := newFakeRepo()
	inactive := memberGrant(userA, orgTwo, "vendor", "approved", "vendor.order.view")
	inactive.Active = false
	grants := fakeGrants{{userA, orgTwo}: inactive}
	s := newTestService(repo, grants, &fakeAssistant{})

	gone := candidate(1, "pharmacy.order.view", "طلب") // no grant for orgOne: membership ended
	off := candidate(2, "vendor.order.view", "طلب")
	off.OrganizationID = orgTwo
	repo.candidates = []chatbridge.Candidate{gone, off}
	if _, err := s.ClaimOutbox(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if d := decisionFor(t, repo, 1); d.DropReason != "membership_ended" {
		t.Fatalf("got %+v", d)
	}
	if d := decisionFor(t, repo, 2); d.DropReason != "account_inactive" {
		t.Fatalf("got %+v", d)
	}
}

func TestOwnerReceivesWithoutExplicitPermission(t *testing.T) {
	repo := newFakeRepo()
	owner := memberGrant(userA, orgOne, "customer", "approved")
	owner.IsOrgOwner = true
	s := newTestService(repo, fakeGrants{{userA, orgOne}: owner}, &fakeAssistant{})
	repo.candidates = []chatbridge.Candidate{candidate(1, "pharmacy.wallet.view", "محفظة")}
	if _, err := s.ClaimOutbox(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if d := decisionFor(t, repo, 1); d.DropReason != "" {
		t.Fatalf("owner refused: %+v", d)
	}
}

func TestNotificationTextIsEscaped(t *testing.T) {
	s := newTestService(newFakeRepo(), fakeGrants{}, &fakeAssistant{})
	c := candidate(1, "", `<a href="javascript:x">click</a> & <b>`)
	c.OrganizationID = orgTwo
	c.OrganizationName = "<i>org</i>"
	text := s.notificationText(c)
	if strings.Contains(text, "<a href=\"javascript") || strings.Contains(text, "<i>org") {
		t.Fatalf("producer text reached Telegram as markup: %s", text)
	}
	if !strings.Contains(text, "&lt;a href=") || !strings.Contains(text, "&lt;i&gt;org") {
		t.Fatalf("expected escaped text, got %s", text)
	}
}

func TestDeliveryReportsAreClassified(t *testing.T) {
	repo := newFakeRepo()
	s := newTestService(repo, fakeGrants{}, &fakeAssistant{})
	err := s.ReportDeliveries(context.Background(), []DeliveryResult{
		{ID: 1, OK: true},
		{ID: 2, Error: "Forbidden: bot was blocked by the user"},
		{ID: 3, Error: "Too Many Requests: retry after 35"},
		{ID: 4, Error: "Bad Request: can't parse entities"},
		{ID: 5, Error: "socket hang up"},
		{ID: 0, OK: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.delivered) != 1 || repo.delivered[0] != 1 {
		t.Fatalf("delivered = %v", repo.delivered)
	}
	if len(repo.blocked) != 1 || repo.blocked[0] != 2 {
		t.Fatalf("blocked = %v", repo.blocked)
	}
	if repo.retried[3] != 36*time.Second {
		t.Fatalf("flood control retry = %v", repo.retried[3])
	}
	if repo.failed[4] == "" {
		t.Fatal("an unparseable message must not be retried")
	}
	if _, ok := repo.retried[5]; !ok {
		t.Fatal("a transient error must be retried")
	}
}

func TestCategoryFor(t *testing.T) {
	cases := map[[2]string]chatbridge.Category{
		{"pharmacy.order.view", ""}:               chatbridge.CategoryOrders,
		{"vendor.purchase_request.view", ""}:      chatbridge.CategoryOrders,
		{"vendor.wallet.view", ""}:                chatbridge.CategoryPayments,
		{"billing.payment.view", ""}:              chatbridge.CategoryPayments,
		{"vendor.delivery.view", ""}:              chatbridge.CategoryDelivery,
		{"vendor.offer_package.view", ""}:         chatbridge.CategoryOffers,
		{"vendor.ad.view", ""}:                    chatbridge.CategoryOffers,
		{"vendor.organization.view", ""}:          chatbridge.CategoryAccount,
		{"admin.organizations.manage", ""}:        chatbridge.CategoryAccount,
		{"workflow.issue.view", ""}:               chatbridge.CategoryGeneral,
		{"", "تحديث حالة الطلب #2298"}:            chatbridge.CategoryOrders,
		{"", "تم تسليم الشحنة"}:                   chatbridge.CategoryDelivery,
		{"", "بلاغ دعم جديد"}:                     chatbridge.CategoryGeneral,
		{"vendor.offer.view", "تحديث حالة الطلب"}: chatbridge.CategoryOffers,
	}
	for in, want := range cases {
		if got := chatbridge.CategoryFor(in[0], in[1]); got != want {
			t.Errorf("chatbridge.CategoryFor(%q, %q) = %s, want %s", in[0], in[1], got, want)
		}
	}
}
