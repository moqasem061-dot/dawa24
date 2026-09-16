package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// renderCmsBlock renders a database-driven content page by block key.
func (h *UIHandler) renderCmsBlock(w http.ResponseWriter, r *http.Request, key, fallbackTitle string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	title, body := fallbackTitle, ""
	if h.adminSvc != nil {
		if b, err := h.adminSvc.GetContentBlockByKey(ctx, key); err == nil && b != nil && b.IsActive {
			title = b.Title.Get(i18n.Lang(lang))
			body = b.Body.Get(i18n.Lang(lang))
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CmsPage(lang, dir, title, body).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render cms page", "key", key, "error", err)
	}
}

// AboutPage renders the interactive About Us page with database-driven content.
func (h *UIHandler) AboutPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	data := pages.AboutPageData{}
	if h.adminSvc != nil {
		if b, _ := h.adminSvc.GetContentBlockByKey(ctx, "about"); b != nil && b.IsActive {
			data.HeroTitle = b.Title.Get(i18n.Lang(lang))
			data.HeroSubtitle = b.Body.Get(i18n.Lang(lang))
		}
		if b, _ := h.adminSvc.GetContentBlockByKey(ctx, "about-hero"); b != nil && b.IsActive {
			if data.HeroTitle == "" {
				data.HeroTitle = b.Title.Get(i18n.Lang(lang))
			}
			if data.HeroSubtitle == "" {
				data.HeroSubtitle = b.Body.Get(i18n.Lang(lang))
			}
		}
		if b, _ := h.adminSvc.GetContentBlockByKey(ctx, "about-vision"); b != nil && b.IsActive {
			data.VisionTitle = b.Title.Get(i18n.Lang(lang))
			data.VisionText = b.Body.Get(i18n.Lang(lang))
		}
		if b, _ := h.adminSvc.GetContentBlockByKey(ctx, "about-mission"); b != nil && b.IsActive {
			data.MissionTitle = b.Title.Get(i18n.Lang(lang))
			data.MissionText = b.Body.Get(i18n.Lang(lang))
		}
		if b, _ := h.adminSvc.GetContentBlockByKey(ctx, "about-banner"); b != nil && b.IsActive {
			data.BannerTitle = b.Title.Get(i18n.Lang(lang))
			data.BannerText = b.Body.Get(i18n.Lang(lang))
		}
	}

	h.renderPage(ctx, w, "render about page", pages.AboutPage(lang, dir, data))
}

// HowItWorksPage renders the dedicated interactive How It Works page.
func (h *UIHandler) HowItWorksPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	h.renderPage(ctx, w, "render how it works page", pages.HowItWorksPage(lang, dir))
}

// FaqPage renders the dedicated interactive FAQ page with dynamic database overrides.
func (h *UIHandler) FaqPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	customTitle, customSubtitle := "", ""
	if h.adminSvc != nil {
		if b, _ := h.adminSvc.GetContentBlockByKey(ctx, "faq"); b != nil && b.IsActive {
			customTitle = b.Title.Get(i18n.Lang(lang))
			customSubtitle = b.Body.Get(i18n.Lang(lang))
		}
	}

	h.renderPage(ctx, w, "render faq page", pages.FAQPage(lang, dir, customTitle, customSubtitle))
}

// renderPolicy renders a published legal document by slug.
func (h *UIHandler) renderPolicy(w http.ResponseWriter, r *http.Request, slug, fallbackTitle string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	title, body := fallbackTitle, ""
	var version string
	var publishedAt string
	lookupKey := slug
	switch slug {
	case "shipping-returns", "refund":
		lookupKey = "shipping_return"
	case "payment-policy", "payments":
		lookupKey = "payment"
	case "cookie":
		lookupKey = "cookies"
	}

	if h.adminSvc != nil {
		p, err := h.adminSvc.GetActivePolicy(ctx, lookupKey)
		if err != nil && (lookupKey == "shipping_return") {
			// Fallback check for legacy refund key
			p, err = h.adminSvc.GetActivePolicy(ctx, "refund")
		}
		if err == nil && p != nil {
			if t := p.Title.Get(i18n.Lang(lang)); t != "" {
				title = t
			} else if t := p.Title.Get("ar"); t != "" {
				title = t
			}
			if c := p.Content.Get(i18n.Lang(lang)); c != "" {
				body = c
			} else if c := p.Content.Get("ar"); c != "" {
				body = c
			}
			version = p.Version
			if p.PublishedAt != nil {
				publishedAt = p.PublishedAt.Format("2006-01-02")
			} else if !p.UpdatedAt.IsZero() {
				publishedAt = p.UpdatedAt.Format("2006-01-02")
			}
		}
	}

	if body == "" {
		switch lookupKey {
		case "privacy":
			title = "سياسة الخصوصية وحماية البيانات"
			body = "تلتزم منصة Dawa24 بحماية سرية البيانات التجارية والطبية لكافة المنشآت والصيدليات المسجلة طبقاً للقانون المصري رقم 151 لسنة 2020."
		case "terms":
			title = "شروط وأحكام استخدام منصة Dawa24"
			body = "شروط وأحكام استخدام منصة Dawa24 لربط الصيدليات بالموردين والمستودعات المعتمدة في جمهورية مصر العربية."
		case "shipping_return":
			title = "سياسة الشحن والتسليم والاسترجاع والإلغاء"
			body = "الضوابط المنظمة لعمليات الشحن، النقل الجيد، إجراءات الاستلام وفحص الشحنات، وسياسة المرتجعات الدوائية وإلغاء الطلبات."
		case "cookies":
			title = "سياسة ملفات تعريف الارتباط (Cookies)"
			body = "بيان استخدام ملفات تعريف الارتباط وتقنيات حفظ الجلسات الآمنة، ومهلة الخمول التلقائي لحماية الحسابات."
		case "payment":
			title = "سياسة الدفع والتعاملات المالية"
			body = "الضوابط المنظمة للمدفوعات المباشرة بين الصيدليات والموردين، وطرق الدفع والائتمان المعتمدة، وإصدار الفواتير الإلكترونية."
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.PolicyPage(lang, dir, title, body, slug, version, publishedAt).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render policy page", "slug", slug, "error", err)
	}
}

// AdminContentPage renders the comprehensive CMS content blocks and highlight sections editor.
func (h *UIHandler) AdminContentPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var blocks []*platformadmin.ContentBlock
	var total int
	if h.adminSvc != nil {
		blocks, total, _ = h.adminSvc.ListContentBlocksWithTotal(database.AsSystem(ctx), limit, offset)
	}

	h.renderPage(ctx, w, "render admin content", pages.AdminContent(lang, dir, blocks, page, limit, total))
}

// AdminContentSubmit creates or updates a CMS block or highlight section.
func (h *UIHandler) AdminContentSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/content", "error", i18n.T(lang, "admin.content.service_unavailable"))
		return
	}

	_ = r.ParseForm()

	key := strings.TrimSpace(r.FormValue("key"))
	if key == "" {
		h.redirectWithNotice(w, r, "/admin/content", "error", i18n.T(lang, "admin.content.key_required"))
		return
	}

	pos := strings.TrimSpace(r.FormValue("position"))
	if pos == "" {
		pos = "page"
	}

	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	isActive := r.FormValue("is_active") != "false"

	block := &platformadmin.ContentBlock{
		Key:       key,
		Title:     i18n.New(strings.TrimSpace(r.FormValue("title_ar")), strings.TrimSpace(r.FormValue("title_en"))),
		Body:      i18n.New(strings.TrimSpace(r.FormValue("body_ar")), strings.TrimSpace(r.FormValue("body_en"))),
		Position:  pos,
		SortOrder: sortOrder,
		IsActive:  isActive,
	}

	if err := h.adminSvc.UpsertContentBlock(database.AsSystem(ctx), block); err != nil {
		h.log.ErrorContext(ctx, "admin upsert content block failed", "key", key, "error", err)
		h.redirectWithNotice(w, r, "/admin/content", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/content", "success", i18n.T(lang, "admin.content.saved_success"))
}

// AdminContentToggleSubmit toggles the active status of a content block.
func (h *UIHandler) AdminContentToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/content", "error", i18n.T(lang, "admin.content.invalid_id"))
		return
	}

	if err := h.adminSvc.ToggleContentBlockStatus(database.AsSystem(ctx), id); err != nil {
		h.log.ErrorContext(ctx, "admin toggle content block failed", "id", id, "error", err)
		h.redirectWithNotice(w, r, "/admin/content", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/content", "success", i18n.T(lang, "admin.content.status_updated_success"))
}

// AdminContentDeleteSubmit deletes a content block.
func (h *UIHandler) AdminContentDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/content", "error", i18n.T(lang, "admin.content.invalid_id"))
		return
	}

	if err := h.adminSvc.DeleteContentBlock(database.AsSystem(ctx), id); err != nil {
		h.log.ErrorContext(ctx, "admin delete content block failed", "id", id, "error", err)
		h.redirectWithNotice(w, r, "/admin/content", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/content", "success", i18n.T(lang, "admin.content.deleted_success"))
}
