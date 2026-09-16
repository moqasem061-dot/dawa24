package postgres

import (
	"fmt"
	"strings"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// enrichAuditEntry generates human-readable localized title, description, module and severity for audit entries.
func enrichAuditEntry(e *platformadmin.AuditEntry) {
	if e == nil {
		return
	}

	if e.ActorName == "" {
		e.ActorName = i18n.TDefault("audit.actor.system_user")
	}
	actor := e.ActorName

	if e.OrganizationName == "" && e.OrganizationID != nil {
		e.OrganizationName = fmt.Sprintf(i18n.TDefault("audit.desc.org_hash"), fmt.Sprintf("%d", *e.OrganizationID))
	} else if e.OrganizationName == "" {
		e.OrganizationName = i18n.TDefault("audit.module.platform")
	}

	switch strings.ToLower(e.EntityType) {
	case "organization", "org":
		e.EntityTypeAr = i18n.TDefault("audit.entity.organization")
		e.Module = i18n.TDefault("audit.module.organizations")
	case "user", "identity.user":
		e.EntityTypeAr = i18n.TDefault("audit.entity.user")
		e.Module = i18n.TDefault("audit.module.users")
	case "role", "admin_role":
		e.EntityTypeAr = i18n.TDefault("audit.entity.role")
		e.Module = i18n.TDefault("audit.module.roles")
	case "ad", "promo.ad":
		e.EntityTypeAr = i18n.TDefault("audit.entity.ad")
		e.Module = i18n.TDefault("audit.module.ads")
	case "offer", "promo.offer", "special_offer":
		e.EntityTypeAr = i18n.TDefault("audit.entity.offer")
		e.Module = i18n.TDefault("audit.module.offers")
	case "offer_package", "promo.offer_package":
		e.EntityTypeAr = i18n.TDefault("audit.entity.offer_package")
		e.Module = i18n.TDefault("audit.module.offer_packages")
	case "sponsorship_request", "adv_product":
		e.EntityTypeAr = i18n.TDefault("audit.entity.sponsorship_request")
		e.Module = i18n.TDefault("audit.module.sponsorships")
	case "product", "catalog.product":
		e.EntityTypeAr = i18n.TDefault("audit.entity.product")
		e.Module = i18n.TDefault("audit.module.catalog")
	case "order", "commerce.order":
		e.EntityTypeAr = i18n.TDefault("audit.entity.order")
		e.Module = i18n.TDefault("audit.module.orders")
	case "invoice", "billing.invoice":
		e.EntityTypeAr = i18n.TDefault("audit.entity.invoice")
		e.Module = i18n.TDefault("audit.module.invoices")
	case "wallet", "billing.wallet":
		e.EntityTypeAr = i18n.TDefault("audit.entity.wallet")
		e.Module = i18n.TDefault("audit.module.wallets")
	case "setting", "platform.setting":
		e.EntityTypeAr = i18n.TDefault("audit.entity.setting")
		e.Module = i18n.TDefault("audit.module.settings")
	case "trash", "recycle_bin":
		e.EntityTypeAr = i18n.TDefault("audit.entity.trash")
		e.Module = i18n.TDefault("audit.module.trash")
	default:
		e.EntityTypeAr = e.EntityType
		if e.EntityTypeAr == "" {
			e.EntityTypeAr = i18n.TDefault("audit.entity.default")
		}
		e.Module = i18n.TDefault("audit.module.platform")
	}

	e.Severity = "info"

	// Check if there is an i18n key defined for this action
	if label, ok := i18n.Lookup(i18n.AR, "audit.action."+e.Action); ok && label != "" {
		e.ActionLabelAr = label
	}

	act := strings.ToLower(e.Action)
	switch {
	case strings.Contains(act, "registered") || act == "org.registered":
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = i18n.TDefault("audit.action.org_registered")
		}
		e.Title = i18n.TDefault("audit.desc.org_registered_title")
		orgName := e.OrganizationName
		if orgName == "" {
			orgName = fmt.Sprintf(i18n.TDefault("audit.desc.org_hash"), e.EntityID)
		}
		e.Description = fmt.Sprintf(i18n.TDefault("audit.desc.org_registered_body"), actor, orgName, e.EntityID)
		e.Severity = "success"

	case strings.Contains(act, "approved") || strings.Contains(act, "approve"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = i18n.TDefault("audit.action.approved")
		}
		e.Title = fmt.Sprintf(i18n.TDefault("audit.desc.approved_title"), e.EntityTypeAr)
		e.Description = fmt.Sprintf(i18n.TDefault("audit.desc.approved_body"), actor, e.EntityTypeAr, e.EntityID, e.OrganizationName)
		e.Severity = "success"

	case strings.Contains(act, "rejected") || strings.Contains(act, "reject"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = i18n.TDefault("audit.action.rejected")
		}
		e.Title = fmt.Sprintf(i18n.TDefault("audit.desc.rejected_title"), e.EntityTypeAr)
		e.Description = fmt.Sprintf(i18n.TDefault("audit.desc.rejected_body"), actor, e.EntityTypeAr, e.EntityID, e.OrganizationName)
		e.Severity = "warning"

	case strings.Contains(act, "suspend"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = i18n.TDefault("audit.action.suspend")
		}
		e.Title = fmt.Sprintf(i18n.TDefault("audit.desc.suspend_title"), e.EntityTypeAr)
		e.Description = fmt.Sprintf(i18n.TDefault("audit.desc.suspend_body"), actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "warning"

	case strings.Contains(act, "reactivate"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = i18n.TDefault("audit.action.reactivate")
		}
		e.Title = fmt.Sprintf(i18n.TDefault("audit.desc.reactivate_title"), e.EntityTypeAr)
		e.Description = fmt.Sprintf(i18n.TDefault("audit.desc.reactivate_body"), actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "success"

	case strings.Contains(act, "toggled") || strings.Contains(act, "status_changed") || strings.Contains(act, "toggle"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = i18n.TDefault("audit.action.toggle")
		}
		e.Title = fmt.Sprintf(i18n.TDefault("audit.desc.toggle_title"), e.EntityTypeAr)
		e.Description = fmt.Sprintf(i18n.TDefault("audit.desc.toggle_body"), actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "info"

	case strings.Contains(act, "trash.purge") || strings.Contains(act, "purge"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = i18n.TDefault("audit.action.purge")
		}
		e.Title = i18n.TDefault("audit.desc.purge_title")
		e.Description = fmt.Sprintf(i18n.TDefault("audit.desc.purge_body"), actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "critical"

	case strings.Contains(act, "trash.restore") || strings.Contains(act, "restore"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = i18n.TDefault("audit.action.restore")
		}
		e.Title = i18n.TDefault("audit.desc.restore_title")
		e.Description = fmt.Sprintf(i18n.TDefault("audit.desc.restore_body"), actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "info"

	case strings.Contains(act, "create") || strings.Contains(act, "insert"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = i18n.TDefault("audit.action.create")
		}
		e.Title = fmt.Sprintf(i18n.TDefault("audit.desc.create_title"), e.EntityTypeAr)
		e.Description = fmt.Sprintf(i18n.TDefault("audit.desc.create_body"), actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "success"

	case strings.Contains(act, "update") || strings.Contains(act, "edit") || strings.Contains(act, "modify"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = i18n.TDefault("audit.action.update")
		}
		e.Title = fmt.Sprintf(i18n.TDefault("audit.desc.update_title"), e.EntityTypeAr)
		e.Description = fmt.Sprintf(i18n.TDefault("audit.desc.update_body"), actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "info"

	case strings.Contains(act, "delete") || strings.Contains(act, "remove"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = i18n.TDefault("audit.action.delete")
		}
		e.Title = fmt.Sprintf(i18n.TDefault("audit.desc.delete_title"), e.EntityTypeAr)
		e.Description = fmt.Sprintf(i18n.TDefault("audit.desc.delete_body"), actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "warning"

	case strings.Contains(act, "login") || strings.Contains(act, "auth.sign_in"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = i18n.TDefault("audit.action.login")
		}
		e.Title = i18n.TDefault("audit.desc.login_title")
		e.Description = fmt.Sprintf(i18n.TDefault("audit.desc.login_body"), actor, e.IPAddress)
		e.Severity = "info"

	case strings.Contains(act, "logout") || strings.Contains(act, "auth.sign_out"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = i18n.TDefault("audit.action.logout")
		}
		e.Title = i18n.TDefault("audit.desc.logout_title")
		e.Description = fmt.Sprintf(i18n.TDefault("audit.desc.logout_body"), actor)
		e.Severity = "info"

	default:
		if e.ActionLabelAr == "" {
			parts := strings.Split(e.Action, ".")
			actionName := e.Action
			if len(parts) > 1 {
				actionName = parts[len(parts)-1]
			}
			e.ActionLabelAr = actionName
		}
		e.Title = fmt.Sprintf(i18n.TDefault("audit.desc.default_title"), e.Action, e.EntityTypeAr)
		e.Description = fmt.Sprintf(i18n.TDefault("audit.desc.default_body"), actor, e.Action, e.EntityTypeAr, e.EntityID)
	}
}
