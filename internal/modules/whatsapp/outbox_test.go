package whatsapp

import (
	"context"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
)

func TestClaimChoosesTextTemplateOrDropByWindow(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := pharmacist()
	repo.leased = []LeasedDelivery{
		{ID: 1, WAID: waA, Text: "🔔 *طلب جديد*\nرقم 55", WindowOpen: true},
		{ID: 2, WAID: waB, Text: "🔔 *طلب جديد*\nرقم    56\n\nعرض", WindowOpen: false},
	}

	withTemplate := newTestService(repo, grants, asst, "dawa24_notification")
	out, err := withTemplate.ClaimOutbox(context.Background(), 10)
	if err != nil || len(out) != 2 {
		t.Fatalf("claim: %v %+v", err, out)
	}
	if out[0].Payload.Type != "text" || out[0].Payload.Text.Body != repo.leased[0].Text {
		t.Fatalf("open window not sent as text: %+v", out[0].Payload)
	}
	tpl := out[1].Payload.Template
	if out[1].Payload.Type != "template" || tpl.Name != "dawa24_notification" || tpl.Language.Code != "ar" {
		t.Fatalf("closed window not sent as template: %+v", out[1].Payload)
	}
	param := tpl.Components[0].Parameters[0].Text
	if strings.ContainsAny(param, "\n\t*") || strings.Contains(param, "  ") {
		t.Fatalf("template parameter not flattened: %q", param)
	}

	withoutTemplate := newTestService(repo, grants, asst, "")
	out, _ = withoutTemplate.ClaimOutbox(context.Background(), 10)
	if len(out) != 1 || repo.dropped[2] != "window_closed" {
		t.Fatalf("closed window without template: out=%+v dropped=%v", out, repo.dropped)
	}
}

func TestNotificationsUseTheSharedEligibility(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := pharmacist()
	repo.candidates = []chatbridge.Candidate{
		{LogID: 1, LinkID: 1, UserID: userA, OrganizationID: orgOne, Title: "طلب", RequiredPermission: "pharmacy.orders.view"},
		{LogID: 2, LinkID: 1, UserID: userA, OrganizationID: orgOne, Title: "مساعد", RequiredPermission: "pharmacy.assistant.use"},
	}
	s := newTestService(repo, grants, asst, "")
	if _, err := s.ClaimOutbox(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(repo.decisions) != 2 || repo.decisions[0].DropReason != "permission_revoked" || repo.decisions[1].DropReason != "" {
		t.Fatalf("decisions %+v", repo.decisions)
	}
	if !strings.HasPrefix(repo.decisions[1].Text, "🔔 *مساعد*") {
		t.Fatalf("text %q", repo.decisions[1].Text)
	}
}

func TestDeliveryReportsAreClassified(t *testing.T) {
	repo := newFakeRepo()
	grants, asst := pharmacist()
	s := newTestService(repo, grants, asst, "")
	err := s.ReportDeliveries(context.Background(), []DeliveryResult{
		{ID: 1, OK: true},
		{ID: 2, Error: `(#131026) Message Undeliverable`},
		{ID: 3, Error: `{"error":{"code":131047,"message":"Re-engagement message"}}`},
		{ID: 4, Error: `(#130429) Rate limit hit`},
		{ID: 5, Error: `(#132001) Template name does not exist in the translation`},
		{ID: 6, Error: `socket hang up`},
	})
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case len(repo.delivered) != 1 || repo.delivered[0] != 1:
		t.Errorf("delivered %v", repo.delivered)
	case len(repo.blocked) != 1 || repo.blocked[0] != 2:
		t.Errorf("blocked %v", repo.blocked)
	case len(repo.windowShut) != 1 || repo.windowShut[0] != 3:
		t.Errorf("window %v", repo.windowShut)
	case repo.retried[4] != rateLimitBackoff:
		t.Errorf("rate limit retry %v", repo.retried[4])
	case repo.failed[5] != "unretryable":
		t.Errorf("template error %v", repo.failed)
	case repo.retried[6] != 0 || repo.failed[6] != "":
		t.Errorf("generic error %v %v", repo.retried, repo.failed)
	}
	if _, ok := repo.retried[6]; !ok {
		t.Error("generic error not retried")
	}
}

func TestRenderMarkdownForWhatsApp(t *testing.T) {
	got := strings.Join(RenderMarkdown("## ملخص\n- **3** طلبات [فتح](https://dawa24.test/orders)\n- [x](javascript:alert(1))\n| a | b |\n|---|---|\n| 1 | 2 |\n`**raw**`"), "\n")
	for _, want := range []string{"*ملخص*", "• *3* طلبات فتح (https://dawa24.test/orders)", "• x", "1  ·  2", "`**raw**`"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "javascript") {
		t.Errorf("unsafe link kept: %q", got)
	}

	long := "```\n" + strings.Repeat("سطر كود طويل نسبياً\n", 400) + "```"
	chunks := RenderMarkdown(long)
	if len(chunks) < 2 {
		t.Fatalf("not split: %d", len(chunks))
	}
	for i, c := range chunks {
		if units(c) > MaxMessageUnits || strings.Count(c, fence)%2 != 0 {
			t.Fatalf("chunk %d: %d units, %d fences", i, units(c), strings.Count(c, fence))
		}
	}
}
