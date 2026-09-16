package corpus

import (
	"encoding/json"
	"os"
	"testing"
)

// baselineFile records the last committed corpus result.
//
// It is written by `go test ./test/corpus -update`, read by everyone else, and
// diffed in review. A refactor that changes matching should change this file in
// the same commit, so the reviewer sees the effect rather than taking the
// author's word for it.
const baselineFile = "baseline.json"

// TestCorpusReport scores every corpus file and prints the report.
//
// It fails only when a file the engine used to read becomes unreadable, or when
// the settled rate on a file drops. Rates going *up* rewrite nothing on their
// own: the baseline is updated deliberately, because "the number moved" and
// "the number moved for the reason I intended" are different claims.
func TestCorpusReport(t *testing.T) {
	if !Available() {
		t.Skip("corpus not exported; run `go run ./cmd/cli corpus-export`")
	}

	entries, err := LoadManifest()
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	idx, err := LoadIndex()
	if err != nil {
		t.Fatalf("catalogue snapshot: %v", err)
	}
	t.Logf("catalogue: %d products", idx.Size())

	reports := make([]FileReport, 0, len(entries))
	for _, e := range entries {
		rep := Run(idx, e)
		reports = append(reports, rep)
		t.Log(rep.Format())
	}

	if os.Getenv("CORPUS_UPDATE") != "" {
		writeBaseline(t, reports)
		return
	}
	compareBaseline(t, reports)
}

func writeBaseline(t *testing.T, reports []FileReport) {
	t.Helper()
	raw, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		t.Fatalf("encode baseline: %v", err)
	}
	if err := os.WriteFile(baselineFile, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
	t.Logf("baseline updated: %s", baselineFile)
}

// compareBaseline reports every file whose outcome moved, and fails on the
// movements that are always regressions.
func compareBaseline(t *testing.T, reports []FileReport) {
	t.Helper()
	raw, err := os.ReadFile(baselineFile)
	if err != nil {
		t.Skipf("no baseline yet; run with CORPUS_UPDATE=1 to record one")
		return
	}
	var before []FileReport
	if err := json.Unmarshal(raw, &before); err != nil {
		t.Fatalf("decode baseline: %v", err)
	}

	was := make(map[string]FileReport, len(before))
	for _, r := range before {
		was[r.Entry.File] = r
	}

	for _, now := range reports {
		old, known := was[now.Entry.File]
		if !known {
			t.Logf("NEW    %s", now.Entry.File)
			continue
		}
		switch {
		case old.Error == "" && now.Error != "":
			t.Errorf("REGRESSION %s: was readable, now fails: %s", now.Entry.File, now.Error)
		case now.Parsed < old.Parsed:
			t.Errorf("REGRESSION %s: parsed %d -> %d", now.Entry.File, old.Parsed, now.Parsed)
		case now.SettledPct < old.SettledPct:
			t.Errorf("REGRESSION %s: settled %d%% -> %d%%", now.Entry.File, old.SettledPct, now.SettledPct)
		case now.SettledPct > old.SettledPct:
			t.Logf("IMPROVED %s: settled %d%% -> %d%%", now.Entry.File, old.SettledPct, now.SettledPct)
		}
	}
}
