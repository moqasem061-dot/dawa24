package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
)

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
