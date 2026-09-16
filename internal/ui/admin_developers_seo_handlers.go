package ui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) buildSEOValues(r *http.Request) *pages.AdminDevelopersSEOValues {
	ctx := database.AsSystem(r.Context())
	q := strings.TrimSpace(r.URL.Query().Get("seo_q"))
	page, _ := strconv.Atoi(r.URL.Query().Get("seo_page"))
	if page <= 0 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	val := &pages.AdminDevelopersSEOValues{
		SearchQuery: q,
		Page:        page,
		PerPage:     limit,
		NoticeMsg:   strings.TrimSpace(r.URL.Query().Get("notice")),
		NoticeType:  strings.TrimSpace(r.URL.Query().Get("notice_type")),
	}

	if h.adminSvc != nil {
		filter := platformadmin.SEOPagesFilter{
			Search: q,
			Limit:  limit,
			Offset: offset,
		}
		pgs, total, err := h.adminSvc.ListSEOPages(ctx, filter)
		if err == nil {
			val.Pages = pgs
			val.TotalPages = total
		}

		if sets, err := h.adminSvc.GetSEOSettings(ctx); err == nil && sets != nil {
			val.RobotsTxt = sets.RobotsTxt
		}
	}

	return val
}

// AdminDevelopersSEOSaveSubmit handles saving an updated or new SEO page configuration.
func (h *UIHandler) AdminDevelopersSEOSaveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=seo", "error", "بيانات النموذج غير صالحة")
		return
	}

	routePattern := strings.TrimSpace(r.FormValue("route_pattern"))
	if routePattern == "" {
		routePattern = "/"
	}
	if !strings.HasPrefix(routePattern, "/") {
		routePattern = "/" + routePattern
	}

	var kw []string
	rawKW := strings.Split(r.FormValue("keywords"), ",")
	for _, k := range rawKW {
		trimmed := strings.TrimSpace(k)
		if trimmed != "" {
			kw = append(kw, trimmed)
		}
	}

	page := &platformadmin.SEOPage{
		RoutePattern:     routePattern,
		TitleAR:          strings.TrimSpace(r.FormValue("title_ar")),
		TitleEN:          strings.TrimSpace(r.FormValue("title_en")),
		MetaDescAR:       strings.TrimSpace(r.FormValue("meta_desc_ar")),
		MetaDescEN:       strings.TrimSpace(r.FormValue("meta_desc_en")),
		CanonicalURL:     strings.TrimSpace(r.FormValue("canonical_url")),
		RobotsDirectives: strings.TrimSpace(r.FormValue("robots_directives")),
		OGTitle:          strings.TrimSpace(r.FormValue("og_title")),
		OGDesc:           strings.TrimSpace(r.FormValue("og_desc")),
		OGImage:          strings.TrimSpace(r.FormValue("og_image")),
		TwitterCard:      strings.TrimSpace(r.FormValue("twitter_card")),
		Keywords:         kw,
		IsPublic:         r.FormValue("is_public") != "false",
		ChangeFreq:       "monthly",
		Priority:         0.70,
	}

	if page.RobotsDirectives == "" {
		page.RobotsDirectives = "index, follow"
	}
	if page.TwitterCard == "" {
		page.TwitterCard = "summary_large_image"
	}
	if page.OGImage == "" {
		page.OGImage = "/static/img/og-default.jpg"
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.UpsertSEOPage(ctx, page); err != nil {
			h.redirectWithNotice(w, r, "/admin/developers?tab=seo", "error", "فشل حفظ إعدادات الـ SEO: "+err.Error())
			return
		}
	}

	if isJSONOrAJAX(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "route": routePattern})
		return
	}

	h.redirectWithNotice(w, r, "/admin/developers?tab=seo", "success", "تم حفظ إعدادات الـ SEO للمسار بنجاح")
}

// AdminDevelopersSEORobotsSubmit handles updates to live robots.txt content.
func (h *UIHandler) AdminDevelopersSEORobotsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=seo", "error", "بيانات غير صالحة")
		return
	}

	robotsTxt := r.FormValue("robots_txt")
	if strings.TrimSpace(robotsTxt) == "" {
		h.redirectWithNotice(w, r, "/admin/developers?tab=seo", "error", "لا يمكن حفظ ملف robots.txt فارغاً")
		return
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.UpdateSEORobotsTxt(ctx, robotsTxt); err != nil {
			h.redirectWithNotice(w, r, "/admin/developers?tab=seo", "error", "فشل تحديث ملف robots.txt: "+err.Error())
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/developers?tab=seo", "success", "تم تحديث ملف robots.txt بنجاح")
}
