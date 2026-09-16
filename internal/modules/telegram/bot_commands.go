package telegram

import (
	"context"
	"html"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
)

// htmlMarkup formats the shared command texts for Telegram's HTML parse mode.
type htmlMarkup struct{}

func (htmlMarkup) Escape(s string) string { return html.EscapeString(s) }
func (htmlMarkup) Bold(s string) string   { return "<b>" + s + "</b>" }
func (htmlMarkup) Code(s string) string   { return "<code>" + s + "</code>" }

func (s *Service) cmdWhoAmI(ctx context.Context, link *Link) (Reply, error) {
	return s.runCommand(link, func(c *chatbridge.Chat) (string, error) { return s.core.WhoAmI(ctx, c) })
}

func (s *Service) cmdOrg(ctx context.Context, link *Link, arg string) (Reply, error) {
	return s.runCommand(link, func(c *chatbridge.Chat) (string, error) { return s.core.Org(ctx, c, arg) })
}

func (s *Service) cmdNotify(ctx context.Context, link *Link, arg string) (Reply, error) {
	return s.runCommand(link, func(c *chatbridge.Chat) (string, error) { return s.core.Notify(ctx, c, arg) })
}

func (s *Service) runCommand(link *Link, run func(*chatbridge.Chat) (string, error)) (Reply, error) {
	chat := link.chat()
	text, err := run(chat)
	if err != nil {
		return Reply{}, err
	}
	link.adopt(chat)
	return replyTo(link.ChatID, text), nil
}
