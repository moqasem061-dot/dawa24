package telegram

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

func TestIsTelegramFileURL(t *testing.T) {
	cases := map[string]bool{
		"https://api.telegram.org/file/bot123:ABC/photos/file_1.jpg": true,
		"http://api.telegram.org/file/bot1/x":                        false,
		"https://api.telegram.org/bot123:ABC/getMe":                  false,
		"https://api.telegram.org.evil.example/file/x":               false,
		"https://user@api.telegram.org/file/x":                       false,
		"https://api.telegram.org:8443/file/x":                       false,
		"http://169.254.169.254/latest/meta-data/":                   false,
		"https://localhost/file/x":                                   false,
	}
	for raw, want := range cases {
		if got := isTelegramFileURL(raw); got != want {
			t.Errorf("isTelegramFileURL(%q) = %v; want %v", raw, got, want)
		}
	}
}

func TestDownloadFileBoundsInlineData(t *testing.T) {
	svc := &Service{}
	oversized := strings.Repeat("A", base64.StdEncoding.EncodedLen(maxTelegramFileSize)+1)
	if _, err := svc.DownloadFile(context.Background(), "", "", oversized); err == nil {
		t.Fatal("oversized inline attachment accepted")
	}
	content, err := svc.DownloadFile(context.Background(), "", "", base64.StdEncoding.EncodeToString([]byte("fixture")))
	if err != nil || string(content) != "fixture" {
		t.Fatalf("valid inline content=%q err=%v", content, err)
	}
}
