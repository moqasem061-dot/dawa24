package pages

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/attachments"
	"github.com/moqasem061-dot/dawa24/internal/modules/hr"
	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
)

// JobSeekerApprovalItem pairs a user with their job seeker profile for the approvals tab.
type JobSeekerApprovalItem struct {
	User    *identity.User
	Profile *hr.JobSeekerProfile
}

// AdminApprovalsData holds the unified view models for org approvals, document audits, and document requests.
type AdminApprovalsData struct {
	ActiveTab        string
	StatusFilter     string
	Organizations    []*org.Organization
	AllOrganizations []*org.Organization
	OrgDocs          map[int64][]*attachments.Document
	UploadedDocs     []*attachments.Document
	DocRequests      []*attachments.DocumentRequest
	JobSeekers       []JobSeekerApprovalItem
	OrgNames         map[int64]string
	OrgPage          int
	OrgPerPage       int
	OrgTotalCount    int
}
