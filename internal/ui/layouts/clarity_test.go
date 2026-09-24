package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
)

func renderBaseWithClarity(t *testing.T, id string, actor *authctx.Actor) string {
	t.Helper()
	prev := ClarityProjectID
	ClarityProjectID = id
	t.Cleanup(func() { ClarityProjectID = prev })

	ctx := httpx.WithNonce(context.Background(), "testnonce")
	if actor != nil {
		ctx = authctx.WithActor(ctx, *actor)
	}
	var buf bytes.Buffer
	if err := Base("Test", "ar", "rtl").Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestClarityTagInEveryPageHead(t *testing.T) {
	html := renderBaseWithClarity(t, "ymd15b26tc", nil)

	head := html[:strings.Index(html, "</head>")]
	tag := `<script async nonce="testnonce" src="https://www.clarity.ms/tag/ymd15b26tc"></script>`
	if !strings.Contains(head, tag) {
		t.Fatalf("the Clarity tag must be a nonced, parsed <script src> in <head>:\n%s", head)
	}
	stub := `<script nonce="testnonce">window.clarity=window.clarity||function(){(window.clarity.q=window.clarity.q||[]).push(arguments)};`
	if !strings.Contains(head, stub) {
		t.Fatal("the queue stub must run first, under the nonce")
	}
	// security.js installs the Trusted Types policy the recorder's load needs.
	if strings.Index(head, "/static/js/security.js") > strings.Index(head, "www.clarity.ms") {
		t.Fatal("Clarity must come after security.js")
	}
	if !strings.Contains(head, `window.clarity("set","surface","public")`) {
		t.Fatal("a signed-out page is tagged public")
	}
	// Every stylesheet is exempt from masking, or a replay can lose its href.
	if n, u := strings.Count(head, `rel="stylesheet"`), strings.Count(head, `rel="stylesheet" data-clarity-unmask="true"`); n == 0 || n != u {
		t.Fatalf("%d of %d stylesheets carry data-clarity-unmask", u, n)
	}
	if strings.Count(html, "www.clarity.ms/tag/") != 1 {
		t.Fatal("the tag must load exactly once")
	}
}

func TestClaritySurfaceTags(t *testing.T) {
	cases := []struct {
		actor authctx.Actor
		want  string
	}{
		{authctx.Actor{UserID: 1, IsStaff: true}, "admin"},
		{authctx.Actor{UserID: 2, OrganizationID: 5, OrgType: "supplier"}, "vendor"},
		{authctx.Actor{UserID: 3, OrganizationID: 6, OrgType: "chain_pharmacy"}, "pharmacy"},
		{authctx.Actor{UserID: 4}, "account"},
	}
	for _, c := range cases {
		html := renderBaseWithClarity(t, "ymd15b26tc", &c.actor)
		if !strings.Contains(html, `window.clarity("set","surface","`+c.want+`");window.clarity("set","signed_in","yes")`) {
			t.Errorf("actor %+v: want surface %q", c.actor, c.want)
		}
	}
}

func TestClarityOffRendersNothing(t *testing.T) {
	for _, id := range []string{"", `x"><script>alert(1)</script>`} {
		html := renderBaseWithClarity(t, id, nil)
		if strings.Contains(html, "clarity.ms") || strings.Contains(html, "window.clarity") {
			t.Errorf("id %q must render no Clarity script", id)
		}
	}
}
