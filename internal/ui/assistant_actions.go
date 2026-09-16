package ui

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/components"
)

// Dashboard actions offered to Capsule.
//
// Every command here is the dashboard's own code path: the same helpers the
// HTTP handler for that screen calls, with the HTTP rendering left in the
// handler. A command also re-applies what the route chain in front of that
// handler enforces — the dashboard audience, organisation approval and the
// route's permission — because a confirmation arrives on an assistant route,
// not on the screen's.
//
// Prepare renders the confirmation card from current data and changes
// nothing. Execute re-validates and commits. Both run for the live actor.

type commandAudience int

const (
	// audienceBuyer is RequireBuyer + RequireApproved + RequireCapability.
	audienceBuyer commandAudience = iota + 1
	// audienceVendor is RequireVendor + RequireApproved + RequireTenantPagePermission.
	audienceVendor
	// audienceStaff is RequireStaff + RequirePagePermission.
	audienceStaff
)

type assistantCommand struct {
	def      actions.Definition
	audience commandAudience
	// caps are the buying capabilities of the route (audienceBuyer).
	caps []rbac.Capability
	// keys are the route's permission keys (vendor and staff audiences).
	keys    []string
	prepare func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error)
	execute func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error)
}

// AssistantActions implements actions.Executor over the dashboard.
type AssistantActions struct {
	h        *UIHandler
	commands map[string]assistantCommand
}

var _ actions.Executor = (*AssistantActions)(nil)

// AssistantActions returns the dashboard actions Capsule may propose.
func (h *UIHandler) AssistantActions() *AssistantActions {
	a := &AssistantActions{h: h, commands: map[string]assistantCommand{}}
	for _, group := range [][]assistantCommand{h.buyingCommands(), h.vendorCommands(), h.staffCommands()} {
		for _, c := range group {
			if _, dup := a.commands[c.def.Name]; dup {
				panic("assistant actions: duplicate command " + c.def.Name)
			}
			a.commands[c.def.Name] = c
		}
	}
	return a
}

// Definitions lists the actions of the caller's dashboard.
func (a *AssistantActions) Definitions(actor authctx.Actor) []actions.Definition {
	var out []actions.Definition
	for _, c := range a.commands {
		if audienceOf(actor) == c.audience || (c.audience == audienceBuyer && actor.IsBuyer()) {
			out = append(out, c.def)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Permitted applies the route chain of the screen the command comes from.
func (a *AssistantActions) Permitted(actor authctx.Actor, name string) bool {
	c, ok := a.commands[name]
	if !ok || actor.UserID <= 0 {
		return false
	}
	switch c.audience {
	case audienceBuyer:
		keys := rbac.RequiredKeys(actor.DashboardScope(), c.caps...)
		return actor.IsBuyer() && orgApproved(actor) && len(keys) > 0 && actor.CanAny(keys...)
	case audienceVendor:
		return !actor.IsStaff && actor.IsVendor() && actor.OrganizationID > 0 && orgApproved(actor) && actor.CanAny(c.keys...)
	case audienceStaff:
		return actor.IsStaff && actor.CanAny(c.keys...)
	}
	return false
}

// Prepare validates a command and renders its preview.
func (a *AssistantActions) Prepare(ctx context.Context, actor authctx.Actor, name string, args actions.Args) (actions.Preview, error) {
	c, ok := a.commands[name]
	if !ok || !a.Permitted(actor, name) {
		return actions.Preview{}, actions.ErrNotAllowed
	}
	return c.prepare(commandContext(ctx, actor), actor, args)
}

// Execute performs a command.
func (a *AssistantActions) Execute(ctx context.Context, actor authctx.Actor, name string, args actions.Args) (actions.Outcome, error) {
	c, ok := a.commands[name]
	if !ok || !a.Permitted(actor, name) {
		return actions.Outcome{}, actions.ErrNotAllowed
	}
	return c.execute(commandContext(ctx, actor), actor, args)
}

func audienceOf(actor authctx.Actor) commandAudience {
	switch {
	case actor.IsStaff:
		return audienceStaff
	case actor.IsVendor():
		return audienceVendor
	case actor.IsBuyer():
		return audienceBuyer
	}
	return 0
}

// orgApproved is RequireApproved.
func orgApproved(actor authctx.Actor) bool {
	switch actor.OrgStatus {
	case "approved", "active", "verified":
		return true
	}
	return false
}

// commandContext is what the session middleware would have put on a dashboard
// request: the actor and the tenant.
func commandContext(ctx context.Context, actor authctx.Actor) context.Context {
	if actor.OrganizationID > 0 {
		ctx = database.WithTenant(ctx, actor.OrganizationID)
	}
	return authctx.WithActor(ctx, actor)
}

// buyingContext is what BuyingBranchSelector would have put on a buying
// request, for an explicit branch or the caller's default one. A branch that
// is not one of the caller's own buying branches is refused.
func (h *UIHandler) buyingContext(ctx context.Context, actor authctx.Actor, branchID int64) (context.Context, authctx.Actor, string, error) {
	if h.orgSvc == nil {
		return ctx, actor, "", actions.Refuse("خدمة الفروع غير متاحة حالياً.")
	}
	options := h.loadCustomerBranchOptions(ctx, actor, "ar")
	if len(options) == 0 {
		return ctx, actor, "", actions.Refuse("لا يوجد فرع مفعّل للشراء له. أضف فرعاً أو فعّله أولاً.")
	}
	var active *authctx.BranchOption
	locked := !actor.IsOwner && actor.BranchID != nil && *actor.BranchID > 0 && len(options) == 1 && options[0].ID == *actor.BranchID
	for i := range options {
		switch {
		case branchID > 0 && options[i].ID == branchID:
			active = &options[i]
		case branchID <= 0 && actor.BranchID != nil && options[i].ID == *actor.BranchID:
			active = &options[i]
		}
	}
	if branchID > 0 && active == nil {
		return ctx, actor, "", actions.Refuse("هذا الفرع ليس من فروع الشراء المتاحة لك.")
	}
	if active == nil {
		active = &options[0]
	}
	id := active.ID
	actor.BranchID = &id
	ctx = authctx.WithBuyingBranch(ctx, authctx.BuyingBranch{Branches: options, Active: &id, IsLocked: locked})
	return authctx.WithActor(ctx, actor), actor, active.Name, nil
}

// egp formats an amount for a confirmation card.
func egp(a money.Amount) string {
	s := a.String()
	whole, frac, _ := strings.Cut(s, ".")
	neg := strings.HasPrefix(whole, "-")
	whole = strings.TrimPrefix(whole, "-")
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := b.String()
	if frac != "" && frac != "00" {
		out += "." + frac
	}
	if neg {
		out = "-" + out
	}
	return out + " ج.م"
}

func refuseMessage(msg string) error {
	if strings.TrimSpace(msg) == "" {
		msg = "تعذّر تنفيذ هذا الإجراء."
	}
	return actions.Refuse("%s", msg)
}

func countLabel(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// CapsuleAssistantPanel renders the lazy-loaded Capsule AI assistant drawer over HTMX.
func (h *UIHandler) CapsuleAssistantPanel(w http.ResponseWriter, r *http.Request) {
	_ = components.CapsuleAssistantPanel().Render(r.Context(), w)
}
