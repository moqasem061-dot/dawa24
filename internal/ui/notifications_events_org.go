package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// notifyProfileChangeRequested alerts platform admins that an organization requested profile edits.
func (h *UIHandler) notifyProfileChangeRequested(ctx context.Context, orgID int64, section string) {
	orgName := h.resolveOrgName(ctx, orgID)
	if orgName == "" {
		orgName = "منشأة"
	}
	vars := map[string]string{
		"org_name": orgName,
		"section":  section,
	}
	h.dispatchAdminEvent(ctx, notifications.EventOrgProfileChangeRequested, vars)
}

// notifyProfileChangeDecision alerts the requesting organization of admin approval/rejection.
func (h *UIHandler) notifyProfileChangeDecision(ctx context.Context, orgID int64, section string, approved bool, reason string) {
	if orgID <= 0 {
		return
	}
	vars := map[string]string{
		"section": section,
		"reason":  reason,
	}
	key := notifications.EventOrgProfileChangeApproved
	if !approved {
		key = notifications.EventOrgProfileChangeRejected
	}
	h.dispatchOrgEvent(ctx, key, orgID, vars)
}

// notifyOrgDeletionRequested alerts platform admins when an owner requests organization deletion.
func (h *UIHandler) notifyOrgDeletionRequested(ctx context.Context, orgID int64, reason string) {
	orgName := h.resolveOrgName(ctx, orgID)
	if orgName == "" {
		orgName = "منشأة"
	}
	vars := map[string]string{
		"org_name": orgName,
		"reason":   reason,
	}
	h.dispatchAdminEvent(ctx, notifications.EventOrgDeletionRequested, vars)
}

// notifyOrgDeletionDecision alerts the organization owner of the deletion request outcome.
func (h *UIHandler) notifyOrgDeletionDecision(ctx context.Context, orgID int64, approved bool, reason string) {
	if orgID <= 0 {
		return
	}
	orgName := h.resolveOrgName(ctx, orgID)
	vars := map[string]string{
		"org_name": orgName,
		"reason":   reason,
	}
	key := notifications.EventOrgDeletionApproved
	if !approved {
		key = notifications.EventOrgDeletionRejected
	}
	evt, ok := notifications.GetEvent(key)
	if !ok {
		return
	}
	title, body := evt.Render("ar", vars)
	h.dispatchOrgOwnerNotification(ctx, orgID, evt.RequiredPermission, title, body)
}

// notifyAccountDeletionRequested alerts admins when a user submits an account deletion request.
func (h *UIHandler) notifyAccountDeletionRequested(ctx context.Context, userID int64, reason string) {
	userName := h.resolveUserName(ctx, userID)
	vars := map[string]string{
		"user_name":  userName,
		"user_email": fmt.Sprintf("user-%d", userID),
		"reason":     reason,
	}
	h.dispatchAdminEvent(ctx, notifications.EventAccountDeletionRequested, vars)
}

// notifyAccountDeletionDecision alerts the user of their account deletion outcome.
func (h *UIHandler) notifyAccountDeletionDecision(ctx context.Context, userID int64, approved bool, reason string) {
	if userID <= 0 {
		return
	}
	vars := map[string]string{
		"reason": reason,
	}
	key := notifications.EventAccountDeletionApproved
	if !approved {
		key = notifications.EventAccountDeletionRejected
	}
	h.dispatchEvent(ctx, key, userID, nil, vars)
}

// notifyOrgApproved dispatches celebration notification when admin approves an organization.
func (h *UIHandler) notifyOrgApproved(ctx context.Context, orgID int64) {
	if orgID <= 0 {
		return
	}
	h.dispatchOrgEvent(ctx, notifications.EventOrgApproved, orgID, nil)
}

// notifyOrgRejected dispatches notification when admin rejects an organization.
func (h *UIHandler) notifyOrgRejected(ctx context.Context, orgID int64, reason string) {
	if orgID <= 0 {
		return
	}
	reasonStr := ""
	if strings.TrimSpace(reason) != "" {
		reasonStr = fmt.Sprintf("السبب: %s", reason)
	}
	vars := map[string]string{"reason": reasonStr}
	h.dispatchOrgEvent(ctx, notifications.EventOrgRejected, orgID, vars)
}

// notifyOrgSuspended alerts the organization when it is placed in suspended status.
func (h *UIHandler) notifyOrgSuspended(ctx context.Context, orgID int64, reason string) {
	if orgID <= 0 {
		return
	}
	reasonStr := ""
	if strings.TrimSpace(reason) != "" {
		reasonStr = fmt.Sprintf("السبب: %s", reason)
	}
	vars := map[string]string{"reason": reasonStr}
	h.dispatchOrgEvent(ctx, notifications.EventOrgSuspended, orgID, vars)
}

// notifyOrgReactivated alerts the organization when its suspension is lifted.
func (h *UIHandler) notifyOrgReactivated(ctx context.Context, orgID int64) {
	if orgID <= 0 {
		return
	}
	h.dispatchOrgEvent(ctx, notifications.EventOrgReactivated, orgID, nil)
}

// notifyDocumentRequested alerts an organization that a formal document is required by admin.
func (h *UIHandler) notifyDocumentRequested(ctx context.Context, orgID int64, docName, description string, deadlineDays int) {
	if orgID <= 0 {
		return
	}
	if deadlineDays <= 0 {
		deadlineDays = 30
	}
	vars := map[string]string{
		"document_name": docName,
		"description":   description,
		"deadline_days": fmt.Sprintf("%d", deadlineDays),
	}
	h.dispatchOrgEvent(ctx, notifications.EventDocumentRequested, orgID, vars)
}

// notifyDocumentVerified alerts an organization when an uploaded document is verified or rejected.
func (h *UIHandler) notifyDocumentVerified(ctx context.Context, orgID int64, docName string, verified bool, notes string) {
	if orgID <= 0 {
		return
	}
	if docName == "" {
		docName = "مستند رسمي"
	}
	if verified {
		vars := map[string]string{"document_name": docName}
		h.dispatchOrgEvent(ctx, notifications.EventDocumentVerified, orgID, vars)
	} else {
		h.notifyDocumentRejected(ctx, orgID, docName, notes)
	}
}

// notifyDocumentRejected alerts an organization when an uploaded document is rejected with reason.
func (h *UIHandler) notifyDocumentRejected(ctx context.Context, orgID int64, docName, reason string) {
	if orgID <= 0 {
		return
	}
	if docName == "" {
		docName = "مستند رسمي"
	}
	vars := map[string]string{
		"document_name": docName,
		"reason":        reason,
	}
	h.dispatchOrgEvent(ctx, notifications.EventDocumentRejected, orgID, vars)
}

// notifyBranchCreated alerts the organization owner when a new branch is created.
func (h *UIHandler) notifyBranchCreated(ctx context.Context, orgID int64, branchName, branchCode string) {
	if orgID <= 0 {
		return
	}
	vars := map[string]string{
		"branch_name": branchName,
		"branch_code": branchCode,
	}
	evt, _ := notifications.GetEvent(notifications.EventBranchCreated)
	title, body := evt.Render("ar", vars)
	h.dispatchOrgOwnerNotification(ctx, orgID, evt.RequiredPermission, title, body)
}

// notifyBranchDisabled alerts the organization owner when a branch is disabled.
func (h *UIHandler) notifyBranchDisabled(ctx context.Context, orgID int64, branchName string) {
	if orgID <= 0 {
		return
	}
	vars := map[string]string{
		"branch_name": branchName,
	}
	evt, _ := notifications.GetEvent(notifications.EventBranchDisabled)
	title, body := evt.Render("ar", vars)
	h.dispatchOrgOwnerNotification(ctx, orgID, evt.RequiredPermission, title, body)
}

// notifyBranchInstitutionalWorksChanged alerts the organization owner when institutional works are updated.
func (h *UIHandler) notifyBranchInstitutionalWorksChanged(ctx context.Context, orgID int64, branchName string) {
	if orgID <= 0 {
		return
	}
	vars := map[string]string{
		"branch_name": branchName,
	}
	evt, _ := notifications.GetEvent(notifications.EventBranchInstitutionalWorksChanged)
	title, body := evt.Render("ar", vars)
	h.dispatchOrgOwnerNotification(ctx, orgID, evt.RequiredPermission, title, body)
}

// notifyUserAddedToOrg alerts both the user and the organization owner when a member is added.
func (h *UIHandler) notifyUserAddedToOrg(ctx context.Context, orgID int64, userID int64, roleName string) {
	orgName := h.resolveOrgName(ctx, orgID)
	userName := h.resolveUserName(ctx, userID)
	vars := map[string]string{
		"user_name": userName,
		"org_name":  orgName,
		"role_name": roleName,
	}
	evt, _ := notifications.GetEvent(notifications.EventUserAddedToOrg)
	title, body := evt.Render("ar", vars)
	if userID > 0 {
		h.dispatchInAppNotification(ctx, userID, &orgID, "", title, body)
	}
	if orgID > 0 {
		h.dispatchOrgOwnerNotification(ctx, orgID, "", title, body)
	}
}

// notifyUserRemovedFromOrg alerts both the user and the organization owner when a member is removed.
func (h *UIHandler) notifyUserRemovedFromOrg(ctx context.Context, orgID int64, userID int64) {
	orgName := h.resolveOrgName(ctx, orgID)
	userName := h.resolveUserName(ctx, userID)
	vars := map[string]string{
		"user_name": userName,
		"org_name":  orgName,
	}
	evt, _ := notifications.GetEvent(notifications.EventUserRemovedFromOrg)
	title, body := evt.Render("ar", vars)
	if userID > 0 {
		h.dispatchInAppNotification(ctx, userID, &orgID, "", title, body)
	}
	if orgID > 0 {
		h.dispatchOrgOwnerNotification(ctx, orgID, "", title, body)
	}
}

// notifyUserRoleChanged alerts the user when their assigned role changes.
func (h *UIHandler) notifyUserRoleChanged(ctx context.Context, userID int64, orgID int64, newRole string) {
	if userID <= 0 {
		return
	}
	vars := map[string]string{
		"new_role": newRole,
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchEvent(ctx, notifications.EventUserRoleChanged, userID, orgPtr, vars)
}

// notifyUserPasswordSetByAdmin alerts the user when an admin sets a new password for their account.
func (h *UIHandler) notifyUserPasswordSetByAdmin(ctx context.Context, userID int64) {
	if userID <= 0 {
		return
	}
	h.dispatchEvent(ctx, notifications.EventUserPasswordSetByAdmin, userID, nil, nil)
}

// notifyAccountRegistered dispatches welcome notification to a newly registered account.
func (h *UIHandler) notifyAccountRegistered(ctx context.Context, userID int64, orgID *int64) {
	h.dispatchEvent(ctx, notifications.EventAccountRegistered, userID, orgID, nil)
}

// notifyAdminsNewRegistration alerts active platform staff members of a new registration.
func (h *UIHandler) notifyAdminsNewRegistration(ctx context.Context, _ int64, orgID int64, orgName, accountType string) {
	if strings.TrimSpace(orgName) == "" {
		orgName = i18n.T("ar", "notif.a_pharmacy")
	}
	if accountType == "job_seeker" || accountType == "seeker" {
		h.dispatchAdminEvent(ctx, notifications.EventAdminsNewJobSeekerRegistration, map[string]string{"user_name": orgName})
		return
	}
	kind := "صيدلية"
	if accountType == "vendor" || accountType == "supplier" {
		kind = "مورد"
	}
	h.dispatchAdminEvent(ctx, notifications.EventAdminsNewRegistration, map[string]string{
		"org_name":     orgName,
		"account_type": kind,
	})
}

// notifySubscriptionUpdated dispatches notification when a tenant subscribes or upgrades a plan.
func (h *UIHandler) notifySubscriptionUpdated(ctx context.Context, userID int64, orgID int64, planName string, cycle string, cost money.Amount, isUpgrade bool) {
	cycleStr := "شهري"
	if cycle == "annual" {
		cycleStr = "سنوي"
	}
	vars := map[string]string{
		"plan_name": planName,
		"cycle":     cycleStr,
		"cost":      cost.String(),
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	if userID > 0 {
		h.dispatchEvent(ctx, notifications.EventSubscriptionUpdated, userID, orgPtr, vars)
	}
	if orgID > 0 {
		h.dispatchOrgEvent(ctx, notifications.EventSubscriptionUpdated, orgID, vars)
	}
}

// notifySubscriptionExpiring alerts an organization when its subscription is expiring soon.
func (h *UIHandler) notifySubscriptionExpiring(ctx context.Context, orgID int64, planName string, daysLeft int) {
	if orgID <= 0 {
		return
	}
	vars := map[string]string{
		"plan_name": planName,
		"days_left": fmt.Sprintf("%d", daysLeft),
	}
	h.dispatchOrgEvent(ctx, notifications.EventSubscriptionExpiring, orgID, vars)
}

// notifySubscriptionExpired alerts an organization when its subscription has expired.
func (h *UIHandler) notifySubscriptionExpired(ctx context.Context, orgID int64, planName string) {
	if orgID <= 0 {
		return
	}
	vars := map[string]string{
		"plan_name": planName,
	}
	h.dispatchOrgEvent(ctx, notifications.EventSubscriptionExpired, orgID, vars)
}

// notifyNewReview alerts an organization when a customer publishes a review.
func (h *UIHandler) notifyNewReview(ctx context.Context, orgID int64, rating int, reviewText string) {
	if orgID <= 0 {
		return
	}
	vars := map[string]string{
		"rating":      fmt.Sprintf("%d", rating),
		"review_text": reviewText,
	}
	h.dispatchOrgEvent(ctx, notifications.EventOrgReviewCreated, orgID, vars)
}

// notifyOfflineChatMessage alerts an offline recipient of a new chat message.
func (h *UIHandler) notifyOfflineChatMessage(ctx context.Context, recipientUserID int64, senderName, snippet string) {
	if recipientUserID <= 0 {
		return
	}
	vars := map[string]string{
		"sender_name":     senderName,
		"message_snippet": snippet,
	}
	h.dispatchEvent(ctx, notifications.EventChatMessageOffline, recipientUserID, nil, vars)
}

// notifyJobApplicationReceived alerts an organization when a new candidate applies to a job.
func (h *UIHandler) notifyJobApplicationReceived(ctx context.Context, orgID int64, jobTitle, applicantName, applicantPhone string) {
	if orgID <= 0 {
		return
	}
	vars := map[string]string{
		"job_title":       jobTitle,
		"applicant_name":  applicantName,
		"applicant_phone": applicantPhone,
	}
	h.dispatchOrgEvent(ctx, notifications.EventJobApplicationReceived, orgID, vars)
}

// notifyOrgExtraDevicesGranted alerts an organization when an administrator grants extra concurrent sessions.
func (h *UIHandler) notifyOrgExtraDevicesGranted(ctx context.Context, orgID int64, extraDevices int, expiresAt *time.Time, totalSessions int) {
	if orgID <= 0 {
		return
	}
	expiresText := ""
	if expiresAt != nil && !expiresAt.IsZero() {
		expiresText = fmt.Sprintf(" صالحة حتى %s", expiresAt.Format("2006-01-02"))
	}
	vars := map[string]string{
		"extra_devices":  fmt.Sprintf("%d", extraDevices),
		"total_sessions": fmt.Sprintf("%d", totalSessions),
		"expires_text":   expiresText,
	}
	h.dispatchOrgEvent(ctx, notifications.EventOrgExtraDevicesGranted, orgID, vars)
}

// notifyOrgExtraDevicesRevoked alerts an organization when extra concurrent sessions are updated or reset.
func (h *UIHandler) notifyOrgExtraDevicesRevoked(ctx context.Context, orgID int64, totalSessions int) {
	if orgID <= 0 {
		return
	}
	vars := map[string]string{
		"total_sessions": fmt.Sprintf("%d", totalSessions),
	}
	h.dispatchOrgEvent(ctx, notifications.EventOrgExtraDevicesRevoked, orgID, vars)
}
