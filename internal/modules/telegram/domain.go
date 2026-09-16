// Package telegram makes Telegram a second interface to Dawa24.
//
// It owns exactly three things: which Telegram account belongs to which Dawa24
// user, the conversation a chat is currently in, and which notifications have
// already been handed to n8n for delivery. It owns no permissions, no roles and
// no business data. Every message is answered for an actor rebuilt from the
// live RBAC resolver — the same resolver the browser's session goes through —
// and every question is answered by the Capsule assistant itself, through a
// port, with its own gates, tools and audit trail.
//
// n8n is the transport. It forwards raw Telegram updates here, sends what this
// package tells it to send, and drains the notification outbox. It is never
// asked who a user is or what they may see.
package telegram

import (
	"errors"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
)

// LinkStatus is where a Telegram account is in its lifecycle.
type LinkStatus string

const (
	// LinkPending: the user opened the bot with a valid link code, and has not
	// yet confirmed in the browser that this Telegram account is theirs.
	LinkPending LinkStatus = "pending"
	// LinkActive: confirmed; the bot answers and notifications are delivered.
	LinkActive LinkStatus = "active"
	// LinkBlocked: the user blocked the bot. Nothing is sent until they write
	// to it again.
	LinkBlocked LinkStatus = "blocked"
	// LinkRevoked: unlinked, replaced, rejected or expired. Terminal.
	LinkRevoked LinkStatus = "revoked"
)

// Link is one Telegram account bound to one Dawa24 user.
type Link struct {
	ID             int64
	PublicID       string
	UserID         int64
	TelegramUserID int64
	ChatID         int64
	Username       string
	DisplayName    string
	Status         LinkStatus
	// ActiveOrgID is the منشأة the assistant works in for this chat. It is a
	// preference, not an authorisation: membership is re-read on every message.
	ActiveOrgID      *int64
	ConversationID   *int64
	MutedCategories  []string
	ConfirmExpiresAt *time.Time
	ConfirmedAt      *time.Time
}

// OrgID returns the active organisation, zero for none.
func (l *Link) OrgID() int64 {
	if l == nil || l.ActiveOrgID == nil {
		return 0
	}
	return *l.ActiveOrgID
}

// Muted reports whether the user switched a notification category off.
func (l *Link) Muted(category chatbridge.Category) bool {
	return chatbridge.Muted(l.MutedCategories, category)
}

// chat is the link as the shared flow sees it; adopt takes back what the flow
// changed.
func (l *Link) chat() *chatbridge.Chat {
	return &chatbridge.Chat{
		LinkID: l.ID, UserID: l.UserID, ActiveOrgID: l.ActiveOrgID,
		ConversationID: l.ConversationID, MutedCategories: l.MutedCategories,
	}
}

func (l *Link) adopt(c *chatbridge.Chat) {
	l.ActiveOrgID, l.ConversationID, l.MutedCategories = c.ActiveOrgID, c.ConversationID, c.MutedCategories
}

// Errors the repository reports as outcomes rather than failures.
var (
	ErrTokenInvalid    = chatbridge.ErrTokenInvalid
	ErrLinkedElsewhere = errors.New("telegram: this Telegram account is linked to another user")
	ErrNoPendingLink   = errors.New("telegram: no pending link to confirm")
	ErrTooManyCodes    = errors.New("telegram: too many link codes requested")
	ErrDisabled        = errors.New("telegram: integration is not configured")
)

// ---------------------------------------------------------------------------
// Telegram Bot API shapes — only the fields this package reads.
// ---------------------------------------------------------------------------

// Update is one incoming Telegram update, exactly as Telegram sent it.
type Update struct {
	UpdateID      int64              `json:"update_id"`
	Message       *Message           `json:"message,omitempty"`
	MyChatMember  *ChatMemberUpdated `json:"my_chat_member,omitempty"`
	CallbackQuery *CallbackQuery     `json:"callback_query,omitempty"`
}

// Message is a Telegram message.
type Message struct {
	MessageID int64       `json:"message_id"`
	From      *User       `json:"from,omitempty"`
	Chat      Chat        `json:"chat"`
	Text      string      `json:"text,omitempty"`
	Caption   string      `json:"caption,omitempty"`
	Photo     []PhotoSize `json:"photo,omitempty"`
	Document  *Document   `json:"document,omitempty"`
}

// PhotoSize is one size of a photo or a file thumbnail.
type PhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id,omitempty"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int64  `json:"file_size,omitempty"`
	FileURL      string `json:"file_url,omitempty"`
	FileData     string `json:"file_data,omitempty"`
}

// Document is a general file sent via Telegram.
type Document struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id,omitempty"`
	FileName     string `json:"file_name,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
	FileURL      string `json:"file_url,omitempty"`
	FileData     string `json:"file_data,omitempty"`
}

// User is a Telegram user.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// Chat is a Telegram chat.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// ChatMemberUpdated reports the bot being blocked or unblocked in a chat.
type ChatMemberUpdated struct {
	Chat          Chat       `json:"chat"`
	From          User       `json:"from"`
	NewChatMember ChatMember `json:"new_chat_member"`
}

// ChatMember carries the bot's new status in the chat.
type ChatMember struct {
	Status string `json:"status"`
}

// ---------------------------------------------------------------------------
// What n8n is told to do.
// ---------------------------------------------------------------------------

// OutMessage is one message n8n sends with parse_mode HTML.
//
// chat_id travels as a string: Telegram ids fit in 52 bits today, but a
// JavaScript number silently loses precision above 2^53 and n8n is JavaScript.
type OutMessage struct {
	ChatID int64  `json:"chat_id,string"`
	Text   string `json:"text"`
	// ReplyMarkup carries confirmation buttons.
	ReplyMarkup *InlineKeyboard `json:"reply_markup,omitempty"`
	// Document, when set, is sent as a file with Text as its caption.
	Document *OutDocument `json:"document,omitempty"`
}

// Reply is the complete response to one update.
type Reply struct {
	Messages []OutMessage `json:"messages"`
	// AnswerCallback acknowledges a pressed button.
	AnswerCallback *CallbackAnswer `json:"answer_callback,omitempty"`
}

func replyTo(chatID int64, texts ...string) Reply {
	r := Reply{Messages: make([]OutMessage, 0, len(texts))}
	for _, t := range texts {
		if t != "" {
			r.Messages = append(r.Messages, OutMessage{ChatID: chatID, Text: t})
		}
	}
	return r
}

// Outgoing is one leased outbox row for n8n to send.
type Outgoing struct {
	ID     int64  `json:"id,string"`
	ChatID int64  `json:"chat_id,string"`
	Text   string `json:"text"`
}

// DeliveryResult is n8n's report on one Outgoing.
type DeliveryResult struct {
	ID    int64  `json:"id,string"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}
