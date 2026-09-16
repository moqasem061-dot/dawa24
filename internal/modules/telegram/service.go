package telegram

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Config is what the bridge needs to know about its surroundings.
type Config struct {
	// BotUsername is the bot's @name without the @, for t.me deep links.
	BotUsername string
	// BaseURL is the public site address, for links back into the dashboard.
	BaseURL string
	// BotToken is the optional Telegram Bot API token for downloading attachments.
	BotToken string
}

const (
	// linkCodeTTL bounds how long a link code is worth anything.
	linkCodeTTL = 10 * time.Minute
	// confirmTTL bounds how long a pending link waits for the browser.
	confirmTTL = 10 * time.Minute
	// maxCodesPerWindow and codeWindow rate-limit link code creation.
	maxCodesPerWindow = 5
	codeWindow        = 10 * time.Minute
)

// Service is the Telegram bridge.
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
	cfg.BotUsername = strings.TrimPrefix(strings.TrimSpace(cfg.BotUsername), "@")
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	s := &Service{
		repo:      repo,
		assistant: assistant,
		cfg:       cfg,
		log:       log.With("module", "telegram"),
		now:       time.Now,
	}
	s.core = &chatbridge.Core{
		Grants: grants, Store: repo, Assistant: assistant, Markup: htmlMarkup{},
		ChannelName: "تيليجرام", Log: s.log, Now: func() time.Time { return s.now() },
	}
	return s
}

// Enabled reports whether a bot is configured at all.
func (s *Service) Enabled() bool { return s != nil && s.cfg.BotUsername != "" }

// LinkCode is a freshly issued one-time code.
type LinkCode struct {
	DeepLink  string
	ExpiresAt time.Time
}

// StartLink issues a one-time link code for the signed-in user.
//
// The code is 32 random bytes; only its hash is stored. It remembers the
// organisation the user was working in, which becomes the chat's starting
// منشأة. Creating a new code invalidates any unused older one.
func (s *Service) StartLink(ctx context.Context, actor authctx.Actor) (*LinkCode, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	if actor.UserID <= 0 {
		return nil, errors.New("telegram: start link requires a signed-in user")
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
		return nil, fmt.Errorf("telegram: generate link code: %w", err)
	}
	// base64url without padding: 43 characters from [A-Za-z0-9_-], which is
	// exactly the alphabet and within the 64-character limit Telegram allows
	// for a /start payload.
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
	return &LinkCode{
		DeepLink:  "https://t.me/" + s.cfg.BotUsername + "?start=" + code,
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
// the Telegram account that opened their code is theirs.
//
// This second step is what makes a leaked code harmless. Whoever opens the
// code only creates a pending link showing their Telegram name; nothing is
// answered and nothing is delivered until the account owner, signed in to
// Dawa24, accepts it.
func (s *Service) ConfirmLink(ctx context.Context, actor authctx.Actor, publicID string) (*Link, error) {
	ctx = database.AsSystem(ctx)
	link, err := s.repo.ConfirmPendingLink(ctx, actor.UserID, publicID)
	if err != nil {
		return nil, err
	}
	msg := "✅ <b>تم ربط حسابك في Dawa24 بتيليجرام.</b>\n\n" +
		"يمكنك الآن سؤال المساعد كبسولة مباشرة هنا، وستصلك إشعارات منشأتك حسب صلاحياتك.\n" +
		"اكتب /help لعرض الأوامر."
	if err := s.repo.EnqueueSystemMessage(ctx, link.ID, msg); err != nil {
		s.log.WarnContext(ctx, "telegram: enqueue welcome", "error", err, "user_id", actor.UserID)
	}
	s.log.InfoContext(ctx, "telegram link confirmed", "user_id", actor.UserID, "telegram_user_id", link.TelegramUserID)
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
	s.log.InfoContext(ctx, "telegram link revoked", "user_id", actor.UserID, "telegram_user_id", link.TelegramUserID)
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

// DownloadFile retrieves the content of an incoming Telegram file.
// It tries fileData (base64) first, then fileURL (HTTP GET), then Telegram Bot API if BotToken is configured.
func (s *Service) DownloadFile(ctx context.Context, fileID, fileURL, fileData string) ([]byte, error) {
	if fileData != "" {
		data, err := base64.StdEncoding.DecodeString(fileData)
		if err == nil && len(data) > 0 {
			return data, nil
		}
	}

	if fileURL != "" {
		return s.fetchURL(ctx, fileURL)
	}

	if fileID != "" && s.cfg.BotToken != "" {
		filePath, err := s.getTelegramFilePath(ctx, fileID)
		if err != nil {
			return nil, err
		}
		downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", s.cfg.BotToken, filePath)
		return s.fetchURL(ctx, downloadURL)
	}

	if fileID != "" && s.cfg.BotToken == "" {
		return nil, errors.New("telegram bot token is not configured on the server to download attachments")
	}

	return nil, errors.New("no file content or identifier available")
}

type getFileResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	Result      struct {
		FileID   string `json:"file_id"`
		FilePath string `json:"file_path"`
		FileSize int64  `json:"file_size"`
	} `json:"result"`
}

func (s *Service) getTelegramFilePath(ctx context.Context, fileID string) (string, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", s.cfg.BotToken, fileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("telegram getFile request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("telegram getFile status %d", resp.StatusCode)
	}

	var res getFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("decode getFile response: %w", err)
	}
	if !res.OK || res.Result.FilePath == "" {
		return "", fmt.Errorf("telegram getFile failed: %s", res.Description)
	}
	return res.Result.FilePath, nil
}

func (s *Service) fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch file url failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch file status %d", resp.StatusCode)
	}

	// 10 MB max attachment size
	const maxTelegramFileSize = 10 << 20
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxTelegramFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxTelegramFileSize {
		return nil, errors.New("file exceeds maximum size of 10MB")
	}
	return content, nil
}
