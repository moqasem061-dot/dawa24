package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// The link reaches the page in an HX-Trigger event, and an empty link is how
// the page learns there is nothing to open. Both must survive encoding.
func TestTelegramLinkTriggerCarriesTheLinkOrNothing(t *testing.T) {
	cases := []struct {
		name string
		link string
	}{
		{"bot link", "https://t.me/dawa24_bot?start=abc123"},
		{"nothing to open", ""},
		{"characters that need escaping", `https://t.me/dawa24_bot?start=a"b\c<d>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			setTelegramLinkTrigger(rec, tc.link)

			var got map[string]map[string]string
			if err := json.Unmarshal([]byte(rec.Header().Get("HX-Trigger")), &got); err != nil {
				t.Fatalf("HX-Trigger is not valid JSON: %v", err)
			}
			if url, ok := got[telegramLinkEvent]["url"]; !ok || url != tc.link {
				t.Fatalf("event %q url = %q (present %v), want %q", telegramLinkEvent, url, ok, tc.link)
			}
		})
	}
}

// The connect button is the element telegram-connect.js opens the tab from.
// Without the marker the tab is never opened in the click and the link is
// blocked by the browser.
func TestTelegramConnectButtonIsMarkedForTheOneClickFlow(t *testing.T) {
	var buf bytes.Buffer
	data := pages.TelegramSettingsData{Enabled: true}
	if err := pages.TelegramSettingsCard(data, "ar").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render card: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, `data-telegram-connect="1"`) {
		t.Fatal("connect button lost its data-telegram-connect marker")
	}
	if !strings.Contains(html, `hx-post="/settings/telegram/link"`) {
		t.Fatal("connect button no longer posts to the link endpoint")
	}
}
