package chatbridge

import (
	"context"
	"log/slog"
	"regexp"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// BusyFor bounds the one-question-at-a-time lock. Longer than the assistant's
// own turn deadline, so a finished turn always releases it first; short enough
// that a process that died mid-turn does not silence the chat for long.
const BusyFor = 3 * time.Minute

// Core runs a chat's questions and confirmations through the platform's gates.
type Core struct {
	Grants    GrantResolver
	Store     Store
	Assistant Assistant
	Markup    Markup
	// ChannelName is the channel as users read it, e.g. "تيليجرام".
	ChannelName string
	Log         *slog.Logger
	Now         func() time.Time
}

// Resolve rebuilds the chat's user for one message, from the database, now.
//
// Nothing about who the user is or what they may do is kept on the link. The
// link names a user and a preferred منشأة; the platform resolver answers the
// rest exactly as it does for a browser request, so a role revoked a second
// ago is revoked here too.
//
// The returned refusal is a user-facing sentence when the caller cannot act at
// all — a suspended account, a membership that has ended, no منشأة chosen. A
// sole membership is chosen and saved on the chat.
func (c *Core) Resolve(ctx context.Context, chat *Chat) (authctx.Actor, string, error) {
	sys := database.AsSystem(ctx)
	orgID := chat.OrgID()

	var memberships []Membership
	if orgID == 0 {
		var err error
		if memberships, err = c.Store.Memberships(sys, chat.UserID); err != nil {
			return authctx.Actor{}, "", err
		}
		if len(memberships) == 1 {
			orgID = memberships[0].OrgID
			id := orgID
			if err := c.Store.SetActiveOrganization(sys, chat.LinkID, &id); err != nil {
				return authctx.Actor{}, "", err
			}
			chat.ActiveOrgID, chat.ConversationID = &id, nil
		}
	}

	grant, err := c.Grants.Resolve(sys, chat.UserID, orgID)
	if err != nil {
		return authctx.Actor{}, "", err
	}
	if !grant.Active {
		return authctx.Actor{}, "🚫 حسابك في Dawa24 غير نشط حالياً، لذلك لا يمكن تنفيذ طلبك.", nil
	}
	platformSide := grant.IsStaff || grant.IsPlatformOwner
	switch {
	case orgID > 0 && grant.Scope == "":
		return authctx.Actor{}, "لم تعد لديك عضوية نشطة في المنشأة المحددة. اختر منشأة أخرى عبر /org", nil
	case orgID == 0 && !platformSide && len(memberships) > 1:
		return authctx.Actor{}, "لديك أكثر من منشأة. اختر المنشأة التي تريد العمل ضمنها عبر /org", nil
	case orgID == 0 && !platformSide:
		return authctx.Actor{}, "لا توجد منشأة نشطة مرتبطة بحسابك في Dawa24.", nil
	}
	return authctx.FromGrant(grant), "", nil
}

// Authorize is Resolve plus the approved-منشأة rule every assistant route
// applies.
func (c *Core) Authorize(ctx context.Context, chat *Chat) (authctx.Actor, string, error) {
	actor, refusal, err := c.Resolve(ctx, chat)
	if err == nil && refusal == "" {
		refusal = ApprovalRefusal(actor)
	}
	return actor, refusal, err
}

// ApprovalRefusal applies the same rule as authctx.RequireApproved on the
// browser's assistant routes: platform staff pass, everyone else needs an
// approved منشأة.
func ApprovalRefusal(actor authctx.Actor) string {
	if actor.IsOrgApproved() {
		return ""
	}
	switch actor.OrgStatus {
	case "pending", "under_review":
		return "⏳ منشأتك قيد المراجعة من إدارة المنصة. سيتاح المساعد بعد اعتمادها."
	case "rejected":
		return "🚫 تم رفض طلب اعتماد المنشأة، لذلك لا يمكن استخدام المساعد."
	case "suspended":
		return "🚫 المنشأة موقوفة حالياً، لذلك لا يمكن استخدام المساعد."
	}
	return "🚫 يلزم اعتماد المنشأة لاستخدام المساعد."
}

// Ask runs one question through every gate and then the assistant. A
// non-empty refusal is the user-facing reason the assistant was not asked.
func (c *Core) Ask(ctx context.Context, chat *Chat, question string, attachmentRefs ...string) (Answer, string, error) {
	sys := database.AsSystem(ctx)
	actor, refusal, err := c.Authorize(ctx, chat)
	if err != nil || refusal != "" {
		return Answer{}, refusal, err
	}
	if !c.Assistant.Allowed(actor) {
		return Answer{}, "🔒 استخدام المساعد كبسولة غير مفعّل لدورك في هذه المنشأة. يمكن لمالك المنشأة تفعيله من صفحة الأدوار والصلاحيات.", nil
	}
	if !c.Assistant.AllowQuestion(actor.UserID) {
		return Answer{}, "أرسلت أسئلة كثيرة خلال دقيقة. انتظر قليلاً ثم أعد المحاولة.", nil
	}

	acquired, err := c.Store.AcquireBusy(sys, chat.LinkID, c.Now().Add(BusyFor))
	if err != nil {
		return Answer{}, "", err
	}
	if !acquired {
		return Answer{}, "⏳ ما زلت أجيب عن سؤالك السابق. سأرد عليه أولاً، ثم أرسل سؤالك الجديد.", nil
	}
	defer func() {
		if err := c.Store.ReleaseBusy(context.WithoutCancel(sys), chat.LinkID); err != nil {
			c.Log.WarnContext(ctx, "chatbridge: release busy", "error", err)
		}
	}()

	var convID int64
	if chat.ConversationID != nil {
		convID = *chat.ConversationID
	}
	ans := c.Assistant.Ask(turnContext(ctx, actor), actor, convID, question, attachmentRefs...)
	if ans.ConversationID > 0 && ans.ConversationID != convID {
		id := ans.ConversationID
		if err := c.Store.SetConversation(context.WithoutCancel(sys), chat.LinkID, &id); err != nil {
			c.Log.WarnContext(ctx, "chatbridge: save conversation", "error", err)
		}
		chat.ConversationID = &id
	}
	return ans, "", nil
}

// Decide confirms or cancels a proposal for the chat's live user, through the
// assistant's own confirm path with its full re-check.
func (c *Core) Decide(ctx context.Context, chat *Chat, confirm bool, proposalID string) (ActionReply, string, error) {
	actor, refusal, err := c.Authorize(ctx, chat)
	if err != nil || refusal != "" {
		return ActionReply{}, refusal, err
	}
	return c.Assistant.Decide(turnContext(ctx, actor), actor, confirm, proposalID), "", nil
}

// turnContext is the context the assistant runs under: the caller's tenant
// and the caller's actor — never the system context a bridge uses for its own
// tables. It is detached from the transport's request so a dropped connection
// still finishes and persists the answer, where the user can read it in the
// dashboard.
func turnContext(ctx context.Context, actor authctx.Actor) context.Context {
	out := context.WithoutCancel(ctx)
	if actor.OrgID > 0 {
		out = database.WithTenant(out, actor.OrgID)
	}
	return authctx.WithActor(out, actor)
}

// Decision payloads name only a proposal's public id and the choice —
// "act:c:<uuid>" or "act:x:<uuid>" — well under every channel's button limit,
// and nothing in them is secret or sufficient on its own.
var decisionPayload = regexp.MustCompile(`^act:([cx]):([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

// DecisionPayload is the button payload for a choice on a proposal.
func DecisionPayload(confirm bool, proposalID string) string {
	if confirm {
		return "act:c:" + proposalID
	}
	return "act:x:" + proposalID
}

// ParseDecisionPayload reads a button payload; ok is false for anything else.
func ParseDecisionPayload(payload string) (confirm bool, proposalID string, ok bool) {
	m := decisionPayload.FindStringSubmatch(payload)
	if m == nil {
		return false, "", false
	}
	return m[1] == "c", m[2], true
}
