package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Confirmations and files on Telegram.
//
// An action Capsule prepares arrives as its own message with two inline
// buttons. The buttons carry nothing but the proposal's public id and the
// decision. Pressing one produces a callback update, which is handled like a
// message: the chat must be private, the presser must be the linked Telegram
// account, their Dawa24 grant is resolved again, and the assistant's own
// confirm path — the one the web drawer uses — decides and executes.
//
// A spreadsheet Capsule exports arrives as a document. n8n downloads it from
// the bridge with its shared secret; the bridge serves an export only to a user
// who still has an active Telegram link.

// CallbackQuery is a pressed inline button.
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}

// InlineKeyboard is Telegram's reply_markup for inline buttons.
type InlineKeyboard struct {
	Rows [][]InlineButton `json:"inline_keyboard"`
}

// InlineButton is one inline button.
type InlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

// OutDocument is a file for n8n to download from the bridge and send.
type OutDocument struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
}

// CallbackAnswer stops the button's loading spinner.
type CallbackAnswer struct {
	ID   string `json:"callback_query_id"`
	Text string `json:"text,omitempty"`
}

// handleCallback decides a proposal from a pressed button.
func (s *Service) handleCallback(ctx context.Context, updateID int64, q *CallbackQuery) (Reply, error) {
	sys := database.AsSystem(ctx)
	if q.Message == nil || q.Message.Chat.Type != "private" || q.Message.Chat.ID != q.From.ID || q.From.IsBot {
		return Reply{}, nil
	}
	first, err := s.repo.MarkUpdateProcessed(sys, updateID)
	if err != nil || !first {
		return Reply{}, err
	}
	chatID := q.Message.Chat.ID
	reply := Reply{AnswerCallback: &CallbackAnswer{ID: q.ID}}

	confirm, proposalID, ok := chatbridge.ParseDecisionPayload(q.Data)
	if !ok {
		return reply, nil
	}
	link, err := s.repo.LiveLinkByTelegramUser(sys, q.From.ID)
	if err != nil {
		return Reply{}, err
	}
	if link == nil || link.Status != LinkActive {
		reply.Messages = []OutMessage{{ChatID: chatID, Text: s.notLinkedText()}}
		return reply, nil
	}
	result, refusal, err := s.core.Decide(ctx, link.chat(), confirm, proposalID)
	if err != nil {
		return Reply{}, err
	}
	if refusal != "" {
		reply.Messages = []OutMessage{{ChatID: chatID, Text: refusal}}
		return reply, nil
	}

	reply.AnswerCallback.Text = truncate(result.Message, 180)
	text := escape(result.Message)
	if result.URL != "" && s.cfg.BaseURL != "" {
		if u := safeHTTPURL(s.cfg.BaseURL + result.URL); u != "" {
			text += "\n<a href=\"" + escape(u) + "\">فتح في Dawa24</a>"
		}
	}
	reply.Messages = []OutMessage{{ChatID: chatID, Text: text}}
	return reply, nil
}

// proposalMessage renders one proposal as a message with its buttons.
func proposalMessage(chatID int64, p chatbridge.AnswerProposal) OutMessage {
	var b strings.Builder
	b.WriteString("📝 <b>" + escape(p.Title) + "</b>")
	if p.Summary != "" {
		b.WriteString("\n" + escape(p.Summary))
	}
	for i, d := range p.Details {
		if i == 12 {
			b.WriteString(fmt.Sprintf("\n… و%d تفاصيل أخرى في Dawa24", len(p.Details)-12))
			break
		}
		b.WriteString("\n• " + escape(d[0]) + ": <b>" + escape(d[1]) + "</b>")
	}
	for _, w := range p.Warnings {
		b.WriteString("\n⚠️ " + escape(w))
	}
	b.WriteString("\n\n<i>لن يُنفَّذ شيء قبل ضغط «تأكيد».</i>")
	text := b.String()
	if units(text) > MaxMessageUnits {
		text = escape(splitUnits(plain(text), MaxMessageUnits)[0])
	}
	return OutMessage{
		ChatID: chatID,
		Text:   text,
		ReplyMarkup: &InlineKeyboard{Rows: [][]InlineButton{{
			{Text: "✅ تأكيد", CallbackData: chatbridge.DecisionPayload(true, p.ID)},
			{Text: "✖️ إلغاء", CallbackData: chatbridge.DecisionPayload(false, p.ID)},
		}}},
	}
}

// exportPath is where n8n downloads an export from the bridge.
const exportPath = "/api/v1/integrations/telegram/exports/"

func (s *Service) documentMessage(chatID int64, f chatbridge.AnswerFile) (OutMessage, bool) {
	if s.cfg.BaseURL == "" || f.Token == "" {
		return OutMessage{}, false
	}
	return OutMessage{
		ChatID:   chatID,
		Text:     "📎 " + escape(f.Name),
		Document: &OutDocument{URL: strings.TrimRight(s.cfg.BaseURL, "/") + exportPath + f.Token, Filename: f.Name},
	}, true
}

// Export returns a stored export for n8n to send, only while its owner still
// has an active Telegram link.
func (s *Service) Export(ctx context.Context, token string) (*chatbridge.ExportFile, error) {
	file, err := s.assistant.Export(ctx, token)
	if err != nil || file == nil {
		return nil, err
	}
	link, err := s.repo.CurrentLinkForUser(database.AsSystem(ctx), file.UserID)
	if err != nil {
		return nil, err
	}
	if link == nil || link.Status != LinkActive {
		return nil, nil
	}
	return file, nil
}
