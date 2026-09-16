package assistant

import (
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Which assistant a caller gets, and why it is not a setting.
//
// There are three agents because there are three dashboards, and a supplier
// asking "how are my sales" means something entirely different from a pharmacy
// asking the same words. Each agent has its own instructions, its own
// vocabulary and its own tools.
//
// Selection is a pure function of the authenticated actor. No request field
// names an agent, and there is no way to ask for a different one: the
// assistant is chosen by the same DashboardScope() call that decides which
// sidebar the user sees, so the agent and the screens can never disagree about
// who somebody is.
//
// This is a routing decision, not a security boundary. Even a caller who
// somehow reached the wrong agent would gain nothing: every tool re-checks the
// live actor's scope and permissions before it touches a query.

// AgentConfig is one assistant persona.
type AgentConfig struct {
	// Role is the dashboard scope this agent serves, stored on the
	// conversation so a user whose role changed cannot resume an old thread.
	Role rbac.Scope
	// Gate is the permission that must be held to use the assistant at all.
	// A pharmacy owner grants it per employee role; without it there is no
	// assistant, whatever else the employee can open.
	Gate string
	// SystemPrompt is the persona and the rules.
	SystemPrompt string
	// Label is what the drawer shows as the assistant's remit.
	Label string
}

// Gate permission keys. They are declared in the RBAC catalogue
// (internal/platform/rbac/catalog_*.go) and therefore appear in the role editor
// of the dashboard they belong to.
const (
	GatePharmacy = "pharmacy.assistant.use"
	GateVendor   = "vendor.assistant.use"
	GateAdmin    = "platform.assistant.use"
)

var (
	pharmacyAgent = AgentConfig{
		Role:         rbac.ScopePharmacy,
		Gate:         GatePharmacy,
		SystemPrompt: pharmacyPrompt,
		Label:        "مساعد الصيدلية",
	}
	vendorAgent = AgentConfig{
		Role:         rbac.ScopeVendor,
		Gate:         GateVendor,
		SystemPrompt: vendorPrompt,
		Label:        "مساعد المورّد",
	}
	adminAgent = AgentConfig{
		Role:         rbac.ScopeAdmin,
		Gate:         GateAdmin,
		SystemPrompt: adminPrompt,
		Label:        "مساعد إدارة المنصة",
	}
)

// Act gate keys: the owner's separate switch for letting Capsule prepare
// actions. Declared in the RBAC catalogue beside the gates above.
const (
	ActPharmacy = "pharmacy.assistant.act"
	ActVendor   = "vendor.assistant.act"
	ActAdmin    = "platform.assistant.act"
)

// CanAct reports whether this caller may have Capsule prepare actions: they
// may use the assistant, and hold their dashboard's act grant. Each action then
// requires its own screen's permission on top.
func CanAct(actor authctx.Actor) bool {
	cfg, ok := Allowed(actor)
	if !ok {
		return false
	}
	switch cfg.Role {
	case rbac.ScopePharmacy:
		return actor.Can(ActPharmacy)
	case rbac.ScopeVendor:
		return actor.Can(ActVendor)
	case rbac.ScopeAdmin:
		return actor.Can(ActAdmin)
	}
	return false
}

// AgentFor returns the assistant this caller gets, and false when they get
// none — a user with no dashboard has no agent, rather than falling through to
// a default that would be somebody else's.
func AgentFor(actor authctx.Actor) (AgentConfig, bool) {
	switch actor.DashboardScope() {
	case rbac.ScopeAdmin:
		return adminAgent, true
	case rbac.ScopeVendor:
		return vendorAgent, true
	case rbac.ScopePharmacy:
		return pharmacyAgent, true
	}
	return AgentConfig{}, false
}

// Allowed reports whether this caller may use the assistant at all.
//
// Two conditions, both server-side: they must have a dashboard, and they must
// hold that dashboard's assistant grant. Company owners hold their whole scope
// and therefore always pass; an employee passes only where the owner said so.
func Allowed(actor authctx.Actor) (AgentConfig, bool) {
	cfg, ok := AgentFor(actor)
	if !ok {
		return AgentConfig{}, false
	}
	if !actor.Can(cfg.Gate) {
		return AgentConfig{}, false
	}
	return cfg, true
}
