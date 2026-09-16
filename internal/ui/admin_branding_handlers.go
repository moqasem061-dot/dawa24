package ui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/media"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AdminSiteSettingsSubmit persists public contact info and social media links.
func (h *UIHandler) AdminSiteSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", i18n.T(lang, "admin.settings.service_unavailable"))
		return
	}

	curr, _ := h.adminSvc.GetSiteSettings(ctx)
	if curr == nil {
		curr = &platformadmin.SiteSettings{SocialLinks: map[string]string{}}
	}
	if curr.SocialLinks == nil {
		curr.SocialLinks = map[string]string{}
	}

	section := r.FormValue("section")
	if section == "contact" || section == "" {
		if name := strings.TrimSpace(r.FormValue("site_name")); name != "" {
			curr.SiteName = name
		}
		if desc := strings.TrimSpace(r.FormValue("site_description")); desc != "" || r.Form.Has("site_description") {
			curr.SiteDescription = desc
		}
		if email := strings.TrimSpace(r.FormValue("contact_email")); email != "" || r.Form.Has("contact_email") {
			curr.ContactEmail = email
		}
		if supEmail := strings.TrimSpace(r.FormValue("support_email")); supEmail != "" || r.Form.Has("support_email") {
			curr.SupportEmail = supEmail
		}
		if phone := strings.TrimSpace(r.FormValue("phone")); phone != "" || r.Form.Has("phone") {
			curr.Phone = phone
		}
		if wa := strings.TrimSpace(r.FormValue("whatsapp")); wa != "" || r.Form.Has("whatsapp") {
			curr.WhatsApp = wa
		}
		if addr := strings.TrimSpace(r.FormValue("address")); addr != "" || r.Form.Has("address") {
			curr.Address = addr
		}
		if logo := strings.TrimSpace(r.FormValue("logo_url")); logo != "" {
			curr.LogoURL = logo
		}
		if fav := strings.TrimSpace(r.FormValue("favicon_url")); fav != "" {
			curr.FaviconURL = fav
		}
	} else if section == "socials" {
		curr.SocialLinks["facebook"] = strings.TrimSpace(r.FormValue("social_facebook"))
		curr.SocialLinks["twitter"] = strings.TrimSpace(r.FormValue("social_twitter"))
		curr.SocialLinks["instagram"] = strings.TrimSpace(r.FormValue("social_instagram"))
		curr.SocialLinks["linkedin"] = strings.TrimSpace(r.FormValue("social_linkedin"))
		curr.SocialLinks["youtube"] = strings.TrimSpace(r.FormValue("social_youtube"))
		curr.SocialLinks["tiktok"] = strings.TrimSpace(r.FormValue("social_tiktok"))
		curr.SocialLinks["snapchat"] = strings.TrimSpace(r.FormValue("social_snapchat"))
		curr.SocialLinks["telegram"] = strings.TrimSpace(r.FormValue("social_telegram"))
		if curr.WhatsApp != "" {
			curr.SocialLinks["whatsapp"] = "https://wa.me/" + curr.WhatsApp
		}
	}

	if err := h.adminSvc.SaveSiteSettings(ctx, curr); err != nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", h.safeMessage(err, lang))
		return
	}

	InvalidateSiteSettingsCache()
	h.redirectWithNotice(w, r, "/admin/settings?tab=site", "success", i18n.T(lang, "admin.branding.site_saved_success"))
}

// uploadBrandingFile processes a multipart file upload and saves it to storage or local directory.
func (h *UIHandler) uploadBrandingFile(ctx context.Context, r *http.Request, fieldName, prefix string) (string, error) {
	file, header, err := r.FormFile(fieldName)
	if err != nil || file == nil {
		return "", err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil || len(data) == 0 {
		return "", err
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		ext = ".png"
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		switch ext {
		case ".svg":
			contentType = "image/svg+xml"
		case ".ico":
			contentType = "image/x-icon"
		case ".webp":
			contentType = "image/webp"
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".png":
			contentType = "image/png"
		default:
			contentType = "image/png"
		}
	}

	// Optimize and compress raster images (keep SVG, ICO untouched)
	if ext != ".svg" && ext != ".ico" {
		if compBytes, compExt, compCT, wasCompressed := media.Compress(data, media.DefaultMaxEdge); wasCompressed {
			data = compBytes
			ext = compExt
			contentType = compCT
		}
	}

	key := fmt.Sprintf("branding/%s_%d%s", prefix, time.Now().UnixNano(), ext)

	if h.storage != nil {
		if err := h.storage.Put(ctx, key, bytes.NewReader(data), int64(len(data)), contentType); err != nil {
			h.log.WarnContext(ctx, "branding: upload to storage failed", "prefix", prefix, "error", err)
		} else {
			pubURL := h.storage.PublicURL(key)
			if pubURL == "" {
				pubURL = "/uploads/" + key
			}
			return pubURL, nil
		}
	}

	savePath := filepath.Join(GetUploadBaseDir(), "branding", fmt.Sprintf("%s_%d%s", prefix, time.Now().UnixNano(), ext))
	if err := os.MkdirAll(filepath.Dir(savePath), 0o755); err != nil {
		h.log.WarnContext(ctx, "branding: fallback mkdir failed", "error", err)
		return "", err
	}
	if err := os.WriteFile(savePath, data, 0644); err != nil {
		h.log.WarnContext(ctx, "branding: fallback write failed", "error", err)
		return "", err
	}

	return "/uploads/branding/" + filepath.Base(savePath), nil
}

// AdminBrandingSubmit updates platform logos (Light and Dark) and favicon.
func (h *UIHandler) AdminBrandingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", i18n.T(lang, "admin.settings.service_unavailable"))
		return
	}

	curr, _ := h.adminSvc.GetSiteSettings(ctx)
	if curr == nil {
		curr = &platformadmin.SiteSettings{}
	}

	_ = r.ParseMultipartForm(uploadMemoryBudget)

	if name := strings.TrimSpace(r.FormValue("site_name")); name != "" {
		curr.SiteName = name
	}

	// 1. Light Mode Logo Upload
	if url, err := h.uploadBrandingFile(ctx, r, "logo_file", "logo_light"); err == nil && url != "" {
		curr.LogoURL = url
	} else if url, err := h.uploadBrandingFile(ctx, r, "logo_light_file", "logo_light"); err == nil && url != "" {
		curr.LogoURL = url
	} else if directURL := strings.TrimSpace(r.FormValue("logo_url")); directURL != "" {
		curr.LogoURL = directURL
	}

	// 2. Dark Mode Logo Upload / Removal
	if r.FormValue("remove_dark_logo") == "1" {
		curr.LogoDarkURL = ""
	} else if url, err := h.uploadBrandingFile(ctx, r, "logo_dark_file", "logo_dark"); err == nil && url != "" {
		curr.LogoDarkURL = url
	} else if directURL := strings.TrimSpace(r.FormValue("logo_dark_url")); directURL != "" {
		curr.LogoDarkURL = directURL
	}

	// 3. Favicon Upload
	if url, err := h.uploadBrandingFile(ctx, r, "favicon_file", "favicon"); err == nil && url != "" {
		curr.FaviconURL = url
	} else if directURL := strings.TrimSpace(r.FormValue("favicon_url")); directURL != "" {
		curr.FaviconURL = directURL
	}

	if err := h.adminSvc.SaveSiteSettings(ctx, curr); err != nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", h.safeMessage(err, lang))
		return
	}

	InvalidateSiteSettingsCache()
	h.redirectWithNotice(w, r, "/admin/settings?tab=site", "success", i18n.T(lang, "admin.branding.branding_saved_success"))
}
