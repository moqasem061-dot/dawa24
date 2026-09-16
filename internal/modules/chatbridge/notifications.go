package chatbridge

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Notifications on a chat channel.
//
// Dawa24 already decides who receives a notification: every producer fans out
// through the in-app feed, checking each member's permission as it goes, and
// the result is a row in notifications.logs per recipient. A channel does not
// decide that again. It reads the rows that already exist, for users with a
// confirmed link, and adds only what time can change:
//
//   - the user must still be entitled at delivery time — the grant is
//     re-resolved, because a role revoked between the event and the send must
//     not leak the event to a phone;
//   - the category must not be muted on the channel, and offers respect the
//     account-wide offers preference.

// DecideNotifications records a decision for each candidate. render writes
// the text of a notification that is to be sent.
func (c *Core) DecideNotifications(ctx context.Context, candidates []Candidate, render func(Candidate) string) ([]Decision, error) {
	sys := database.AsSystem(ctx)
	grants := map[[2]int64]rbac.Grant{}
	offers := map[int64]bool{}
	decisions := make([]Decision, 0, len(candidates))

	for _, cand := range candidates {
		d := Decision{LogID: cand.LogID, LinkID: cand.LinkID, Category: CategoryFor(cand.RequiredPermission, cand.Title)}
		switch {
		case Muted(cand.MutedCategories, d.Category):
			d.DropReason = "muted"
		case d.Category == CategoryOffers && !c.offersEnabled(sys, offers, cand.UserID):
			d.DropReason = "offers_preference_off"
		default:
			// A notification with no organisation is judged against the chat's
			// active منشأة — the same holding the in-app feed filters it by.
			orgID := cand.OrganizationID
			if orgID == 0 {
				orgID = cand.LinkActiveOrgID
			}
			key := [2]int64{cand.UserID, orgID}
			g, ok := grants[key]
			if !ok {
				var err error
				if g, err = c.Grants.Resolve(sys, cand.UserID, orgID); err != nil {
					return nil, err
				}
				grants[key] = g
			}
			d.DropReason = ineligible(g, orgID, cand.RequiredPermission)
		}
		if d.DropReason == "" {
			d.Text = render(cand)
		}
		decisions = append(decisions, d)
	}
	return decisions, nil
}

// ineligible applies the rule the organisation fan-out applies when it writes
// the feed row (owners, platform staff, or holders of the permission), plus
// the two things only time can change: the account and the membership.
func ineligible(g rbac.Grant, orgID int64, permission string) string {
	platformSide := g.IsStaff || g.IsPlatformOwner
	switch {
	case !g.Active:
		return "account_inactive"
	case orgID > 0 && g.Scope == "":
		return "membership_ended"
	case permission == "":
		return ""
	case platformSide || g.IsOrgOwner || g.Can(permission):
		return ""
	}
	return "permission_revoked"
}

func (c *Core) offersEnabled(sys context.Context, cache map[int64]bool, userID int64) bool {
	if v, ok := cache[userID]; ok {
		return v
	}
	v, err := c.Store.OffersTopicEnabled(sys, userID)
	if err != nil {
		// Unknown preference: do not send marketing.
		c.Log.WarnContext(sys, "chatbridge: read offers preference", "error", err, "user_id", userID)
		v = false
	}
	cache[userID] = v
	return v
}
