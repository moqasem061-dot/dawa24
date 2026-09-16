package test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestMarkupHonoursCSP keeps templates and scripts inside what the
// Content-Security-Policy allows (see internal/platform/httpx/security.go).
//
// The policy has no 'unsafe-inline', 'unsafe-eval' or style attributes and
// enforces Trusted Types. Anything outside that does not fail loudly: the
// browser drops the handler, the style or the script, logs a console line, and
// the page quietly stops working. So the rules are checked here instead:
//
//   - no inline event handlers (onclick=...): use data-on-* (actions.js) or
//     Alpine directives;
//   - no style="" attributes and no <style> without a nonce: use classes, or
//     set geometry through the CSSOM (element.style.x = ...);
//   - no executable <script> without a nonce, no templ `script` components and
//     no templ.JS* helpers (they emit inline handlers);
//   - no javascript: URLs;
//   - in JavaScript: no eval, new Function or string timers, and every HTML
//     sink assignment goes through window.dawaHTML (security.js).
func TestMarkupHonoursCSP(t *testing.T) {
	const root = ".."
	var findings []string
	add := func(path string, line int, format string, args ...any) {
		rel, _ := filepath.Rel(root, path)
		findings = append(findings, fmt.Sprintf("%s:%d: %s", filepath.ToSlash(rel), line, fmt.Sprintf(format, args...)))
	}

	walk := func(dir string, fn func(path string, src string)) {
		err := filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if info.Name() == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fn(path, string(b))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	walk("internal", func(path, src string) {
		switch {
		case strings.HasSuffix(path, ".templ"):
			checkTemplate(path, src, add)
		case strings.HasSuffix(path, "_templ.go"), strings.HasSuffix(path, "_test.go"):
		case strings.HasSuffix(path, ".go"):
			// Only the UI builds HTML in Go; elsewhere these strings are data
			// (the upload scanner's list of forbidden SVG tokens, for one).
			if strings.Contains(filepath.ToSlash(path), "internal/ui/") {
				checkGoMarkup(path, src, add)
			}
		case strings.HasSuffix(path, ".js"):
			checkScript(path, src, 1, add)
		case strings.HasSuffix(path, ".html"):
			checkTemplate(path, src, add)
		}
	})

	if len(findings) > 0 {
		sort.Strings(findings)
		t.Errorf("%d markup constructs the Content-Security-Policy blocks:\n  %s",
			len(findings), strings.Join(findings, "\n  "))
	}
}

var (
	reInlineHandler = regexp.MustCompile(`(?i)(?:^|[\s"'])on[a-z]{3,}\s*=\s*["'{]`)
	reStyleAttr     = regexp.MustCompile(`(?i)(?:^|[\s"'])style\s*=\s*["'{]`)
	reStyleOpen     = regexp.MustCompile(`(?i)<style\b[^>]*>`)
	reScriptBlock   = regexp.MustCompile(`(?is)(<script\b[^>]*>)(.*?)</script>`)
	reTemplScript   = regexp.MustCompile(`(?m)^script\s+\w+\s*\(`)
	reTemplJS       = regexp.MustCompile(`templ\.(JSFuncCall|JSUnsafeFuncCall|ComponentScript|JSExpression|SafeScript)\b`)
	reJSURL         = regexp.MustCompile(`(?i)(href|src|action|formaction)\s*=\s*["']\s*javascript:`)
	reNonce         = regexp.MustCompile(`\bnonce\s*=`)
	reScriptType    = regexp.MustCompile(`(?i)\btype\s*=\s*["']([^"']*)["']`)
	reGoComment     = regexp.MustCompile(`(?m)^\s*//.*$`)
)

// Script types the browser does not execute.
var dataScriptTypes = map[string]bool{
	"application/json": true, "application/ld+json": true, "text/template": true,
	"text/html": true, "importmap": false,
}

func lineAt(src string, offset int) int { return strings.Count(src[:offset], "\n") + 1 }

// blank replaces a range with spaces byte for byte, keeping newlines, so
// offsets and line numbers into the original still hold.
func blank(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c != '\n' {
			b[i] = ' '
		}
	}
	return string(b)
}

func checkTemplate(path, src string, add func(string, int, string, ...any)) {
	// templ Go comments are not markup (they mention <script> and friends).
	src = reGoComment.ReplaceAllStringFunc(src, blank)
	// Script bodies are JavaScript, checked separately; strip them so their
	// code does not read as attributes.
	markup := src
	for _, m := range reScriptBlock.FindAllStringSubmatchIndex(src, -1) {
		open := src[m[2]:m[3]]
		body := src[m[4]:m[5]]
		if !strings.Contains(strings.ToLower(open), " src") {
			typ := ""
			if tm := reScriptType.FindStringSubmatch(open); tm != nil {
				typ = strings.ToLower(tm[1])
			}
			if !dataScriptTypes[typ] {
				if !reNonce.MatchString(open) {
					add(path, lineAt(src, m[0]), "inline <script> without nonce={ layouts.Nonce(ctx) }")
				}
				checkScript(path, body, lineAt(src, m[4]), add)
			}
		}
		markup = markup[:m[4]] + blank(body) + markup[m[5]:]
	}

	for _, loc := range reInlineHandler.FindAllStringIndex(markup, -1) {
		add(path, lineAt(markup, loc[0]), "inline event handler %q: use data-on-* or an Alpine directive",
			strings.TrimSpace(markup[loc[0]:loc[1]]))
	}
	for _, loc := range reStyleAttr.FindAllStringIndex(markup, -1) {
		add(path, lineAt(markup, loc[0]), "style attribute: use a class or set it through the CSSOM")
	}
	for _, loc := range reStyleOpen.FindAllStringIndex(markup, -1) {
		if !reNonce.MatchString(markup[loc[0]:loc[1]]) {
			add(path, lineAt(markup, loc[0]), "<style> without nonce={ layouts.Nonce(ctx) }")
		}
	}
	for _, loc := range reTemplScript.FindAllStringIndex(markup, -1) {
		add(path, lineAt(markup, loc[0]), "templ script component: it renders an inline event handler")
	}
	for _, loc := range reTemplJS.FindAllStringIndex(markup, -1) {
		add(path, lineAt(markup, loc[0]), "%s renders an inline event handler", markup[loc[0]:loc[1]])
	}
	for _, loc := range reJSURL.FindAllStringIndex(markup, -1) {
		add(path, lineAt(markup, loc[0]), "javascript: URL")
	}
}

var (
	reGoHandler = regexp.MustCompile(`(?i)[\s\\"']on(click|change|input|submit|error|load|keyup|keydown|focus|blur|mouseover|mouseout)\s*=\s*\\?["']`)
	reGoStyle   = regexp.MustCompile(`(?i)[\s\\"']style\s*=\s*\\?["']`)
)

func checkGoMarkup(path, src string, add func(string, int, string, ...any)) {
	code := reGoComment.ReplaceAllStringFunc(src, blank)
	for _, loc := range reGoHandler.FindAllStringIndex(code, -1) {
		add(path, lineAt(code, loc[0]), "inline event handler in generated HTML")
	}
	for _, loc := range reGoStyle.FindAllStringIndex(code, -1) {
		add(path, lineAt(code, loc[0]), "style attribute in generated HTML")
	}
	for _, loc := range reTemplJS.FindAllStringIndex(code, -1) {
		add(path, lineAt(code, loc[0]), "%s renders an inline event handler", code[loc[0]:loc[1]])
	}
}

var (
	reEval       = regexp.MustCompile(`\beval\s*\(|\bnew\s+Function\s*\(|\bset(Timeout|Interval)\s*\(\s*["'` + "`" + `]`)
	reHTMLSink   = regexp.MustCompile(`\.(innerHTML|outerHTML)\s*(\+?=)(\s*)([^=;\n][^;\n]*)|\.insertAdjacentHTML\s*\(\s*[^,]+,\s*([^)\n]*)|document\.write(ln)?\s*\(|\.srcdoc\s*=|\bcreateContextualFragment\s*\(`)
	reJSHandler  = regexp.MustCompile(`(?i)[\s"'` + "`" + `]on(click|change|input|submit|error|load|keyup|keydown|focus|blur|mouseover|mouseout)\s*=\s*\\?["']`)
	reJSStyle    = regexp.MustCompile(`(?i)<[a-z][^<>]*\sstyle\s*=\s*\\?["']`)
	reJSLineNote = regexp.MustCompile(`(?m)^\s*//.*$`)
	reParseFrom  = regexp.MustCompile(`\.parseFromString\s*\(`)
	reSetStyle   = regexp.MustCompile(`setAttribute\(\s*["']style["']`)
)

func checkScript(path, src string, firstLine int, add func(string, int, string, ...any)) {
	code := reJSLineNote.ReplaceAllStringFunc(src, blank)
	at := func(offset int) int { return firstLine + lineAt(code, offset) - 1 }
	isSecurityRuntime := strings.HasSuffix(filepath.ToSlash(path), "static/js/security.js")

	for _, loc := range reEval.FindAllStringIndex(code, -1) {
		add(path, at(loc[0]), "eval-like call: blocked without 'unsafe-eval'")
	}
	for _, m := range reHTMLSink.FindAllStringSubmatchIndex(code, -1) {
		if isSecurityRuntime {
			break
		}
		rhs := ""
		switch {
		case m[8] >= 0:
			rhs = code[m[8]:m[9]]
		case m[10] >= 0:
			rhs = code[m[10]:m[11]]
		}
		rhs = strings.TrimSpace(rhs)
		if rhs == "''" || rhs == `""` || strings.HasPrefix(rhs, "dawaHTML.") || strings.HasPrefix(rhs, "window.dawaHTML.") {
			continue
		}
		add(path, at(m[0]), "HTML sink with a plain string: wrap it in dawaHTML.sanitize/fromServer, or build nodes")
	}
	if !isSecurityRuntime {
		for _, loc := range reParseFrom.FindAllStringIndex(code, -1) {
			add(path, at(loc[0]), "DOMParser.parseFromString: use dawaHTML.parse")
		}
	}
	for _, loc := range reJSHandler.FindAllStringIndex(code, -1) {
		add(path, at(loc[0]), "inline event handler in built HTML")
	}
	for _, loc := range reJSStyle.FindAllStringIndex(code, -1) {
		add(path, at(loc[0]), "style attribute in built HTML")
	}
	for _, loc := range reSetStyle.FindAllStringIndex(code, -1) {
		add(path, at(loc[0]), "setAttribute('style'): set element.style properties instead")
	}
}
