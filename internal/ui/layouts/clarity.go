package layouts

import (
	"context"
	"io"
	"regexp"

	"github.com/a-h/templ"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
)

// ClarityProjectID is the Microsoft Clarity project the pages report to, set
// once at start-up from CLARITY_PROJECT_ID by cmd/server. Empty renders
// nothing.
//
// A package variable for the same reason as Asset: the value is process-wide
// and every layout that writes a <head> needs it without threading it through
// each page's arguments.
var ClarityProjectID string

var clarityIDShape = regexp.MustCompile(`^[a-z0-9]{6,32}$`)

// ClarityScript renders Microsoft's tag in the form this site's policy allows.
//
// Microsoft's snippet builds the tag with createElement + script.src, which
// is a Trusted Types sink and has no nonce. Here the queue stub is an inline
// script carrying the request's nonce, and the tag is a parsed <script src>,
// which is neither. The tag then loads the recorder from scripts.clarity.ms;
// that script.src passes the default policy's Clarity rule in security.js,
// and CSP admits the host because SecurityOptions.Clarity is set from the
// same configuration.
//
// It must follow security.js in the document: that file installs the policy.
//
// Two custom tags describe the session without identifying anyone: which
// dashboard it was on, and whether someone was signed in. Filter on them in
// the Clarity dashboard.
func ClarityScript() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		id := ClarityProjectID
		if !clarityIDShape.MatchString(id) {
			return nil
		}
		nonce := Nonce(ctx)
		attr := ""
		if nonce != "" {
			attr = ` nonce="` + nonce + `"`
		}
		surface, signedIn := claritySurface(ctx)
		_, err := io.WriteString(w,
			`<script`+attr+`>window.clarity=window.clarity||function(){(window.clarity.q=window.clarity.q||[]).push(arguments)};`+
				`window.clarity("set","surface","`+surface+`");window.clarity("set","signed_in","`+signedIn+`");</script>`+
				`<script async`+attr+` src="https://www.clarity.ms/tag/`+id+`"></script>`)
		return err
	})
}

// claritySurface names the dashboard the page belongs to. Both values come
// from fixed sets, so they are written into the script without escaping.
func claritySurface(ctx context.Context) (surface, signedIn string) {
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		return "public", "no"
	}
	if actor.IsStaff && actor.OrganizationID == 0 {
		return "admin", "yes"
	}
	if s, ok := rbac.TenantScopeFor(actor.OrgType); ok {
		return string(s), "yes"
	}
	return "account", "yes"
}
