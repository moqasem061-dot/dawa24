package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminOrgImportTempWarehouseUploadSubmit delegates directly to CompareUploadSubmit
// to provide identical robust multi-file streaming, plan quota, and error handling.
func (h *UIHandler) AdminOrgImportTempWarehouseUploadSubmit(w http.ResponseWriter, r *http.Request) {
	h.CompareUploadSubmit(w, r)
}

// AdminOrgImportComparePage renders the 3-column Compare Tool workspace on behalf of the target organization.
func (h *UIHandler) AdminOrgImportComparePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	if orgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "validation.invalid_id"))
		return
	}

	sysCtx := database.WithTenant(database.AsSystem(ctx), orgID)
	var orgName string
	if h.orgSvc != nil {
		targetOrg, err := h.orgSvc.GetOrganization(sysCtx, orgID)
		if err != nil || targetOrg == nil {
			h.redirectWithNotice(w, r, "/admin/organizations/import", "error", "المنشأة المحددة غير موجودة")
			return
		}
		orgName, _ = h.resolveTargetOrgInfo(sysCtx, orgID)
	}

	var activeFiles []*compare.CompareFile
	if h.compareSvc != nil {
		activeFiles, _ = h.compareSvc.ListFiles(sysCtx, 0, &orgID, nil)
	}

	// File Center storage quota is determined strictly by the organization's subscription plan.
	maxAllowedFiles := 10
	if h.billSvc != nil {
		if plan, err := h.billSvc.GetEffectivePlan(sysCtx, 0, &orgID); err == nil && plan != nil {
			maxAllowedFiles = plan.GetMaxCompareFiles()
		}
	}

	savingCount := 0
	if h.catSvc != nil {
		if _, stats, err := h.catSvc.ListAllSavingProductsAdmin(sysCtx, nil, &orgID, "", "all", 1, 0); err == nil && stats != nil {
			savingCount = stats.TotalProducts
		}
	}

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")

	view := pages.CompareToolView{
		Lang:                lang,
		Dir:                 dir,
		Files:               activeFiles,
		MaxAllowedFiles:     maxAllowedFiles,
		NoticeType:          noticeType,
		NoticeMsg:           noticeMsg,
		Audience:            "admin",
		TargetOrgID:         orgID,
		TargetOrgName:       orgName,
		UploadURL:           fmt.Sprintf("/admin/organizations/import/%d/temp-warehouse/upload", orgID),
		SavingProductsCount: savingCount,
	}

	h.renderPage(ctx, w, "render admin org compare tool", pages.CompareToolPage(view))
}


