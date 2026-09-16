// Package evals is the assistant's measuring stick.
//
// The work order that produced this package asks for a number before anything
// is built, and the same number after, because "the assistant is better now" is
// not a claim anybody can check. What follows is the corpus that number comes
// from: one hundred and eighty real questions in Arabic, sixty per dashboard,
// each paired with the tool that ought to answer it.
//
// Two things are measured, and they are deliberately separate.
//
// ROUTING (offline, runs in `go test -short`). Does a tool exist that can
// answer this question, and is it actually offered to a fully-granted user of
// that dashboard? This needs no Gateway, no database and no model, so it runs
// on every build and cannot rot. It is also how the coverage gap is computed
// rather than guessed: a question whose expected tool is not declared is a gap,
// and the list of gaps is the remaining tool backlog, printed by the test.
//
// ANSWERING (online, skipped unless a Gateway is configured). Does the model,
// given that question, actually call that tool and produce an answer? This is
// the pass rate the work order asks to move. It costs tokens and needs live
// data, so it is opt-in.
//
// The expected tool is written as the tool that SHOULD answer, not as the tool
// that exists today. A corpus that only ever names existing tools measures the
// implementation against itself and always scores well.
package evals

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

//go:embed corpus_*.jsonl
var corpusFS embed.FS

// RefuseTool marks a question the assistant must NOT answer with data: a
// cross-tenant probe, or a request to change something in a read-only surface.
//
// These are in the corpus for the same reason a test suite contains failing
// inputs. An assistant that answers all sixty pharmacy questions and also
// happily reports a competitor's catalogue has not scored 60/60; it has failed
// in the only way that matters.
const RefuseTool = "__refuse__"

// Case is one question and the tool that ought to answer it.
type Case struct {
	ID       string `json:"id"`
	Scope    string `json:"scope"`
	Question string `json:"q"`
	// Tool is the tool expected to answer, or RefuseTool when the correct
	// behaviour is a refusal.
	Tool string `json:"tool"`
	// Dataset is the dataset query_data, get_record or export_data should
	// read, when Tool is one of them.
	Dataset string `json:"dataset,omitempty"`
	// Action is the action propose_action should prepare.
	Action string `json:"action,omitempty"`
}

// MustRefuse reports whether this case expects a refusal rather than an answer.
func (c Case) MustRefuse() bool { return c.Tool == RefuseTool }

// DashboardScope maps the corpus's scope word onto the RBAC scope.
func (c Case) DashboardScope() rbac.Scope {
	switch c.Scope {
	case "pharmacy":
		return rbac.ScopePharmacy
	case "vendor":
		return rbac.ScopeVendor
	case "admin":
		return rbac.ScopeAdmin
	}
	return ""
}

// Load reads the whole corpus, in file then line order.
func Load() ([]Case, error) {
	names, err := fs.Glob(corpusFS, "corpus_*.jsonl")
	if err != nil {
		return nil, fmt.Errorf("evals: glob corpus: %w", err)
	}
	sort.Strings(names)

	var (
		out  []Case
		seen = map[string]string{}
	)
	for _, name := range names {
		f, err := corpusFS.Open(name)
		if err != nil {
			return nil, fmt.Errorf("evals: open %s: %w", name, err)
		}
		scanner := bufio.NewScanner(f)
		line := 0
		for scanner.Scan() {
			line++
			text := strings.TrimSpace(scanner.Text())
			if text == "" || strings.HasPrefix(text, "//") {
				continue
			}
			var c Case
			if err := json.Unmarshal([]byte(text), &c); err != nil {
				f.Close()
				return nil, fmt.Errorf("evals: %s line %d: %w", name, line, err)
			}
			if c.ID == "" || c.Question == "" || c.Tool == "" || (c.Tool == "propose_action" && c.Action == "") {
				f.Close()
				return nil, fmt.Errorf("evals: %s line %d: id, q and tool are all required", name, line)
			}
			if c.DashboardScope() == "" {
				f.Close()
				return nil, fmt.Errorf("evals: %s line %d: unknown scope %q", name, line, c.Scope)
			}
			// A duplicated id silently overwrites a result in any report that
			// keys by it, which turns a regression into a missing row.
			if prev, dup := seen[c.ID]; dup {
				f.Close()
				return nil, fmt.Errorf("evals: id %s appears in both %s and %s", c.ID, prev, name)
			}
			seen[c.ID] = name
			out = append(out, c)
		}
		err = scanner.Err()
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("evals: read %s: %w", name, err)
		}
	}
	return out, nil
}

// ByScope groups the corpus for per-dashboard reporting.
func ByScope(cases []Case) map[rbac.Scope][]Case {
	out := map[rbac.Scope][]Case{}
	for _, c := range cases {
		out[c.DashboardScope()] = append(out[c.DashboardScope()], c)
	}
	return out
}

// ExpectedTools returns the distinct tools the corpus expects for one scope,
// excluding refusal cases, in a stable order.
func ExpectedTools(cases []Case, scope rbac.Scope) []string {
	set := map[string]bool{}
	for _, c := range cases {
		if c.DashboardScope() == scope && !c.MustRefuse() {
			set[c.Tool] = true
		}
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
