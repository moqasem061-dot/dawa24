package ui_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Routes that are deliberately registered without a permission gate, with the
// reason. Everything else in the dashboard route files must sit inside a
// chi group whose Use() names a Require*Permission guard — otherwise the page
// is reachable by typing its URL, whatever the sidebar shows.
//
// Adding an entry here is a decision, not a formality: say why the route needs
// no permission.
var routesWithoutPermissionGate = map[string]string{
	// Redirects to a page that is itself gated.
	"GET /vendor/session":         "301 to /vendor/sessions, which is gated",
	"GET /vendor/notifications":   "301 to /notifications, which is gated",
	"GET /customer/session":       "301 to /customer/sessions, which is gated",
	"GET /customer/notifications": "301 to /notifications, which is gated",

	// Account actions, not company actions: a member locked out of every page
	// must still be able to secure their own credentials and pick the branch
	// they are buying for.
	"POST /vendor/password":          "changing your own password is an account action",
	"POST /vendor/set-branch":        "choosing your own active branch is an account action",
	"POST /vendor/branches/active":   "choosing your own active branch is an account action",
	"POST /customer/password":        "changing your own password is an account action",
	"POST /customer/set-branch":      "choosing your own buying branch is an account action",
	"POST /customer/branches/active": "choosing your own active branch is an account action",
	"GET /cart/count-badge":          "count badge on navbar for any authenticated caller",

	// --- the shared tiers, in handlers.go --------------------------------
	//
	// These came into view when this audit stopped trusting the *routes*.go
	// filename convention. Each one is a decision, and the wallet, invoice and
	// branch-manager routes that were NOT decisions are now gated instead of
	// listed here.

	// Assets. Public by definition.
	"GET /static/*":   "static assets are public",
	"GET /uploads/*":  "uploaded media is public; the handler checks the path, not the caller",
	"GET /robots.txt": "crawler policy is public",

	// The caller's own account. A member holding nothing must still be able to
	// change their password, revoke a session on a lost device, and read what
	// the platform is telling them.
	"GET /settings":                                     "the caller's own account page",
	"GET /settings/profile":                             "301 to /settings, the caller's own account",
	"GET /settings/addresses":                           "301 to /settings, the caller's own account",
	"GET /settings/security":                            "301 to /settings, the caller's own account",
	"GET /settings/preferences":                         "301 to /settings, the caller's own account",
	"GET /settings/payment-methods":                     "301 to /settings, the caller's own account",
	"GET /settings/employees":                           "301 to the team page, which is gated",
	"POST /settings/profile":                            "editing your own profile is an account action",
	"POST /settings/password":                           "changing your own password is an account action",
	"POST /settings/preferences":                        "your own language and display preferences",
	"GET /settings/telegram":                            "the caller's own Telegram link card",
	"POST /settings/telegram/link":                      "issuing a link code for your own account",
	"POST /settings/telegram/confirm":                   "confirming your own pending Telegram link",
	"POST /settings/telegram/unlink":                    "revoking your own Telegram link",
	"POST /settings/telegram/notify":                    "your own Telegram notification categories",
	"GET /settings/whatsapp":                            "the caller's own WhatsApp link card",
	"POST /settings/whatsapp/link":                      "issuing a link code for your own account",
	"POST /settings/whatsapp/confirm":                   "confirming your own pending WhatsApp link",
	"POST /settings/whatsapp/unlink":                    "revoking your own WhatsApp link",
	"POST /settings/whatsapp/notify":                    "your own WhatsApp notification categories",
	"POST /settings/addresses":                          "your own delivery addresses",
	"POST /settings/addresses/{id}/delete":              "your own delivery addresses",
	"POST /settings/delete-request":                     "asking for your own account to be deleted",
	"POST /settings/delete-request/cancel":              "cancelling your own deletion request",
	"POST /settings/organization/delete-request/cancel": "cancelling your own organization deletion request",
	"POST /settings/security/revoke":                    "revoking your own session on a lost device",
	"POST /settings/sessions/revoke":                    "revoking your own session on a lost device",
	"POST /settings/security/plan/{id}":                 "buying a session plan for your own account, not the company's",
	"POST /settings/payment-methods":                    "redirects to the gated wallet screen",
	"POST /settings/payment-methods/{id}/edit":          "redirects to the gated wallet screen",
	"POST /settings/payment-methods/{id}/default":       "redirects to the gated wallet screen",
	"POST /settings/payment-methods/{id}/delete":        "redirects to the gated wallet screen",
	"GET /notifications":                                "what the platform is telling this caller",
	"GET /notifications/dropdown":                       "what the platform is telling this caller",
	"GET /notifications/unread-badge":                   "what the platform is telling this caller",
	"POST /notifications/{id}/read":                     "marking your own notification read",
	"POST /notifications/read-all":                      "marking your own notifications read",
	"GET /report-issue":                                 "reporting a problem must not require a grant",
	"POST /report-issue":                                "reporting a problem must not require a grant",
	"GET /org/switch/{id}":                              "choosing which of your own memberships you are acting under",
	"GET /wallet":                                       "301 to /customer/wallet or /vendor/wallet, which carry the gate",
	"GET /components/capsule-assistant":                 "the assistant drawer shell; every tool inside it re-checks",

	// Onboarding and documents. A company still under review holds no
	// dashboard permission at all, and these are the two things it must be
	// able to do: read why it is waiting, and send its papers.
	"GET /onboarding/pending":               "a company under review holds no permissions yet",
	"GET /documents":                        "submitting papers is how a pending company becomes approved",
	"GET /documents/{id}/view":              "submitting papers is how a pending company becomes approved",
	"GET /documents/{id}/download":          "submitting papers is how a pending company becomes approved",
	"POST /documents/upload":                "submitting papers is how a pending company becomes approved",
	"POST /documents/delete":                "submitting papers is how a pending company becomes approved",
	"GET /customer/documents":               "pre-approval tier; the gated copy is in customer_routes.go",
	"GET /customer/documents/{id}/view":     "pre-approval tier; the gated copy is in customer_routes.go",
	"GET /customer/documents/{id}/download": "pre-approval tier; the gated copy is in customer_routes.go",
	"POST /customer/documents/upload":       "pre-approval tier; the gated copy is in customer_routes.go",
	"POST /customer/documents/delete":       "pre-approval tier; the gated copy is in customer_routes.go",
	"GET /vendor/documents":                 "pre-approval tier; the gated copy is in vendor_routes.go",
	"GET /vendor/documents/{id}/view":       "pre-approval tier; the gated copy is in vendor_routes.go",
	"GET /vendor/documents/{id}/download":   "pre-approval tier; the gated copy is in vendor_routes.go",
	"POST /vendor/documents/upload":         "pre-approval tier; the gated copy is in vendor_routes.go",
	"POST /vendor/documents/delete":         "pre-approval tier; the gated copy is in vendor_routes.go",

	// Correspondence. Neither dashboard declares a permission for messaging or
	// requests and neither sidebar links to them, so there is no key to gate
	// on; the handlers scope every read to the caller's own organization.
	"GET /messages":            "no messaging permission is declared in either scope",
	"GET /messages/{id}":       "no messaging permission is declared in either scope",
	"POST /messages/{id}/send": "no messaging permission is declared in either scope",
	"GET /requests":            "no requests permission is declared in either scope",
	"POST /requests":           "no requests permission is declared in either scope",

	// Progress of an import the caller started; the handler resolves the run
	// against their own organization.
	"GET /imports/{id}/progress": "progress of the caller's own import run",
	"GET /imports/{id}/stream":   "progress of the caller's own import run",

	// The staff landing pages. RequireStaff is the gate; a staff member with
	// no further grants still needs somewhere to land.
	"GET /admin/dashboard": "every staff member lands here; RequireStaff is the gate",
	"GET /admin/gallery":   "the component gallery is staff-only reference material",
}

var (
	guardRe = regexp.MustCompile(`\bRequire[A-Za-z0-9_]*\(`)
	// Only paths, so a stray http.Get("https://...") in a handler is not read
	// as a route registration.
	routeRe = regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z0-9_]*\.(Get|Post|Put|Delete|Patch|Head)\(\s*"(/[^"]*)"`)
)

// routeRegistrarFiles are the files whose route registrations must all sit
// behind a permission gate.
//
// It used to be the glob internal/ui/*routes*.go, and that naming convention is
// exactly what an audit must not depend on: RegisterSmartOrderRoutes lives in
// smart_order_actions.go, so every Smart Ordering route — upload a list,
// re-match its lines, switch supplier, finalise the order — sat outside this
// check, and was in fact registered with no permission gate at all, while
// pharmacy.smart_order.view and .run existed, appeared in the role editor and
// controlled nothing.
//
// So the file set is discovered instead: any non-test file in internal/ui that
// registers a path. A new registrar is covered the day it is written, whatever
// it happens to be called.
func routeRegistrarFiles(t *testing.T) []string {
	t.Helper()
	all, err := filepath.Glob(filepath.Join("..", "..", "internal", "ui", "*.go"))
	if err != nil {
		t.Fatalf("glob internal/ui: %v", err)
	}
	var out []string
	for _, f := range all {
		base := filepath.Base(f)
		if strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, "_templ.go") {
			continue
		}
		// public_routes.go is the signed-out surface by definition; its
		// handlers carry their own checks and audience_separation_test.go
		// covers them.
		if base == "public_routes.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if routeRe.Match(src) {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

// TestEveryDashboardRouteSitsBehindAPermission walks the admin, vendor and
// pharmacy route tables and fails on any registration that is not inside a
// permission-guarded group.
//
// This is the direct-URL guarantee. A sidebar that hides a link proves nothing;
// only a middleware on the route does. The check is textual because that is
// what it is protecting: the shape of the route file, at the moment somebody
// adds a line to it.
func TestEveryDashboardRouteSitsBehindAPermission(t *testing.T) {
	files := routeRegistrarFiles(t)
	if len(files) == 0 {
		t.Fatal("no route files found — has internal/ui moved?")
	}

	var ungated []string
	checked := 0
	for _, f := range files {
		base := filepath.Base(f)
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var stack []map[string]bool
		for _, line := range strings.Split(string(src), "\n") {
			if guardRe.MatchString(line) && len(stack) > 0 {
				stack[len(stack)-1]["guard"] = true
			}
			if m := routeRe.FindStringSubmatch(line); m != nil && !strings.Contains(line, ".Use(") {
				checked++
				guarded := false
				for _, frame := range stack {
					if frame["guard"] {
						guarded = true
						break
					}
				}
				key := strings.ToUpper(m[1]) + " " + m[2]
				if !guarded {
					if _, allowed := routesWithoutPermissionGate[key]; !allowed {
						ungated = append(ungated, key+"   ("+base+")")
					}
				}
			}
			opens := strings.Count(line, "{") - strings.Count(line, "}")
			for i := 0; i < opens; i++ {
				stack = append(stack, map[string]bool{})
			}
			for i := 0; i > opens && len(stack) > 0; i-- {
				stack = stack[:len(stack)-1]
			}
		}
	}

	if checked == 0 {
		t.Fatal("parsed no routes at all — the route-file shape changed and this gate is not checking anything")
	}
	if len(ungated) > 0 {
		sort.Strings(ungated)
		t.Fatalf("%d dashboard route(s) are reachable without a permission check.\n"+
			"Put each inside a chi group whose Use() names a Require*Permission guard,\n"+
			"or add it to routesWithoutPermissionGate with the reason:\n  %s",
			len(ungated), strings.Join(ungated, "\n  "))
	}
	t.Logf("checked %d dashboard routes; all behind a permission gate", checked)
}
