package whatsapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Confirmations and files on WhatsApp.
//
// An action Capsule prepares arrives as a message with two reply buttons whose
// ids carry nothing but the proposal's public id and the choice. A press
// arrives as an interactive message from the number, which is handled like any
// message — the number must be the linked one and its link active — and then
// decided by chatbridge.Core.Decide, the path Telegram and the web drawer use.
//
// A spreadsheet Capsule exports arrives as a document: n8n downloads it from
// the bridge with its shared secret, uploads it to WhatsApp and sends it. The
// bridge serves an export only to a user who still has an active WhatsApp link.

const detailsShown = 12

// handleDecision decides a proposal from a pressed reply button.
func (s *Service) handleDecision(ctx context.Context, link *Link, buttonID string) ([]OutMessage, error) {
	confirm, proposalID, ok := chatbridge.ParseDecisionPayload(buttonID)
	if !ok {
		return nil, nil
	}
	if link.Status != LinkActive {
		return texts(link.WAID, s.notLinkedText()), nil
	}
	result, refusal, err := s.core.Decide(ctx, link.chat(), confirm, proposalID)
	if err != nil {
		return nil, err
	}
	if refusal != "" {
		return texts(link.WAID, refusal), nil
	}
	text := result.Message
	if result.URL != "" && s.cfg.BaseURL != "" {
		if u := safeHTTPURL(s.cfg.BaseURL + result.URL); u != "" {
			text += "\nفتح في Dawa24: " + u
		}
	}
	return texts(link.WAID, text), nil
}

// proposalMessage renders one proposal as a message with its buttons.
func proposalMessage(to string, p chatbridge.AnswerProposal) OutMessage {
	var b strings.Builder
	b.WriteString("📝 *" + p.Title + "*")
	if p.Summary != "" {
		b.WriteString("\n" + p.Summary)
	}
	for i, d := range p.Details {
		if i == detailsShown {
			fmt.Fprintf(&b, "\n… و%d تفاصيل أخرى في Dawa24", len(p.Details)-detailsShown)
			break
		}
		b.WriteString("\n• " + d[0] + ": *" + d[1] + "*")
	}
	for _, w := range p.Warnings {
		b.WriteString("\n⚠️ " + w)
	}
	b.WriteString("\n\n_لن يُنفَّذ شيء قبل ضغط «تأكيد»._")
	body := truncateRunes(b.String(), maxInteractiveBody)
	return OutMessage{To: to, Payload: buttonsPayload(to, body,
		ReplyTitle{ID: chatbridge.DecisionPayload(true, p.ID), Title: "✅ تأكيد"},
		ReplyTitle{ID: chatbridge.DecisionPayload(false, p.ID), Title: "✖️ إلغاء"},
	)}
}

// ExportPath is where n8n downloads an export from the bridge.
const ExportPath = "/api/v1/integrations/whatsapp/exports/"

func (s *Service) documentMessage(to string, f chatbridge.AnswerFile) (OutMessage, bool) {
	if s.cfg.BaseURL == "" || f.Token == "" {
		return OutMessage{}, false
	}
	return OutMessage{To: to, Document: &OutDocument{
		URL:      s.cfg.BaseURL + ExportPath + f.Token,
		Filename: f.Name,
		Caption:  "📎 " + f.Name,
	}}, true
}

// Export returns a stored export for n8n to send, only while its owner still
// has an active WhatsApp link.
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
