package org

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// A company's own profile, edited one section at a time.
//
// /vendor/organization used to be a single form with a single "save everything"
// button, and the one field it wrote — org.organizations.name — is read by
// nothing. Every screen that displays a company reads trade_name first and
// falls back to legal_name, so a supplier edited their trade name, the write
// succeeded, and nothing they could see anywhere changed.
//
// Splitting it into sections is what makes an approval workflow possible
// without making the page unusable: the four fields the platform verified when
// it approved the company need a moderator, and a phone number does not.

// ProfileSection names one group of company fields.
type ProfileSection string

const (
	// SectionIdentity is what the platform verified at approval: the legal
	// name, the trade name, the commercial register and the tax number.
	SectionIdentity ProfileSection = "identity"
	// SectionLimits is the minimum and maximum order value.
	SectionLimits ProfileSection = "limits"
	// SectionContact is the e-mail, phone, address and organization number.
	SectionContact ProfileSection = "contact"
	// SectionDescription is the public blurb.
	SectionDescription ProfileSection = "description"
	// SectionMedia is the logo and cover image.
	SectionMedia ProfileSection = "media"
)

// ProfileSections lists every section, in the order the page renders them.
func ProfileSections() []ProfileSection {
	return []ProfileSection{
		SectionIdentity, SectionLimits, SectionContact, SectionDescription, SectionMedia,
	}
}

// NeedsApproval reports whether changing this section requires a moderator.
//
// Only identity does. Those four fields are what the platform checked against
// the company's papers when it approved them; letting a supplier rewrite them
// afterwards would leave an approved record that no longer describes what was
// approved. Everything else is the company's own business and applies at once —
// a wrong phone number should not need a review queue.
func (s ProfileSection) NeedsApproval() bool { return s == SectionIdentity }

// Valid reports whether this is a section the platform knows.
func (s ProfileSection) Valid() bool {
	for _, known := range ProfileSections() {
		if s == known {
			return true
		}
	}
	return false
}

// ProfileChangeStatus is where a request has got to.
type ProfileChangeStatus string

const (
	ChangePending   ProfileChangeStatus = "pending"
	ChangeApproved  ProfileChangeStatus = "approved"
	ChangeRejected  ProfileChangeStatus = "rejected"
	ChangeWithdrawn ProfileChangeStatus = "withdrawn"
)

// ProfileFields is one section's values, keyed by form field name.
//
// A map rather than a struct per section: the sections do not share fields, the
// set is small, and an administrator's review screen has to render whatever a
// request happens to carry without knowing which section it is.
type ProfileFields map[string]string

// Value implements driver.Valuer for the JSONB columns.
func (f ProfileFields) Value() (driver.Value, error) {
	if f == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(f)
}

// Scan implements sql.Scanner for the JSONB columns.
func (f *ProfileFields) Scan(src any) error {
	if src == nil {
		*f = ProfileFields{}
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("org: cannot scan %T into ProfileFields", src)
	}
	if len(raw) == 0 || string(raw) == "null" {
		*f = ProfileFields{}
		return nil
	}
	return json.Unmarshal(raw, f)
}

// ProfileChangeRequest is one proposed edit awaiting a decision.
type ProfileChangeRequest struct {
	ID             int64               `json:"id"`
	PublicID       string              `json:"public_id"`
	OrganizationID int64               `json:"organization_id"`
	RequestedBy    int64               `json:"requested_by"`
	Section        ProfileSection      `json:"section"`
	Proposed       ProfileFields       `json:"proposed"`
	Previous       ProfileFields       `json:"previous"`
	Status         ProfileChangeStatus `json:"status"`
	AdminNotes     string              `json:"admin_notes"`
	ReviewedBy     *int64              `json:"reviewed_by,omitempty"`
	ReviewedAt     *time.Time          `json:"reviewed_at,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`

	// OrganizationName, RequesterName and ReviewerName are filled in for the admin queue,
	// which lists requests across every company.
	OrganizationName string `json:"organization_name,omitempty"`
	RequesterName    string `json:"requester_name,omitempty"`
	ReviewerName     string `json:"reviewer_name,omitempty"`
}

// ProfileChangeCounts aggregates queue counts by status.
type ProfileChangeCounts struct {
	Pending   int `json:"pending"`
	Approved  int `json:"approved"`
	Rejected  int `json:"rejected"`
	Withdrawn int `json:"withdrawn"`
	All       int `json:"all"`
}

// ProfileChangeFilter defines criteria for filtering the admin review queue.
type ProfileChangeFilter struct {
	Status         string     `json:"status,omitempty"`
	OrganizationID int64      `json:"organization_id,omitempty"`
	Section        string     `json:"section,omitempty"`
	DateFrom       *time.Time `json:"date_from,omitempty"`
	DateTo         *time.Time `json:"date_to,omitempty"`
	Search         string     `json:"search,omitempty"`
}

// Changed lists the fields whose value the request would alter, so the review
// screen can show a diff rather than two blocks of identical text.
func (r *ProfileChangeRequest) Changed() []string {
	var out []string
	for key, next := range r.Proposed {
		if r.Previous[key] != next {
			out = append(out, key)
		}
	}
	return out
}

// SaveProfileSection is a request to change one section.
type SaveProfileSection struct {
	OrganizationID int64
	UserID         int64
	Section        ProfileSection
	Fields         ProfileFields
}

// Validate checks the shape of the request, not the values.
func (in SaveProfileSection) Validate() error {
	if in.OrganizationID <= 0 || in.UserID <= 0 {
		return apperr.Validation("profile.invalid_actor",
			"A valid organization and user are required.", nil)
	}
	if !in.Section.Valid() {
		return apperr.Validation("org.profile.unknown_section",
			"Unknown profile section.", nil)
	}
	if len(in.Fields) == 0 {
		return apperr.Validation("profile.empty_section",
			"The submitted section carried no fields.", nil)
	}
	return nil
}
