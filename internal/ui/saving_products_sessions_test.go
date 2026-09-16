package ui

import (
	"encoding/json"
	"testing"
)

// The browser polls the import-progress endpoints with:
//
//	if (!sess || !sess.success) { alert('فشلت معالجة الجلسة.'); }
//
// The handlers encode SavingImportSession directly, so if the struct carries no
// `success` field the check reads `undefined` — falsy — and a perfectly healthy
// import reports failure on its very first poll and throws the user back to the
// upload screen. The failure paths encode their own object with success:false,
// so the two shapes have to agree.
func TestSavingImportSessionCarriesSuccessOnTheWire(t *testing.T) {
	raw, err := json.Marshal(&SavingImportSession{Success: true, ID: "abc", Progress: 40})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	got, present := wire["success"]
	if !present {
		t.Fatal("the session has no `success` field; the browser will read undefined and report failure")
	}
	if got != true {
		t.Fatalf("expected success:true, got %v", got)
	}
}

// A zero-valued session must not claim success, so a handler that forgets to set
// it fails loudly rather than reporting a broken import as healthy.
func TestZeroSessionDoesNotClaimSuccess(t *testing.T) {
	raw, _ := json.Marshal(&SavingImportSession{ID: "abc"})
	var wire map[string]any
	_ = json.Unmarshal(raw, &wire)

	if wire["success"] == true {
		t.Fatal("a session nobody marked successful must not report success")
	}
}
