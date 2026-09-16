package identity

import "github.com/muhiya/dawa24-store/internal/shared/i18n"

// RoleModerator is the platform role key a moderator holds.
const RoleModerator = "moderator"

// Moderator is one moderator and their place in the hierarchy.
type Moderator struct {
	UserID           int64     `json:"user_id"`
	Name             i18n.Text `json:"name"`
	Email            string    `json:"email"`
	Role             string    `json:"role"`
	ParentID         *int64    `json:"parent_id,omitempty"`
	ParentName       i18n.Text `json:"parent_name,omitempty"`
	SubordinateCount int       `json:"subordinate_count"`
}

// IsMain reports whether this moderator is top-level.
func (m Moderator) IsMain() bool { return m.ParentID == nil }

// LeadsTeam reports whether anyone reports to this moderator.
func (m Moderator) LeadsTeam() bool { return m.SubordinateCount > 0 }
