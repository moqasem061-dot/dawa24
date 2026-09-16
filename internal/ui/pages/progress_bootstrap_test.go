package pages_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	scriptBlock = regexp.MustCompile(`(?s)<script\b[^>]*>(.*?)</script>`)
	// Comments are stripped before anything is matched. The first version of
	// this gate looked for the word "DOMContentLoaded" and was satisfied by the
	// COMMENT explaining why it is needed — so removing the actual listener
	// still passed. A gate that a comment can satisfy protects nothing.
	lineComment  = regexp.MustCompile(`(?m)//.*$`)
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	// The listener itself, not a mention of it.
	readyFence = regexp.MustCompile(`addEventListener\(\s*['"]DOMContentLoaded['"]`)
)

// stripComments removes commentary so only executable text is examined.
func stripComments(s string) string {
	return lineComment.ReplaceAllString(blockComment.ReplaceAllString(s, ""), "")
}

// A progress bar wired up at parse time never starts.
//
// import-progress.js and upload-progress.js are loaded with `defer` in the head
// (layouts/base.templ), which means they execute AFTER the document is parsed.
// An inline <script> in the body executes DURING parsing — so at that moment
// window.ImportProgress does not exist yet.
//
// Every one of these call sites guarded itself with
//
//	if (typeof window.ImportProgress !== 'function') return;
//
// and that guard did exactly what it says: it returned. Silently, on every page
// load, with no error anywhere. The bar then kept whatever percentage the
// server had rendered into the HTML and never moved again — the vendor import
// sat at 1% while the run behind it finished, wrote 1,135 rows and moved to
// review, because nothing was ever polling to notice.
//
// The fix is a readiness fence: DOMContentLoaded fires only after every
// deferred script has run, and a document that is already parsed runs the
// initialiser immediately. This test requires one wherever the bars are started
// from an immediately-invoked block.
func TestProgressBarsWaitForTheirDeferredScript(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join(".", "*.templ"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no page templates found; this gate is not looking where it thinks it is")
	}

	checked := 0
	for _, path := range matches {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, block := range scriptBlock.FindAllStringSubmatch(string(src), -1) {
			body := stripComments(block[1])

			usesBar := strings.Contains(body, "window.ImportProgress") ||
				strings.Contains(body, "window.UploadProgress")
			if !usesBar {
				continue
			}
			// Only immediately-invoked blocks run at parse time. A script that
			// merely DEFINES a function is fine: whatever calls it does so from
			// an event handler, long after the deferred scripts have run.
			if !strings.Contains(body, "})();") && !strings.Contains(body, "})()") {
				continue
			}
			checked++

			if !readyFence.MatchString(body) {
				t.Errorf("%s: an inline script starts a progress bar immediately, "+
					"but import-progress.js is deferred and has not executed yet.\n"+
					"  The bar will silently never start. Wrap the initialiser:\n"+
					"    function startProgress() { ... }\n"+
					"    if (document.readyState === 'loading') {\n"+
					"      document.addEventListener('DOMContentLoaded', startProgress);\n"+
					"    } else { startProgress(); }", path)
			}
		}
	}

	if checked == 0 {
		t.Fatal("found no immediately-invoked progress-bar initialisers at all; " +
			"the pattern this gate matches has drifted from the templates")
	}
}
