package notifications

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// EventKey represents a unique, stable notification event identifier.
type EventKey string

const (
	// Commerce & Orders
	EventOrderPlaced           EventKey = "order.placed"
	EventOrderStatusChanged    EventKey = "order.status_changed"
	EventOrderCancelledByBuyer EventKey = "order.cancelled_by_buyer"

	// Purchase Requests & RFQs
	EventPurchaseRequestCreated   EventKey = "purchase_request.created"
	EventPurchaseRequestResponded EventKey = "purchase_request.responded"
	EventQuoteRequested           EventKey = "quote.requested"
	EventQuoteProvided            EventKey = "quote.provided"
	EventQuoteDecision            EventKey = "quote.decision"
	EventNegotiationOffer         EventKey = "negotiation.offer"
	EventNegotiationDecision      EventKey = "negotiation.decision"

	// Promotions, Ads & Sponsorships
	EventSpecialOfferStatus EventKey = "special_offer.status"
	EventSponsorshipStatus  EventKey = "sponsorship.status"
	EventAdStatus           EventKey = "ad.status"

	// Smart Order & Imports
	EventSmartOrderRunFinished EventKey = "smart_order.run_finished"
	EventSmartOrderRunFailed   EventKey = "smart_order.run_failed"
	EventImportRunFinished     EventKey = "import.run_finished"
	EventImportRunFailed       EventKey = "import.run_failed"

	// Quotas
	EventQuotaExhausted EventKey = "quota.exhausted"
	EventQuotaReleased  EventKey = "quota.released"

	// Profile & Organization Changes
	EventOrgProfileChangeRequested EventKey = "org.profile_change.requested"
	EventOrgProfileChangeApproved  EventKey = "org.profile_change.approved"
	EventOrgProfileChangeRejected  EventKey = "org.profile_change.rejected"

	// Organization Deletion
	EventOrgDeletionRequested EventKey = "org.deletion.requested"
	EventOrgDeletionApproved  EventKey = "org.deletion.approved"
	EventOrgDeletionRejected  EventKey = "org.deletion.rejected"

	// Account Deletion
	EventAccountDeletionRequested EventKey = "account.deletion.requested"
	EventAccountDeletionApproved  EventKey = "account.deletion.approved"
	EventAccountDeletionRejected  EventKey = "account.deletion.rejected"

	// Organization Lifecycle & Approvals
	EventOrgApproved    EventKey = "org.approved"
	EventOrgRejected    EventKey = "org.rejected"
	EventOrgSuspended   EventKey = "org.suspended"
	EventOrgReactivated EventKey = "org.reactivated"

	// Documents
	EventDocumentRequested EventKey = "document.requested"
	EventDocumentVerified  EventKey = "document.verified"
	EventDocumentRejected  EventKey = "document.rejected"

	// Branches & Institutional Works
	EventBranchCreated                   EventKey = "branch.created"
	EventBranchDisabled                  EventKey = "branch.disabled"
	EventBranchInstitutionalWorksChanged EventKey = "branch.institutional_works.changed"

	// Team & Identity
	EventUserAddedToOrg         EventKey = "org.user.added"
	EventUserRemovedFromOrg     EventKey = "org.user.removed"
	EventUserRoleChanged        EventKey = "user.role.changed"
	EventUserPasswordSetByAdmin EventKey = "user.password.set_by_admin"
	EventAccountRegistered              EventKey = "account.registered"
	EventAdminsNewRegistration          EventKey = "admin.new_registration"
	EventAdminsNewJobSeekerRegistration EventKey = "admin.new_job_seeker_registration"

	// Subscriptions & Billing
	EventSubscriptionUpdated    EventKey = "subscription.updated"
	EventSubscriptionExpiring   EventKey = "subscription.expiring"
	EventSubscriptionExpired    EventKey = "subscription.expired"
	EventOrgExtraDevicesGranted EventKey = "org.extra_devices.granted"
	EventOrgExtraDevicesRevoked EventKey = "org.extra_devices.revoked"
	EventRefundIssued           EventKey = "billing.refund.issued"

	// Wallet
	EventWalletDepositPending    EventKey = "wallet.deposit.pending"
	EventWalletDepositApproved   EventKey = "wallet.deposit.approved"
	EventWalletDepositRejected   EventKey = "wallet.deposit.rejected"
	EventWalletWithdrawalPending  EventKey = "wallet.withdrawal.pending"
	EventWalletWithdrawalApproved EventKey = "wallet.withdrawal.approved"
	EventWalletWithdrawalRejected EventKey = "wallet.withdrawal.rejected"

	// Reviews, Chat, HR
	EventOrgReviewCreated       EventKey = "org.review.created"
	EventChatMessageOffline     EventKey = "chat.message.offline"
	EventJobApplicationReceived EventKey = "hr.job_application.received"
)

// EventDefinition describes metadata, routing, and bilingual copy for a notification event.
type EventDefinition struct {
	Key                EventKey  `json:"key"`
	DefaultChannels    []Channel `json:"default_channels"`
	RequiredPermission string    `json:"required_permission"`
	TitleAr            string    `json:"title_ar"`
	TitleEn            string    `json:"title_en"`
	BodyAr             string    `json:"body_ar"`
	BodyEn             string    `json:"body_en"`
}

// Render formats the title and body for the given language using supplied variables.
func (e EventDefinition) Render(lang string, vars map[string]string) (string, string) {
	titleTmpl := e.TitleAr
	bodyTmpl := e.BodyAr
	if strings.ToLower(lang) == "en" {
		if e.TitleEn != "" {
			titleTmpl = e.TitleEn
		}
		if e.BodyEn != "" {
			bodyTmpl = e.BodyEn
		}
	}
	return InterpolateTemplate(titleTmpl, vars), InterpolateTemplate(bodyTmpl, vars)
}

// registry is the static catalog of all supported platform notification events.
var registry = make(map[EventKey]EventDefinition)

func registerEvent(def EventDefinition) {
	registry[def.Key] = def
}

// GetEvent looks up an event definition by key.
func GetEvent(key EventKey) (EventDefinition, bool) {
	e, ok := registry[key]
	return e, ok
}

// AllEvents returns all registered platform notification events.
func AllEvents() []EventDefinition {
	list := make([]EventDefinition, 0, len(registry))
	for _, e := range registry {
		list = append(list, e)
	}
	return list
}

// ToTemplate transforms an EventDefinition into a Template struct for database storage.
func (e EventDefinition) ToTemplate() Template {
	channel := ChannelInApp
	if len(e.DefaultChannels) > 0 {
		channel = e.DefaultChannels[0]
	}
	return Template{
		Slug:    string(e.Key),
		Channel: channel,
		Title: i18n.Text{
			i18n.AR: e.TitleAr,
			i18n.EN: e.TitleEn,
		},
		Body: i18n.Text{
			i18n.AR: e.BodyAr,
			i18n.EN: e.BodyEn,
		},
		IsActive: true,
	}
}
