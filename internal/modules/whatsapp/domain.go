// Package whatsapp makes WhatsApp a further interface to Dawa24, with the same
// behaviour as the Telegram bridge.
//
// Like Telegram, it owns only which WhatsApp number belongs to which Dawa24
// user, the conversation a chat is in, and which notifications were handed to
// n8n. Who the user is, what they may do and who is owed a notification are
// decided by chatbridge.Core — the flow Telegram runs too — over the live RBAC
// resolver and the Capsule assistant.
//
// n8n is the transport. It forwards WhatsApp Cloud API webhooks here and posts
// the request bodies this package returns to the Cloud API unchanged, so every
// message's content, buttons and template choice is decided in Go.
package whatsapp

import (
	"errors"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
)

// LinkStatus is where a WhatsApp number is in its lifecycle.
type LinkStatus string

const (
	// LinkPending: the number sent a valid link code, and the user has not yet
	// confirmed in the browser that it is theirs.
	LinkPending LinkStatus = "pending"
	// LinkActive: confirmed; questions are answered and notifications sent.
	LinkActive LinkStatus = "active"
	// LinkBlocked: WhatsApp refused delivery to the number for good. Nothing is
	// sent until the user writes again.
	LinkBlocked LinkStatus = "blocked"
	// LinkRevoked: unlinked, replaced, rejected or expired. Terminal.
	LinkRevoked LinkStatus = "revoked"
)

// Link is one WhatsApp number bound to one Dawa24 user.
type Link struct {
	ID       int64
	PublicID string
	UserID   int64
	// WAID is the number in international form without "+", as WhatsApp
	// identifies the account.
	WAID        string
	DisplayName string
	Status      LinkStatus
	// ActiveOrgID is the منشأة the assistant works in for this chat. It is a
	// preference, not an authorisation: membership is re-read on every message.
	ActiveOrgID      *int64
	ConversationID   *int64
	MutedCategories  []string
	ConfirmExpiresAt *time.Time
	ConfirmedAt      *time.Time
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
	ErrLinkedElsewhere = errors.New("whatsapp: this WhatsApp number is linked to another user")
	ErrNoPendingLink   = errors.New("whatsapp: no pending link to confirm")
	ErrTooManyCodes    = errors.New("whatsapp: too many link codes requested")
	ErrDisabled        = errors.New("whatsapp: integration is not configured")
)

// ---------------------------------------------------------------------------
// WhatsApp Cloud API webhook shapes — only the fields this package reads.
// ---------------------------------------------------------------------------

// Webhook is a Cloud API webhook body. n8n's WhatsApp Trigger forwards one
// change value per item, so the value's fields are accepted at the top level
// as well as inside entry[].changes[].
type Webhook struct {
	Entry []WebhookEntry `json:"entry,omitempty"`
	WebhookValue
}

// WebhookEntry is one entry of a webhook.
type WebhookEntry struct {
	Changes []WebhookChange `json:"changes"`
}

// WebhookChange is one change of an entry.
type WebhookChange struct {
	Field string       `json:"field"`
	Value WebhookValue `json:"value"`
}

// WebhookValue carries the messages of one change.
type WebhookValue struct {
	Contacts []Contact        `json:"contacts,omitempty"`
	Messages []InboundMessage `json:"messages,omitempty"`
}

// Contact is the sender's profile.
type Contact struct {
	WAID    string `json:"wa_id"`
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
}

// InboundMessage is one message a user sent.
type InboundMessage struct {
	ID   string `json:"id"`
	From string `json:"from"`
	Type string `json:"type"`
	// GroupID is set on a message sent in a group.
	GroupID string `json:"group_id,omitempty"`
	Text    *struct {
		Body string `json:"body"`
	} `json:"text,omitempty"`
	Interactive *struct {
		Type        string `json:"type"`
		ButtonReply *struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"button_reply,omitempty"`
	} `json:"interactive,omitempty"`
}

// messages returns every message in the webhook with its sender's name.
func (w Webhook) messages() []namedMessage {
	values := []WebhookValue{w.WebhookValue}
	for _, e := range w.Entry {
		for _, c := range e.Changes {
			if c.Field == "messages" {
				values = append(values, c.Value)
			}
		}
	}
	var out []namedMessage
	for _, v := range values {
		for _, m := range v.Messages {
			out = append(out, namedMessage{InboundMessage: m, name: contactName(v.Contacts, m.From)})
		}
	}
	return out
}

type namedMessage struct {
	InboundMessage
	name string
}

func contactName(contacts []Contact, waID string) string {
	for _, c := range contacts {
		if c.WAID == waID {
			return c.Profile.Name
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// What n8n is told to do.
// ---------------------------------------------------------------------------

// Reply is the complete response to one webhook.
type Reply struct {
	Messages []OutMessage `json:"messages"`
}

// OutMessage is one message for n8n to send. Exactly one of Payload and
// Document is set.
type OutMessage struct {
	To string `json:"to"`
	// Payload is the complete Cloud API /messages request body.
	Payload *Payload `json:"payload,omitempty"`
	// Document asks n8n to download an export from the bridge and send it.
	Document *OutDocument `json:"document,omitempty"`
}

// OutDocument is a file for n8n to download from the bridge and send.
type OutDocument struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Caption  string `json:"caption"`
}

// Outgoing is one leased outbox row for n8n to send.
type Outgoing struct {
	ID      int64    `json:"id,string"`
	To      string   `json:"to"`
	Payload *Payload `json:"payload"`
}

// DeliveryResult is n8n's report on one Outgoing.
type DeliveryResult struct {
	ID    int64  `json:"id,string"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}
