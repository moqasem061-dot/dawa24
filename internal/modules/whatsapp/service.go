package whatsapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Config is what the bridge needs to know about its surroundings.
type Config struct {
	// BusinessNumber is the WhatsApp Business number in international form
	// without "+", for wa.me links.
	BusinessNumber string
	// BaseURL is the public site address, for links back into the dashboard.
	BaseURL string
	// NotificationTemplate is an approved utility template with one body
	// parameter, used for notifications outside the 24-hour window. Empty
	// drops such notifications instead.
	NotificationTemplate string
	// TemplateLanguage is the template's language code, e.g. "ar".
	TemplateLanguage string
}

const (
	// linkCodeTTL bounds how long a link code is worth anything.
	linkCodeTTL = 10 * time.Minute
	// confirmTTL bounds how long a pending link waits for the browser.
	confirmTTL = 10 * time.Minute
	// maxCodesPerWindow and codeWindow rate-limit link code creation.
	maxCodesPerWindow = 5
	codeWindow        = 10 * time.Minute
	// linkMessagePrefix starts the message a wa.me link pre-fills.
	linkMessagePrefix = "ربط Dawa24"
)

// Service is the WhatsApp bridge.
type Service struct {
	repo      Repository
	assistant chatbridge.Assistant
	core      *chatbridge.Core
	cfg       Config
	log       *slog.Logger
	now       func() time.Time
}

// NewService constructs the bridge.
func NewService(repo Repository, grants chatbridge.GrantResolver, assistant chatbridge.Assistant, cfg Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	cfg.BusinessNumber = strings.TrimPrefix(strings.TrimSpace(cfg.BusinessNumber), "+")
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.TemplateLanguage == "" {
		cfg.TemplateLanguage = "ar"
	}
	s := &Service{
		repo:      repo,
		assistant: assistant,
		cfg:       cfg,
		log:       log.With("module", "whatsapp"),
		now:       time.Now,
	}
	s.core = &chatbridge.Core{
		Grants: grants, Store: repo, Assistant: assistant, Markup: waMarkup{},
		ChannelName: "واتساب", Log: s.log, Now: func() time.Time { return s.now() },
	}
	return s
}

// Enabled reports whether a business number is configured at all.
func (s *Service) Enabled() bool { return s != nil && s.cfg.BusinessNumber != "" }

// LinkCode is a freshly issued one-time code.
type LinkCode struct {
	DeepLink  string
	ExpiresAt time.Time
}

// StartLink issues a one-time link code for the signed-in user.
//
// The code is 32 random bytes; only its hash is stored. The wa.me link opens a
// chat with the business number with the code pre-filled; the user sends it.
// A new code invalidates any unused older one.
func (s *Service) StartLink(ctx context.Context, actor authctx.Actor) (*LinkCode, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	if actor.UserID <= 0 {
		return nil, errors.New("whatsapp: start link requires a signed-in user")
	}
	ctx = database.AsSystem(ctx)
	n, err := s.repo.CountLinkTokensSince(ctx, actor.UserID, s.now().Add(-codeWindow))
	if err != nil {
		return nil, err
	}
	if n >= maxCodesPerWindow {
		return nil, ErrTooManyCodes
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("whatsapp: generate link code: %w", err)
	}
	code := base64.RawURLEncoding.EncodeToString(raw)

	var orgID *int64
	if actor.OrgID > 0 {
		id := actor.OrgID
		orgID = &id
	}
	expires := s.now().Add(linkCodeTTL)
	if err := s.repo.CreateLinkToken(ctx, actor.UserID, orgID, hashCode(code), expires); err != nil {
		return nil, err
	}
	text := url.QueryEscape(linkMessagePrefix + " " + code)
	return &LinkCode{
		DeepLink:  "https://wa.me/" + s.cfg.BusinessNumber + "?text=" + text,
		ExpiresAt: expires,
	}, nil
}

// CurrentLink returns the user's pending or confirmed link, nil when none.
func (s *Service) CurrentLink(ctx context.Context, userID int64) (*Link, error) {
	if userID <= 0 {
		return nil, nil
	}
	return s.repo.CurrentLinkForUser(database.AsSystem(ctx), userID)
}

// ConfirmLink is the browser half of linking: the signed-in user confirms that
// the WhatsApp number that sent their code is theirs. Nothing is answered or
// delivered before this, which is what makes a leaked code harmless.
func (s *Service) ConfirmLink(ctx context.Context, actor authctx.Actor, publicID string) (*Link, error) {
	ctx = database.AsSystem(ctx)
	link, err := s.repo.ConfirmPendingLink(ctx, actor.UserID, publicID)
	if err != nil {
		return nil, err
	}
	msg := "✅ *تم ربط حسابك في Dawa24 بواتساب.*\n\n" +
		"يمكنك الآن سؤال المساعد كبسولة مباشرة هنا، وستصلك إشعارات منشأتك حسب صلاحياتك.\n" +
		"اكتب /help لعرض الأوامر."
	if err := s.repo.EnqueueSystemMessage(ctx, link.ID, msg); err != nil {
		s.log.WarnContext(ctx, "whatsapp: enqueue welcome", "error", err, "user_id", actor.UserID)
	}
	s.log.InfoContext(ctx, "whatsapp link confirmed", "user_id", actor.UserID, "link_id", link.ID)
	return link, nil
}

// Unlink revokes the user's pending or confirmed link. Rejecting a pending
// link the user does not recognise is the same operation.
func (s *Service) Unlink(ctx context.Context, actor authctx.Actor) error {
	ctx = database.AsSystem(ctx)
	link, err := s.repo.CurrentLinkForUser(ctx, actor.UserID)
	if err != nil || link == nil {
		return err
	}
	if err := s.repo.RevokeLink(ctx, actor.UserID, link.ID); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "whatsapp link revoked", "user_id", actor.UserID, "link_id", link.ID)
	return nil
}

// SetMuted replaces the user's muted notification categories.
func (s *Service) SetMuted(ctx context.Context, actor authctx.Actor, muted []chatbridge.Category) error {
	ctx = database.AsSystem(ctx)
	link, err := s.repo.CurrentLinkForUser(ctx, actor.UserID)
	if err != nil {
		return err
	}
	if link == nil || link.Status == LinkPending {
		return ErrNoPendingLink
	}
	return s.repo.SetMutedCategories(ctx, link.ID, chatbridge.CategoryKeys(muted))
}

func hashCode(code string) []byte {
	sum := sha256.Sum256([]byte(code))
	return sum[:]
}
