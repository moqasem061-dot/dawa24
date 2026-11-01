package pages

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
)

func renderInbox(t *testing.T, messages ...*platformadmin.ContactMessage) string {
	t.Helper()
	var buf bytes.Buffer
	if err := AdminMessages("ar", "rtl", messages, 1, 20, len(messages)).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render inbox: %v", err)
	}
	return buf.String()
}

func sampleMessage(id int64, status string) *platformadmin.ContactMessage {
	userID := int64(42)
	return &platformadmin.ContactMessage{
		ID:        id,
		Name:      "سامح عبد الله",
		Email:     "sameh@example.com",
		Phone:     "01012345678",
		Subject:   "شكوى صيدلية",
		Status:    status,
		IP:        "41.32.10.7",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36",
		UserID:    &userID,
		CreatedAt: time.Date(2026, 10, 30, 14, 5, 0, 0, time.UTC),
	}
}

// The whole point of the redesign: the free-text body is read in a dialog, not
// printed into the table where it widened the column to its length.
func TestAdminInboxOpensMessageBodyInDialog(t *testing.T) {
	m := sampleMessage(7, "unread")
	m.Message = strings.Repeat("تفاصيل الشكوى طويلة جدا ", 30) + "نهاية-الرسالة"
	html := renderInbox(t, m)

	if got := strings.Count(html, "نهاية-الرسالة"); got != 1 {
		t.Errorf("the end of the message appears %d times, want exactly once (in the dialog)", got)
	}
	if !strings.Contains(html, `id="admin-message-modal-7"`) {
		t.Error("expected a dialog carrying the message")
	}
	if !strings.Contains(html, `data-modal-open="admin-message-modal-7"`) {
		t.Error("expected a button that opens the message dialog")
	}
	if !strings.Contains(html, `class="inbox-message-body"`) {
		t.Error("expected the message body to render in the dialog's body panel")
	}
	// The body sits after the dialog opens, not above it in a table cell.
	bodyAt := strings.Index(html, "class=\"inbox-message-body\"")
	dialogAt := strings.Index(html, `id="admin-message-modal-7"`)
	if bodyAt < 0 || dialogAt < 0 || bodyAt < dialogAt {
		t.Error("expected the message body to be rendered inside the dialog")
	}
}

// The toggle form states the status it wants. The handler no longer re-reads the
// inbox to discover the current one.
func TestAdminInboxToggleCarriesTargetStatus(t *testing.T) {
	unreadHTML := renderInbox(t, sampleMessage(7, "unread"))
	if !strings.Contains(unreadHTML, `name="status" value="read"`) {
		t.Error("an unread message's toggle should ask for status=read")
	}

	readHTML := renderInbox(t, sampleMessage(8, "read"))
	if !strings.Contains(readHTML, `name="status" value="unread"`) {
		t.Error("a read message's toggle should ask for status=unread")
	}
}

// Unread is what an operator scans for, so it is the only row that carries the
// tint and the dot.
func TestAdminInboxMarksUnreadRowsOnly(t *testing.T) {
	html := renderInbox(t, sampleMessage(7, "unread"), sampleMessage(8, "read"))
	if got := strings.Count(html, "is-unread"); got < 1 {
		t.Error("expected the unread row to be marked")
	}
	rows := strings.Split(html, "<tr")
	unreadMarked, readMarked := 0, 0
	for _, row := range rows {
		if !strings.Contains(row, "inbox-row") {
			continue
		}
		if strings.Contains(row, "is-unread") {
			unreadMarked++
			continue
		}
		readMarked++
	}
	if unreadMarked != 1 || readMarked != 1 {
		t.Errorf("got %d unread rows and %d read rows, want 1 and 1", unreadMarked, readMarked)
	}
}

func TestMessagePreviewCollapsesAndTruncates(t *testing.T) {
	if got := messagePreview("  سطر أول\n\nسطر   ثانٍ  ", 100); got != "سطر أول سطر ثانٍ" {
		t.Errorf("preview = %q, want the whitespace collapsed to one line", got)
	}
	long := strings.Repeat("كلمة ", 40)
	got := messagePreview(long, 20)
	if runes := []rune(got); len(runes) > 21 {
		t.Errorf("preview = %q (%d runes), want it cut near 20", got, len(runes))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("preview = %q, want an ellipsis so the reader knows text is hidden", got)
	}
	if got := messagePreview("   ", 20); got == "" {
		t.Error("an empty body should still render a placeholder, not a blank cell")
	}
}

func TestSenderInitials(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"سامح عبد الله", "سع"},
		{"Sameh", "S"},
		{"  ", "؟"},
	} {
		if got := senderInitials(tc.name); got != tc.want {
			t.Errorf("senderInitials(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestMessageStatusMapping(t *testing.T) {
	for _, tc := range []struct{ status, label, class, target string }{
		{"unread", "غير مقروءة", "badge-rose", "read"},
		{"", "غير مقروءة", "badge-rose", "read"},
		{"read", "تم الرد / مكتملة", "badge-emerald", "unread"},
		{"resolved", "تم الرد / مكتملة", "badge-emerald", "unread"},
		{"in_progress", "قيد المتابعة", "badge-warning", "read"},
	} {
		if got := messageStatusLabel(tc.status); got != tc.label {
			t.Errorf("label(%q) = %q, want %q", tc.status, got, tc.label)
		}
		if got := messageStatusClass(tc.status); got != tc.class {
			t.Errorf("class(%q) = %q, want %q", tc.status, got, tc.class)
		}
		if got := messageToggleTarget(tc.status); got != tc.target {
			t.Errorf("toggleTarget(%q) = %q, want %q", tc.status, got, tc.target)
		}
	}
}
