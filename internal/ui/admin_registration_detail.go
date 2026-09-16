package ui

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The registration review, assembled on demand.
//
// It is loaded rather than embedded for two reasons. The page renders one modal
// per row, so embedding the branches, works and documents for twenty-five
// pending companies would ship all of it to a reviewer who opens one. And the
// branch institutional works are a per-branch read: doing them for every row of
// every page is a query count nobody notices until the queue is long.

// AdminOrgRegistrationFragment serves one organisation's full registration.
func (h *UIHandler) AdminOrgRegistrationFragment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.orgSvc == nil {
		h.renderRegistrationFragment(ctx, w, pages.AdminOrgRegistrationDetail{}, lang)
		return
	}

	sysCtx := database.AsSystem(ctx)
	organization, err := h.orgSvc.GetOrganization(sysCtx, id)
	if err != nil {
		h.log.ErrorContext(ctx, "load organization for review", "org_id", id, "error", err)
		h.renderError(w, r, err)
		return
	}

	detail := pages.AdminOrgRegistrationDetail{
		Org:   organization,
		Works: make(map[int64][]*org.InstitutionalWork),
	}

	// Branches and their institutional works. A branch with no work cannot buy
	// or be bought from, so the reviewer is shown that before approving rather
	// than discovering it when the company reports an empty catalogue.
	if branches, bErr := h.orgSvc.ListBranches(sysCtx, id); bErr == nil {
		detail.Branches = branches
		for _, b := range branches {
			if b == nil {
				continue
			}
			works, wErr := h.orgSvc.GetBranchInstitutionalWorks(sysCtx, b.ID)
			if wErr != nil {
				h.log.WarnContext(ctx, "load branch institutional works",
					"branch_id", b.ID, "error", wErr)
				continue
			}
			detail.Works[b.ID] = works
		}
	} else {
		h.log.WarnContext(ctx, "load branches for review", "org_id", id, "error", bErr)
	}

	if h.attSvc != nil {
		if docs, dErr := h.attSvc.ListByOrganization(sysCtx, id); dErr == nil {
			detail.Documents = docs
		} else {
			h.log.WarnContext(ctx, "load documents for review", "org_id", id, "error", dErr)
		}
	}

	if organization != nil && organization.OwnerID > 0 && h.idSvc != nil {
		if u, uErr := h.idSvc.GetUserByID(sysCtx, organization.OwnerID); uErr == nil && u != nil {
			owner := &pages.AdminRegistrationOwner{
				ID:    u.ID,
				Name:  u.Name.Get(i18n.Lang(lang)),
				Email: u.Email,
				Phone: u.Phone,
			}
			if owner.Name == "" {
				owner.Name = u.Name.Get(i18n.AR)
			}
			if owner.Name == "" {
				owner.Name = u.Name.Get(i18n.EN)
			}
			detail.Owner = owner
		}
	}

	h.renderRegistrationFragment(ctx, w, detail, lang)
}

func (h *UIHandler) renderRegistrationFragment(
	ctx context.Context, w http.ResponseWriter, d pages.AdminOrgRegistrationDetail, lang string,
) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOrgRegistrationFragment(d, lang).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render registration fragment", "error", err)
	}
}
