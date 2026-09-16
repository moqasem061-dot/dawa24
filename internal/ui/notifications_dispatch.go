package ui

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/queue"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// dispatchInAppNotification sends a direct in-app notification to a single user.
// If asynchronous queue enqueueing is configured, the notification is handed to
// River for retries and durable background delivery; otherwise, it is persisted
// synchronously to notifications.logs.
func (h *UIHandler) dispatchInAppNotification(ctx context.Context, userID int64, orgID *int64, requiredPerm, title, body string) {
	h.dispatchInAppBranchNotification(ctx, userID, orgID, nil, requiredPerm, title, body)
}

// dispatchInAppBranchNotification sends a direct in-app notification to a single user with optional branch association.
func (h *UIHandler) dispatchInAppBranchNotification(ctx context.Context, userID int64, orgID, branchID *int64, requiredPerm, title, body string) {
	if h.notifSvc == nil || userID <= 0 {
		return
	}

	if h.notificationEnqueue != nil {
		err := h.notificationEnqueue(ctx, queue.NotificationDeliverArgs{
			UserID:             userID,
			OrganizationID:     orgID,
			Channel:            string(notifications.ChannelInApp),
			Recipient:          fmt.Sprintf("user-%d", userID),
			Title:              title,
			Body:               body,
			RequiredPermission: requiredPerm,
		})
		if err == nil {
			return
		}
		h.log.WarnContext(ctx, "failed to enqueue notification job; falling back to direct delivery", "user_id", userID, "error", err)
	}

	sysCtx := database.AsSystem(ctx)
	_, err := h.notifSvc.Send(sysCtx, notifications.SendInput{
		UserID:             userID,
		OrganizationID:     orgID,
		BranchID:           branchID,
		Channel:            notifications.ChannelInApp,
		Recipient:          fmt.Sprintf("user-%d", userID),
		Title:              title,
		Body:               body,
		RequiredPermission: requiredPerm,
	})
	if err != nil {
		h.log.WarnContext(ctx, "failed to dispatch in-app notification", "user_id", userID, "error", err)
	}

	// Deliver email notification asynchronously if user preferences enable email channel
	if h.mailer != nil && userID > 0 && h.idSvc != nil {
		h.safeGo("email-notification-dispatch", func() {
			bgCtx := database.AsSystem(context.Background())
			prefs, pErr := h.idSvc.GetPreferences(bgCtx, userID)
			if pErr != nil || prefs == nil || !prefs.NotificationChannels["email"] {
				return
			}
			u, uErr := h.idSvc.AdminGetUser(bgCtx, userID)
			if uErr != nil || u == nil || u.Email == "" {
				return
			}
			userName := u.Name.Get(i18n.AR)
			if userName == "" {
				userName = u.Name.Get(i18n.EN)
			}
			if sendErr := h.mailer.SendNotification(bgCtx, u.Email, userName, title, body, ""); sendErr != nil {
				h.log.Warn("failed to send notification email", "user_id", userID, "email", u.Email, "error", sendErr)
			}
		})
	}
}

// dispatchOrgNotification sends an in-app notification to authorized active members of an organization.
// Members must hold requiredPerm or be org owner/platform staff.
// Delivery couriers (org_courier) are explicitly excluded unless requiredPerm is vendor.delivery.view.
func (h *UIHandler) dispatchOrgNotification(ctx context.Context, orgID int64, requiredPerm, title, body string) {
	h.dispatchOrgBranchNotification(ctx, orgID, nil, requiredPerm, title, body)
}

// dispatchOrgBranchNotification sends an in-app notification to authorized active members of an organization
// who have access to the target branch (or all branches).
func (h *UIHandler) dispatchOrgBranchNotification(ctx context.Context, orgID int64, branchID *int64, requiredPerm, title, body string) {
	if h.notifSvc == nil || h.orgSvc == nil || orgID <= 0 {
		return
	}
	sysCtx := database.AsSystem(ctx)
	members, err := h.orgSvc.ListMembers(sysCtx, orgID)
	if err != nil || len(members) == 0 {
		return
	}
	seen := make(map[int64]bool, len(members))
	for _, m := range members {
		if m == nil || m.UserID <= 0 || !m.IsActive || seen[m.UserID] {
			continue
		}
		seen[m.UserID] = true

		// Branch scoping: If notification is targeted to a specific branch,
		// members bound to a different branch must not receive it.
		if branchID != nil && *branchID > 0 {
			if m.BranchID != nil && *m.BranchID > 0 && *m.BranchID != *branchID {
				continue
			}
		}

		// Delivery couriers must never receive vendor supply orders, quotes, or wallet alerts
		if m.RoleKey == "org_courier" && requiredPerm != "vendor.delivery.view" {
			continue
		}

		if requiredPerm != "" && h.resolver != nil {
			grant, err := h.resolver.Resolve(sysCtx, m.UserID, orgID)
			if err == nil {
				if !grant.IsOrgOwner && !grant.IsPlatformOwner && !grant.IsStaff && !grant.Can(requiredPerm) {
					continue
				}
			} else if m.RoleKey == "org_courier" {
				continue
			}
		}

		h.dispatchInAppBranchNotification(ctx, m.UserID, &orgID, branchID, requiredPerm, title, body)
	}
}

// dispatchAdminNotification sends an in-app notification to active platform staff members
// who hold the required permission (or super admin).
func (h *UIHandler) dispatchAdminNotification(ctx context.Context, requiredPerm, title, body string) {
	if h.notifSvc == nil || h.idSvc == nil {
		return
	}
	sysCtx := database.AsSystem(ctx)
	staffIDs, err := h.idSvc.ListStaffUserIDs(sysCtx)
	if err != nil || len(staffIDs) == 0 {
		return
	}
	for _, staffID := range staffIDs {
		if staffID <= 0 {
			continue
		}
		if requiredPerm != "" && h.resolver != nil {
			grant, err := h.resolver.Resolve(sysCtx, staffID, 0)
			if err == nil {
				if !grant.IsPlatformOwner && !grant.Can("*") && !grant.Can(requiredPerm) {
					continue
				}
			}
		}
		h.dispatchInAppNotification(ctx, staffID, nil, requiredPerm, title, body)
	}
}

// dispatchOrgOwnerNotification sends a notification directly to the owner of an organization.
func (h *UIHandler) dispatchOrgOwnerNotification(ctx context.Context, orgID int64, requiredPerm, title, body string) {
	if h.orgSvc == nil || orgID <= 0 {
		return
	}
	sysCtx := database.AsSystem(ctx)
	orgObj, err := h.orgSvc.GetOrganization(sysCtx, orgID)
	if err != nil || orgObj == nil || orgObj.OwnerID <= 0 {
		return
	}
	h.dispatchInAppNotification(ctx, orgObj.OwnerID, &orgID, requiredPerm, title, body)
}

// dispatchEvent dispatches a registered notification event using its registry definition or database template.
func (h *UIHandler) dispatchEvent(ctx context.Context, key notifications.EventKey, recipientUserID int64, orgID *int64, vars map[string]string) {
	evt, ok := notifications.GetEvent(key)
	if !ok {
		h.log.WarnContext(ctx, "unknown notification event key", "key", key)
		return
	}

	title, body := evt.Render("ar", vars)
	h.dispatchInAppNotification(ctx, recipientUserID, orgID, evt.RequiredPermission, title, body)
}

// dispatchOrgEvent dispatches a registered notification event to an organization's authorized members.
func (h *UIHandler) dispatchOrgEvent(ctx context.Context, key notifications.EventKey, orgID int64, vars map[string]string) {
	h.dispatchOrgBranchEvent(ctx, key, orgID, nil, vars)
}

// dispatchOrgBranchEvent dispatches a registered notification event to authorized members with branch scoping.
func (h *UIHandler) dispatchOrgBranchEvent(ctx context.Context, key notifications.EventKey, orgID int64, branchID *int64, vars map[string]string) {
	evt, ok := notifications.GetEvent(key)
	if !ok {
		h.log.WarnContext(ctx, "unknown notification event key", "key", key)
		return
	}

	title, body := evt.Render("ar", vars)
	h.dispatchOrgBranchNotification(ctx, orgID, branchID, evt.RequiredPermission, title, body)
}

// dispatchAdminEvent dispatches a registered notification event to platform administrators.
func (h *UIHandler) dispatchAdminEvent(ctx context.Context, key notifications.EventKey, vars map[string]string) {
	evt, ok := notifications.GetEvent(key)
	if !ok {
		h.log.WarnContext(ctx, "unknown notification event key", "key", key)
		return
	}

	title, body := evt.Render("ar", vars)
	h.dispatchAdminNotification(ctx, evt.RequiredPermission, title, body)
}

// resolveOrgName retrieves the localized name of an organization.
func (h *UIHandler) resolveOrgName(ctx context.Context, orgID int64) string {
	if h.orgSvc == nil || orgID <= 0 {
		return ""
	}
	sysCtx := database.AsSystem(ctx)
	orgObj, err := h.orgSvc.GetOrganization(sysCtx, orgID)
	if err != nil || orgObj == nil {
		return ""
	}
	name := orgObj.TradeName.Get(i18n.AR)
	if name == "" {
		name = orgObj.TradeName.Get(i18n.EN)
	}
	if name == "" {
		name = orgObj.LegalName
	}
	return name
}

// resolveUserName retrieves the display name of a user.
func (h *UIHandler) resolveUserName(ctx context.Context, userID int64) string {
	if h.idSvc == nil || userID <= 0 {
		return ""
	}
	sysCtx := database.AsSystem(ctx)
	u, err := h.idSvc.AdminGetUser(sysCtx, userID)
	if err != nil || u == nil {
		return ""
	}
	name := u.Name.Get(i18n.AR)
	if name == "" {
		name = u.Name.Get(i18n.EN)
	}
	if name == "" {
		name = u.Email
	}
	return name
}
