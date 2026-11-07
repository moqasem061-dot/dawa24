package pages

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/modules/workflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// RequestsData is the /requests inbox view model.
type RequestsData struct {
	Requests     []*workflow.Request
	CurrentOrgID int64
	Suppliers    []*org.Organization
	Page         int
	PerPage      int
	TotalCount   int
}

// reqTypeLabel maps a request type onto an Arabic label.
func reqTypeLabel(t workflow.RequestType) string {
	switch t {
	case workflow.RequestAction:
		return i18n.T("ar", "common.action")
	case workflow.RequestApproval:
		return i18n.T("ar", "common.approval")
	default:
		return i18n.T("ar", "common.document")
	}
}

// reqStatusLabel maps a request status onto an Arabic label.
func reqStatusLabel(s workflow.RequestStatus) string {
	switch s {
	case workflow.RequestAccepted:
		return i18n.T("ar", "common.accepted")
	case workflow.RequestDeclined:
		return i18n.T("ar", "common.rejected")
	case workflow.RequestCancelled:
		return i18n.T("ar", "common.cancelled")
	default:
		return i18n.T("ar", "common.pending")
	}
}
