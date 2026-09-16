package ui_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The assistant drawer guards against regressions in layout, responsiveness and sizing.
func TestAssistantMessageListDoesNotClaimFullHeight(t *testing.T) {
	src := readAssistant(t)

	// Verify that messages container uses the class and does not set height:100% inline
	if !strings.Contains(src, `id="capsule-messages-container"`) {
		t.Fatal("could not find the messages container; if it was renamed, update this test rather than deleting it")
	}

	if strings.Contains(src, `id="capsule-messages-container" style="height:100%"`) {
		t.Error("messages container declares height:100%, which hides the composer")
	}

	// Verify the CSS rules for .capsule-messages-scroll in components.css
	if !strings.Contains(src, "min-height: 0") {
		t.Error("messages container needs min-height: 0 or it refuses to shrink")
	}
	if !strings.Contains(src, "flex: 1 1 0%") {
		t.Error("messages container must flex with basis 0")
	}
}

// Alpine wraps objects pushed into a reactive array in a proxy. Holding a
// reference to the ORIGINAL object and mutating it updates nothing, which is
// why the answer bubble stayed permanently empty while tokens streamed in.
//
// The assertion is on the technique, not on a variable name: the streaming
// target must be read back OUT of this.messages after the push, never bound to
// the literal that went in.
func TestAssistantStreamTargetIsTheReactiveProxy(t *testing.T) {
	src := readAssistant(t)

	if regexp.MustCompile(`const \w+ = \{[^}]*isStreaming`).MatchString(src) {
		t.Error("the streaming target is a raw object literal; mutations will not render. " +
			"Push first, then read the element back out of this.messages.")
	}
	if !regexp.MustCompile(`const \w+ = this\.messages\[this\.messages\.length - 1\]`).MatchString(src) {
		t.Error("the streaming target must reference the element inside this.messages " +
			"so Alpine observes it")
	}
}

// The drawer must not parse Server-Sent Events by hand.
//
// It used to, and the parser reset the current event name on every network
// chunk — so whenever a chunk boundary fell between "event: delta" and its
// "data:" line, the token was silently dropped. That is what produced "the
// answer stops when I resize or minimise the window": resizing changes the
// paint cadence, which changes where the chunks land.
//
// EventSource parses frames properly, reconnects on its own, and resends
// Last-Event-ID so the server replays exactly what was missed.
func TestAssistantStreamsWithEventSource(t *testing.T) {
	src := readAssistant(t)

	if !strings.Contains(src, "new EventSource(") {
		t.Error("the drawer must stream with EventSource, not a hand-rolled SSE parser")
	}
	for _, handRolled := range []string{
		"resp.body.getReader()",
		"new TextDecoder(",
		"currentEvent = 'message'",
	} {
		if strings.Contains(src, handRolled) {
			t.Errorf("hand-rolled SSE parsing is back: %q", handRolled)
		}
	}
}

// Asking and reading are separate requests, so an answer survives the browser
// going away: the turn is owned by the server and can be reattached to.
func TestAssistantUsesServerOwnedTurns(t *testing.T) {
	src := readAssistant(t)

	if !strings.Contains(src, "/api/v1/assistant/turns") {
		t.Error("the drawer must create a turn before streaming it")
	}
	if !strings.Contains(src, "running_turn") {
		t.Error("reopening a conversation must reattach to an answer still being written")
	}
}

// The retention promise is stated where the user is, not only in a policy page.
func TestAssistantStatesItsRetentionAndReadOnlyNature(t *testing.T) {
	src := readAssistant(t)

	if !strings.Contains(src, "retentionNote()") {
		t.Error("the drawer must tell the user when the conversation will be deleted")
	}
	if !strings.Contains(src, "ستة أشهر") && !strings.Contains(src, "٦ أشهر") {
		t.Error("the retention note must name the six-month period")
	}
	if !strings.Contains(src, "capsule-usage-circle-wrap") {
		t.Error("the drawer must show the circular context usage indicator")
	}
}

// The thinking state was a bordered card reading "جاري صياغة الإجابة…" that took
// more room than most answers and read as a stuck state.
func TestAssistantThinkingIndicatorIsNotACard(t *testing.T) {
	// Checked against the GENERATED file, not the template: the template still
	// mentions the old wording in the comment explaining why it went.
	b, err := os.ReadFile("components/capsule_assistant_messages_templ.go")
	if err != nil {
		b, err = os.ReadFile("components/capsule_assistant_templ.go")
		if err != nil {
			t.Fatalf("read generated assistant: %v", err)
		}
	}
	rendered := string(b)

	if strings.Contains(rendered, "جاري صياغة الإجابة") {
		t.Error("the thinking state should be an animated indicator, not a labelled card")
	}
	if !strings.Contains(rendered, "capsule-thinking") {
		t.Error("expected the capsule-thinking indicator in the rendered output")
	}
}

func readAssistant(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("components/capsule_assistant.templ")
	if err != nil {
		t.Fatalf("read assistant template: %v", err)
	}
	b2, _ := os.ReadFile("components/capsule_assistant_script.templ")
	b3, _ := os.ReadFile("static/css/components.css")
	b4, _ := os.ReadFile("components/capsule_assistant_messages.templ")
	b5, _ := os.ReadFile("components/capsule_assistant_overlays.templ")
	return string(b) + "\n" + string(b2) + "\n" + string(b3) + "\n" + string(b4) + "\n" + string(b5)
}

// The drawer's size was written as a style attribute, so no media query could
// touch it: the assistant had no small-screen behaviour and on a short viewport
// it ran past the bottom edge with the composer out of reach and the answer
// unscrollable.
func TestAssistantDrawerSizeIsStyleable(t *testing.T) {
	src := readAssistant(t)

	if strings.Contains(src, `height:620px`) {
		t.Error("the drawer height is inline again; a media query cannot override a style attribute")
	}
	if !strings.Contains(src, `class="capsule-drawer"`) {
		t.Error("the drawer should carry the capsule-drawer class, which is where its size lives")
	}
	for _, need := range []string{
		".capsule-drawer {",
		"@media (max-width: 640px)", // full-screen sheet on a phone
		"height: 100dvh",            // visible viewport, not layout viewport
	} {
		if !strings.Contains(src, need) {
			t.Errorf("missing drawer rule: %s", need)
		}
	}
}

// A flex child with basis auto sizes to its content first, so a long answer can
// push the composer out of the parent before min-height:0 is consulted. Basis 0
// makes the scroll area take the leftover space and nothing more.
func TestAssistantScrollAreaUsesDefiniteBasis(t *testing.T) {
	src := readAssistant(t)

	if !strings.Contains(src, "flex: 1 1 0%") {
		t.Error("scroll area must use flex: 1 1 0%, not basis auto")
	}
	if !strings.Contains(src, "overflow-y: auto") {
		t.Error("scroll area must scroll")
	}
}

// Every answer is from the assistant; repeating its name and avatar above each
// one is noise in a two-party chat and costs a line of the drawer.
func TestAssistantMessagesHaveNoPerMessageByline(t *testing.T) {
	b, err := os.ReadFile("components/capsule_assistant_templ.go")
	if err != nil {
		t.Fatalf("read generated assistant: %v", err)
	}
	rendered := string(b)

	// The header keeps its title, so exactly one occurrence is expected.
	if n := strings.Count(rendered, "دكتور كبسولة</span>"); n != 1 {
		t.Errorf("expected the drawer header title only, found %d occurrences of the byline", n)
	}
}
