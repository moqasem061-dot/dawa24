package telegram

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

// telegramFileClient downloads attachments. It never follows a redirect, so an
// accepted api.telegram.org URL cannot be bounced to another host.
var telegramFileClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// isTelegramFileURL reports whether raw is a Bot API file download address.
func isTelegramFileURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return false
	}
	return u.Scheme == "https" && strings.EqualFold(u.Hostname(), "api.telegram.org") &&
		(u.Port() == "" || u.Port() == "443") && strings.HasPrefix(u.Path, "/file/")
}
