package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var telegramActionClient = &http.Client{
	Timeout: 2 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// keepTyping refreshes the initial n8n action before Telegram's five-second
// expiry. Joining on stop prevents an in-flight action reappearing after reply.
func (s *Service) keepTyping(ctx context.Context, chatID int64) func() {
	if s.cfg.BotToken == "" {
		return func() {}
	}
	typingCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-typingCtx.Done():
				return
			case <-ticker.C:
				if typingCtx.Err() != nil {
					return
				}
				s.sendTyping(typingCtx, chatID)
			}
		}
	}()
	return func() { cancel(); <-done }
}

func (s *Service) sendTyping(ctx context.Context, chatID int64) {
	form := url.Values{"chat_id": {strconv.FormatInt(chatID, 10)}, "action": {"typing"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.telegram.org/bot"+s.cfg.BotToken+"/sendChatAction", strings.NewReader(form.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := telegramActionClient.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			// Bot API request errors contain the token in the URL.
			s.log.WarnContext(ctx, "telegram: typing action unavailable")
		}
		return
	}
	defer resp.Body.Close()
	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err != nil || !result.OK || resp.StatusCode != http.StatusOK {
		s.log.WarnContext(ctx, "telegram: typing action rejected", "status", resp.StatusCode)
	}
}
