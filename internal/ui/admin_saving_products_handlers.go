package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminSavingProductsPage renders saving products (منتجات التوفير) across all users and organizations.
func (h *UIHandler) AdminSavingProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var userID *int64
	var orgID *int64
	var selectedUserID, selectedOrgID int64

	if uStr := chi.URLParam(r, "userId"); uStr != "" {
		if uid, err := strconv.ParseInt(uStr, 10, 64); err == nil && uid > 0 {
			userID = &uid
			selectedUserID = uid
		}
	}
	if oStr := chi.URLParam(r, "organizationId"); oStr != "" {
		if oid, err := strconv.ParseInt(oStr, 10, 64); err == nil && oid > 0 {
			orgID = &oid
			selectedOrgID = oid
		}
	}

	if qUID := r.URL.Query().Get("user_id"); qUID != "" {
		if uid, err := strconv.ParseInt(qUID, 10, 64); err == nil && uid > 0 {
			userID = &uid
			selectedUserID = uid
		}
	}
	if qOID := r.URL.Query().Get("org_id"); qOID != "" {
		if oid, err := strconv.ParseInt(qOID, 10, 64); err == nil && oid > 0 {
			orgID = &oid
			selectedOrgID = oid
		}
	}

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	if filter == "" {
		filter = "all"
	}

	var items []*catalog.SavingProductAdminView
	var stats *catalog.SavingProductAdminStats

	limit := h.pageLimit(r)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	if h.catSvc != nil {
		var err error
		items, stats, err = h.catSvc.ListAllSavingProductsAdmin(database.AsSystem(ctx), userID, orgID, search, filter, limit, offset)
		if err != nil {
			h.log.ErrorContext(ctx, "admin list saving products", "error", err)
		}
	}
	if stats == nil {
		stats = &catalog.SavingProductAdminStats{}
	}

	var orgOptions []*pages.SavingUserOrgOption
	if h.orgSvc != nil {
		if orgs, err := h.orgSvc.ListOrganizations(database.AsSystem(ctx), nil, nil, 200, 0); err == nil {
			for _, o := range orgs {
				name := o.LegalName
				if name == "" {
					name = o.TradeName.Get("ar")
				}
				orgOptions = append(orgOptions, &pages.SavingUserOrgOption{
					ID:   o.ID,
					Name: name,
					Type: string(o.Type),
				})
			}
		}
	}

	var userOptions []*pages.SavingUserOrgOption
	if h.idSvc != nil {
		if users, err := h.idSvc.AdminListUsers(database.AsSystem(ctx), "", ""); err == nil {
			for _, u := range users {
				name := u.Name.Get("ar")
				if name == "" {
					name = u.Name.Get("en")
				}
				if name == "" {
					name = u.Email
				}
				userOptions = append(userOptions, &pages.SavingUserOrgOption{
					ID:   u.ID,
					Name: name,
					Type: u.Email,
				})
			}
		}
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	data := pages.AdminSavingProductsData{
		Items:          items,
		Stats:          stats,
		Organizations:  orgOptions,
		Users:          userOptions,
		SelectedOrgID:  selectedOrgID,
		SelectedUserID: selectedUserID,
		SearchQuery:    search,
		ActiveFilter:   filter,
		NoticeType:     noticeType,
		NoticeMsg:      noticeMsg,
		Pagination: components.PaginationProps{
			CurrentPage: page,
			PageSize:    limit,
			TotalCount:  stats.FilteredCount,
			BaseURL:     "/admin/saving-products",
			QueryValues: r.URL.Query(),
		},
	}

	h.renderPage(ctx, w, "render saving products", pages.AdminSavingProductsPage(data, lang, dir))
}

// AdminSavingProductSearchJSON returns JSON autocomplete list of catalog products for admin linking.
// Runs cross-tenant under database.AsSystem, matching WO-12 step 1.
func (h *UIHandler) AdminSavingProductSearchJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	rawName := strings.TrimSpace(r.URL.Query().Get("raw_name"))
	if q == "" && rawName != "" {
		q = rawName
	}

	if len(q) < 2 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}

	type searchResult struct {
		ID             string  `json:"id"`
		Name           string  `json:"name"`
		Label          string  `json:"label"`
		NameAR         string  `json:"name_ar,omitempty"`
		NameEN         string  `json:"name_en,omitempty"`
		Hint           string  `json:"hint,omitempty"`
		Badge          string  `json:"badge,omitempty"`
		Score          float64 `json:"score"`
		SKU            string  `json:"sku,omitempty"`
		Price          string  `json:"price,omitempty"`
		DosageForm     string  `json:"dosage_form,omitempty"`
		ScientificName string  `json:"scientific_name,omitempty"`
	}

	var results []searchResult
	if h.catSvc != nil {
		products, err := h.catSvc.Search(database.AsSystem(ctx), catalog.SearchParams{
			Query:     q,
			FirstWord: catalog.FirstWordOf(q),
			Limit:     20,
		})
		if err == nil {
			targetName := rawName
			if targetName == "" {
				targetName = q
			}
			for _, p := range products {
				nameAR := p.Name.Get(i18n.AR)
				nameEN := p.Name.Get(i18n.EN)
				label := nameAR
				if label == "" {
					label = nameEN
				}

				score := calculateSavingMatchScore(targetName, p)
				hint := p.SKU
				if p.DosageForm != "" {
					if hint != "" {
						hint = p.DosageForm + " | " + hint
					} else {
						hint = p.DosageForm
					}
				}

				badge := fmt.Sprintf("%.0f%% تطابق", score*100)

				results = append(results, searchResult{
					ID:             fmt.Sprintf("%d", p.ID),
					Name:           label,
					Label:          label,
					NameAR:         nameAR,
					NameEN:         nameEN,
					Hint:           hint,
					Badge:          badge,
					Score:          score,
					SKU:            p.SKU,
					Price:          p.Price.String(),
					DosageForm:     p.DosageForm,
					ScientificName: p.ScientificName,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(results)
}

// AdminSavingProductLinkSubmit links or unlinks a saving product to a platform master catalog product.
// Preserves all query context parameters (page, q, filter, org_id, user_id) on redirect (B1).
// Also writes the decision to shared memory (WO-12 step 4 / WO-13).
func (h *UIHandler) AdminSavingProductLinkSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/saving-products", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectAdminSavingProducts(w, r, "error", i18n.T(lang, "admin.saving.not_found"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectAdminSavingProducts(w, r, "error", i18n.T(lang, "common.form_invalid"))
		return
	}

	prodStr := strings.TrimSpace(r.FormValue("product_id"))
	if prodStr == "" {
		prodStr = strings.TrimSpace(r.FormValue("combobox_product_id"))
	}

	var productID int64
	if prodStr != "" {
		if pid, err := strconv.ParseInt(prodStr, 10, 64); err == nil && pid > 0 {
			productID = pid
		}
	}

	sp, err := h.catSvc.GetSavingProduct(database.AsSystem(ctx), id)
	if err != nil || sp == nil {
		h.log.ErrorContext(ctx, "admin get saving product for link error", "error", err, "id", id)
		h.redirectAdminSavingProducts(w, r, "error", i18n.T(lang, "admin.saving.not_found"))
		return
	}

	if productID > 0 {
		sp.ProductID = &productID
		if err := h.catSvc.UpdateSavingProduct(database.AsSystem(ctx), sp); err != nil {
			h.log.ErrorContext(ctx, "admin update saving product link error", "error", err, "id", id)
			h.redirectAdminSavingProducts(w, r, "error", fmt.Sprintf(i18n.T(lang, "admin.saving.link_error"), h.safeMessage(err, lang)))
			return
		}

		// Write decision to shared memory (catalog.match_decisions + customer_product_mappings)
		if sp.OrganizationID > 0 {
			_ = h.catSvc.SaveManualDecision(ctx, sp.OrganizationID, actor.UserID, sp.NameProduct, productID, "admin_manual_link")
		}
		if h.matchMemory != nil {
			_ = h.matchMemory.SaveAlias(ctx, productID, sp.NameProduct, "manual", 1.0)
		}

		h.redirectAdminSavingProducts(w, r, "success", i18n.T(lang, "admin.saving.link_success"))
		return
	}

	// Unlink action
	sp.ProductID = nil
	if err := h.catSvc.UpdateSavingProduct(database.AsSystem(ctx), sp); err != nil {
		h.log.ErrorContext(ctx, "admin unlink saving product error", "error", err, "id", id)
		h.redirectAdminSavingProducts(w, r, "error", fmt.Sprintf(i18n.T(lang, "admin.saving.link_error"), h.safeMessage(err, lang)))
		return
	}

	h.redirectAdminSavingProducts(w, r, "success", i18n.T(lang, "admin.saving.unlink_success"))
}

// redirectAdminSavingProducts redirects to /admin/saving-products preserving filter, pagination and search query (B1).
func (h *UIHandler) redirectAdminSavingProducts(w http.ResponseWriter, r *http.Request, noticeType, noticeMsg string) {
	q := url.Values{}

	copyField := func(k string) {
		val := strings.TrimSpace(r.FormValue(k))
		if val == "" {
			val = strings.TrimSpace(r.URL.Query().Get(k))
		}
		if val != "" {
			q.Set(k, val)
		}
	}

	copyField("page")
	copyField("limit")
	copyField("q")
	copyField("filter")
	copyField("org_id")
	copyField("user_id")

	if noticeType != "" && noticeMsg != "" {
		q.Set("notice_type", noticeType)
		q.Set("notice", noticeMsg)
	}

	target := "/admin/saving-products"
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
