package whatsapp

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Notifications on WhatsApp.
//
// Who is still owed a notification is chatbridge.Core.DecideNotifications, the
// rule Telegram applies too. WhatsApp adds the link rules Telegram has — the
// link must be active, and only notifications created after confirmation and
// recently are sent — and one of its own: a free-form message is accepted only
// within 24 hours of the user's last message. Outside that window a delivery
// travels as the configured approved template, or is dropped when there is
// none.

const (
	// notificationWindow is how old a notification may be and still be sent.
	notificationWindow = 2 * time.Hour
	candidateBatch     = 500
	deliveryLease      = 2 * time.Minute
	maxDeliveryTries   = 5
	// MaxClaim bounds one outbox claim.
	MaxClaim = 50
	// processedMessageRetention bounds the message de-duplication table.
	processedMessageRetention = 7 * 24 * time.Hour
	// rateLimitBackoff is the wait after WhatsApp reports a rate limit, which
	// it does without saying for how long.
	rateLimitBackoff = time.Minute
)

// ClaimOutbox evaluates new notifications and leases due messages to n8n.
func (s *Service) ClaimOutbox(ctx context.Context, limit int) ([]Outgoing, error) {
	sys := database.AsSystem(ctx)
	if limit <= 0 || limit > MaxClaim {
		limit = MaxClaim
	}
	if err := s.materialize(sys); err != nil {
		// Leasing what is already queued is still worth doing.
		s.log.ErrorContext(ctx, "whatsapp: evaluate notifications", "error", err)
	}
	if err := s.repo.PurgeProcessedMessages(sys, s.now().Add(-processedMessageRetention)); err != nil {
		s.log.WarnContext(ctx, "whatsapp: purge processed messages", "error", err)
	}
	leased, err := s.repo.ClaimDeliveries(sys, limit, deliveryLease)
	if err != nil {
		return nil, err
	}
	out := make([]Outgoing, 0, len(leased))
	for _, d := range leased {
		payload := s.deliveryPayload(d)
		if payload == nil {
			if err := s.repo.DropDelivery(sys, d.ID, "window_closed"); err != nil {
				return nil, err
			}
			continue
		}
		out = append(out, Outgoing{ID: d.ID, To: d.WAID, Payload: payload})
	}
	return out, nil
}

// deliveryPayload is a text inside the 24-hour window, the template outside
// it, and nil when the window is closed and no template is configured.
func (s *Service) deliveryPayload(d LeasedDelivery) *Payload {
	switch {
	case d.WindowOpen:
		return textPayload(d.WAID, d.Text)
	case s.cfg.NotificationTemplate != "":
		return templatePayload(d.WAID, s.cfg.NotificationTemplate, s.cfg.TemplateLanguage, d.Text)
	}
	return nil
}

func (s *Service) materialize(sys context.Context) error {
	candidates, err := s.repo.NotificationCandidates(sys, s.now().Add(-notificationWindow), candidateBatch)
	if err != nil || len(candidates) == 0 {
		return err
	}
	decisions, err := s.core.DecideNotifications(sys, candidates, s.notificationText)
	if err != nil {
		return err
	}
	return s.repo.RecordDecisions(sys, decisions)
}

func (s *Service) notificationText(c chatbridge.Candidate) string {
	var b strings.Builder
	b.WriteString("🔔 *" + strings.TrimSpace(c.Title) + "*")
	if body := strings.TrimSpace(c.Body); body != "" {
		b.WriteString("\n" + body)
	}
	if c.OrganizationID > 0 && c.OrganizationID != c.LinkActiveOrgID && c.OrganizationName != "" {
		b.WriteString("\n\n🏢 " + c.OrganizationName)
	}
	if u := safeHTTPURL(s.cfg.BaseURL + "/notifications"); u != "" {
		b.WriteString("\n\nعرض الإشعارات في Dawa24: " + u)
	}
	return truncateRunes(b.String(), MaxMessageUnits)
}

// Cloud API error codes, as they appear in the error text n8n reports.
// https://developers.facebook.com/docs/whatsapp/cloud-api/support/error-codes
var (
	reErrorCode = regexp.MustCompile(`\b\d{3,6}\b`)
	// unreachable: the number is not on WhatsApp, cannot receive, or is the
	// sender itself. Nothing more is sent until the user writes again.
	unreachable = codeSet("131026", "131021", "133010")
	// windowClosed: the 24-hour customer-service window has closed.
	windowClosed = codeSet("131047")
	// rateLimited: throughput, pair and spam rate limits.
	rateLimited = codeSet("130429", "131056", "131048", "80007")
	// unsendable: the message itself is wrong and will never be accepted —
	// invalid parameters or a template that does not exist or does not fit.
	unsendable = codeSet("100", "131008", "131009", "132000", "132001", "132005", "132007", "132012", "132015", "132016")
)

func codeSet(codes ...string) map[string]bool {
	m := make(map[string]bool, len(codes))
	for _, c := range codes {
		m[c] = true
	}
	return m
}

func hasCode(msg string, set map[string]bool) bool {
	for _, c := range reErrorCode.FindAllString(msg, -1) {
		if set[c] {
			return true
		}
	}
	return false
}

// ReportDeliveries records what happened to leased messages.
func (s *Service) ReportDeliveries(ctx context.Context, results []DeliveryResult) error {
	sys := database.AsSystem(ctx)
	for _, r := range results {
		if r.ID <= 0 {
			continue
		}
		errText := truncateRunes(r.Error, 500)
		var err error
		switch {
		case r.OK:
			err = s.repo.MarkDelivered(sys, r.ID)
		case hasCode(r.Error, unreachable):
			err = s.repo.FailDeliveryAndBlockLink(sys, r.ID, errText)
		case hasCode(r.Error, windowClosed):
			err = s.repo.CloseWindowAndRetry(sys, r.ID, errText, maxDeliveryTries)
		case hasCode(r.Error, rateLimited):
			err = s.repo.RetryDelivery(sys, r.ID, errText, rateLimitBackoff, maxDeliveryTries)
		case hasCode(r.Error, unsendable):
			err = s.repo.RetryDelivery(sys, r.ID, errText, 0, 0)
		default:
			err = s.repo.RetryDelivery(sys, r.ID, errText, 0, maxDeliveryTries)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
