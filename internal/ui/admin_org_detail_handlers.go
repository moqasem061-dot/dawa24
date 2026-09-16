package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminOrganizationDetailPage renders the deep 360° profile overview for an organization.
func (h *UIHandler) AdminOrganizationDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	orgID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || orgID <= 0 {
		http.Redirect(w, r, "/admin/organizations", http.StatusSeeOther)
		return
	}

	var organization *org.Organization
	var branches []*org.Branch
	var employees []*org.EmployeeView
	var docs []*attachments.Document
	var wallet *billing.Wallet
	var recentTxs []*billing.WalletTransaction
	var recentDeposits []*billing.WalletDeposit
	var ordersCount int
	var recentOrders []*commerce.Order

	if h.orgSvc != nil {
		organization, _ = h.orgSvc.GetOrganization(sysCtx, orgID)
		branches, _ = h.orgSvc.ListBranches(sysCtx, orgID)
		employees, _ = h.orgSvc.ListEmployees(sysCtx, orgID)
	}

	if organization == nil {
		h.redirectWithNotice(w, r, "/admin/organizations", "error", i18n.T(lang, "admin.org.not_found"))
		return
	}

	if h.attSvc != nil {
		docs, _ = h.attSvc.ListByOrganization(sysCtx, orgID)
	}

	if h.billSvc != nil && organization.OwnerID > 0 {
		wallet, _ = h.billSvc.GetWallet(sysCtx, organization.OwnerID, "EGP")
		if wallet != nil {
			recentTxs, _ = h.billSvc.ListWalletTransactions(sysCtx, wallet.ID, 5, 0)
		}
		recentDeposits, _ = h.billSvc.ListUserDeposits(sysCtx, organization.OwnerID, 5, 0)
	}

	if h.commSvc != nil {
		if ords, err := h.commSvc.AdminSearchOrders(sysCtx, "", 20, 0); err == nil {
			for _, o := range ords {
				if o != nil && o.OrganizationID != nil && *o.OrganizationID == orgID {
					ordersCount++
					if len(recentOrders) < 5 {
						recentOrders = append(recentOrders, o)
					}
				}
			}
		}
	}

	aiUserID, aiKey := h.EnsureOrgAIGatewayProvisioned(ctx, orgID)

	var planName string
	maxSessions := 3
	maxDevices := 3
	if h.idSvc != nil {
		if s, d, p, err := h.idSvc.GetOrgPlanLimits(sysCtx, orgID); err == nil {
			maxSessions = s
			maxDevices = d
			planName = p
		}
	}
	activeExtra := organization.ActiveExtraDevices()
	baseMaxSessions := maxSessions - activeExtra
	if baseMaxSessions < 1 {
		baseMaxSessions = 1
	}
	baseMaxDevices := maxDevices - activeExtra
	if baseMaxDevices < 1 {
		baseMaxDevices = 1
	}

	activeSessionsCount := 0
	if h.idSvc != nil {
		if orgSessions, err := h.idSvc.ListOrgSessions(sysCtx, orgID); err == nil {
			activeSessionsCount = len(orgSessions)
		}
	}

	data := pages.AdminOrgDetailData{
		Organization:        organization,
		Branches:            branches,
		Employees:           employees,
		Documents:           docs,
		Wallet:              wallet,
		RecentTxs:           recentTxs,
		RecentDeposits:      recentDeposits,
		OrdersCount:         ordersCount,
		RecentOrders:        recentOrders,
		AIUserID:            aiUserID,
		AIVirtualKey:        aiKey,
		PlanName:            planName,
		BaseMaxSessions:     baseMaxSessions,
		BaseMaxDevices:      baseMaxDevices,
		TotalMaxSessions:    maxSessions,
		ActiveSessionsCount: activeSessionsCount,
	}

	h.renderPage(ctx, w, "render admin org detail", pages.AdminOrganizationDetailPage(data, lang, dir))
}

// AdminOrganizationInfoPage redirects to the 360 org detail profile.
func (h *UIHandler) AdminOrganizationInfoPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, "/admin/organizations/"+idStr, http.StatusMovedPermanently)
}

// AdminOrganizationUsersPage redirects to the 360 org detail profile.
func (h *UIHandler) AdminOrganizationUsersPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, "/admin/organizations/"+idStr, http.StatusMovedPermanently)
}

// AdminOrganizationBranchesPage redirects to the branches filter for this org.
func (h *UIHandler) AdminOrganizationBranchesPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, "/admin/branches?org_id="+idStr, http.StatusSeeOther)
}

// AdminOrgEditSubmit processes admin edits to an organization's profile and settings.
func (h *UIHandler) AdminOrgEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/admin/organizations", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/organizations", "error", "معرف المنشأة غير صالح")
		return
	}

	redirectTo := strings.TrimSpace(r.PostFormValue("redirect_to"))
	if redirectTo == "" {
		redirectTo = "/admin/organizations"
	}

	existing, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), id)
	if err != nil || existing == nil {
		h.redirectWithNotice(w, r, redirectTo, "error", "لم يتم العثور على المنشأة المطلوبة")
		return
	}

	legalName := strings.TrimSpace(r.PostFormValue("legal_name"))
	if legalName == "" {
		h.redirectWithNotice(w, r, redirectTo, "error", "الاسم القانوني للمنشأة مطلوب ولا يمكن تركه فارغاً")
		return
	}

	tradeNameAr := strings.TrimSpace(r.PostFormValue("trade_name_ar"))
	if tradeNameAr == "" {
		tradeNameAr = legalName
	}
	tradeNameEn := strings.TrimSpace(r.PostFormValue("trade_name_en"))
	if tradeNameEn == "" {
		tradeNameEn = tradeNameAr
	}

	orgTypeStr := strings.TrimSpace(r.PostFormValue("type"))
	var orgType org.OrganizationType
	if orgTypeStr == string(org.TypeCustomer) {
		orgType = org.TypeCustomer
	} else {
		orgType = org.TypeVendor
	}

	orgStatusStr := strings.TrimSpace(r.PostFormValue("status"))
	var orgStatus org.OrganizationStatus
	switch orgStatusStr {
	case string(org.StatusApproved):
		orgStatus = org.StatusApproved
	case string(org.StatusPending):
		orgStatus = org.StatusPending
	case string(org.StatusRejected):
		orgStatus = org.StatusRejected
	case string(org.StatusSuspended):
		orgStatus = org.StatusSuspended
	case "deleted":
		orgStatus = "deleted"
	default:
		orgStatus = existing.Status
	}

	commReg := strings.TrimSpace(r.PostFormValue("commercial_register"))
	taxNum := strings.TrimSpace(r.PostFormValue("tax_number"))
	pharmaLic := strings.TrimSpace(r.PostFormValue("pharmacist_license"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	email := strings.TrimSpace(r.PostFormValue("email"))
	address := strings.TrimSpace(r.PostFormValue("address"))
	notes := strings.TrimSpace(r.PostFormValue("verification_notes"))

	creditLimit := existing.CreditLimit
	if val := strings.TrimSpace(r.PostFormValue("credit_limit")); val != "" {
		if amt, err := money.Parse(val); err == nil && !amt.IsNegative() {
			creditLimit = amt
		}
	}

	payTerms := existing.PaymentTermsDays
	if val := strings.TrimSpace(r.PostFormValue("payment_terms_days")); val != "" {
		if days, err := strconv.Atoi(val); err == nil && days >= 0 {
			payTerms = days
		}
	}

	minPrice := existing.MinOrderPrice
	if val := strings.TrimSpace(r.PostFormValue("min_order_price")); val != "" {
		if amt, err := money.Parse(val); err == nil && !amt.IsNegative() {
			minPrice = amt
		}
	}

	maxPrice := existing.MaxOrderPrice
	if val := strings.TrimSpace(r.PostFormValue("max_order_price")); val != "" {
		if amt, err := money.Parse(val); err == nil && !amt.IsNegative() {
			maxPrice = amt
		}
	}

	if maxPrice.Minor() < minPrice.Minor() {
		maxPrice = minPrice
	}

	updated := &org.Organization{
		ID:                 existing.ID,
		PublicID:           existing.PublicID,
		LegalName:          legalName,
		TradeName:          i18n.New(tradeNameAr, tradeNameEn),
		Name:               i18n.New(tradeNameAr, tradeNameEn),
		Type:               orgType,
		Status:             orgStatus,
		CommercialRegister: commReg,
		TaxNumber:          taxNum,
		PharmacistLicense:  pharmaLic,
		Phone:              phone,
		Email:              email,
		Address:            address,
		CreditLimit:        creditLimit,
		PaymentTermsDays:   payTerms,
		MinOrderPrice:      minPrice,
		MaxOrderPrice:      maxPrice,
		VerificationNotes:  notes,
		OwnerID:            existing.OwnerID,
	}

	if err := h.orgSvc.UpdateOrganization(database.AsSystem(ctx), updated); err != nil {
		h.log.ErrorContext(ctx, "admin update organization error", "error", err, "org_id", id)
		h.redirectWithNotice(w, r, redirectTo, "error", "فشل في حفظ تعديلات المنشأة: "+err.Error())
		return
	}

	h.redirectWithNotice(w, r, redirectTo, "success", "تم تحديث بيانات المنشأة بنجاح.")
}

// AdminOrgExtraDevicesSubmit processes temporary extra connected devices quota and duration for an organization.
func (h *UIHandler) AdminOrgExtraDevicesSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/admin/organizations", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/organizations", "error", "معرف المنشأة غير صالح")
		return
	}

	redirectTo := fmt.Sprintf("/admin/organizations/%d", id)

	extraDevicesStr := strings.TrimSpace(r.PostFormValue("extra_devices"))
	durationDaysStr := strings.TrimSpace(r.PostFormValue("duration_days"))
	customExpiresAtStr := strings.TrimSpace(r.PostFormValue("expires_at"))

	extraDevices, _ := strconv.Atoi(extraDevicesStr)
	if extraDevices < 0 {
		extraDevices = 0
	}

	var expiresAt *time.Time
	if extraDevices > 0 {
		if customExpiresAtStr != "" {
			if parsed, tErr := time.Parse("2006-01-02T15:04", customExpiresAtStr); tErr == nil {
				expiresAt = &parsed
			} else if parsed, tErr := time.Parse("2006-01-02", customExpiresAtStr); tErr == nil {
				parsed = parsed.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
				expiresAt = &parsed
			}
		}
		if expiresAt == nil && durationDaysStr != "" && durationDaysStr != "custom" {
			days, dErr := strconv.Atoi(durationDaysStr)
			if dErr == nil && days > 0 {
				exp := time.Now().AddDate(0, 0, days)
				expiresAt = &exp
			}
		}
		// Default to 30 days if extra devices granted without specific expiry
		if expiresAt == nil {
			exp := time.Now().AddDate(0, 0, 30)
			expiresAt = &exp
		}
	}

	if err := h.orgSvc.SetExtraDevices(database.AsSystem(ctx), id, extraDevices, expiresAt); err != nil {
		h.log.ErrorContext(ctx, "admin set extra devices error", "error", err, "org_id", id)
		h.redirectWithNotice(w, r, redirectTo, "error", "تعذر تحديث الأجهزة الإضافية للمنشأة: "+err.Error())
		return
	}

	h.safeGo("notify-org-extra-devices", func() {
		bgCtx := database.AsSystem(context.Background())
		totalSessions := 3
		if h.idSvc != nil {
			if s, _, _, err := h.idSvc.GetOrgPlanLimits(bgCtx, id); err == nil && s > 0 {
				totalSessions = s
			}
		}
		if extraDevices > 0 {
			h.notifyOrgExtraDevicesGranted(bgCtx, id, extraDevices, expiresAt, totalSessions)
		} else {
			h.notifyOrgExtraDevicesRevoked(bgCtx, id, totalSessions)
		}
	})

	if extraDevices > 0 {
		h.redirectWithNotice(w, r, redirectTo, "success", fmt.Sprintf("تم منح المنشأة +%d أجهزة إضافية بنجاح حتى %s.", extraDevices, expiresAt.Format("2006-01-02")))
	} else {
		h.redirectWithNotice(w, r, redirectTo, "success", "تم إلغاء الأجهزة الإضافية وإعادة المنشأة لحد باقتها الأصلي بنجاح.")
	}
}
