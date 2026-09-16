package ui

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// DocumentViewHandler streams or redirects to the document file for in-browser viewing / preview.
func (h *UIHandler) DocumentViewHandler(w http.ResponseWriter, r *http.Request) {
	h.serveDocumentFile(w, r, false)
}

// DocumentDownloadHandler forces downloading the document file.
func (h *UIHandler) DocumentDownloadHandler(w http.ResponseWriter, r *http.Request) {
	h.serveDocumentFile(w, r, true)
}

// serveDocumentFile safely finds, verifies access to, and streams a document file.
func (h *UIHandler) serveDocumentFile(w http.ResponseWriter, r *http.Request, download bool) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, i18n.T(lang, "docs.serve.auth_required"), http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, i18n.T(lang, "docs.serve.invalid_id"), http.StatusBadRequest)
		return
	}

	if h.attSvc == nil {
		http.Error(w, i18n.T(lang, "docs.serve.service_unavailable"), http.StatusServiceUnavailable)
		return
	}

	sysCtx := database.AsSystem(ctx)
	doc, err := h.attSvc.GetByIDAdmin(sysCtx, id)
	if err != nil || doc == nil {
		h.renderMissingDocError(w, r, nil, i18n.T(lang, "docs.serve.not_found"), lang, dir)
		return
	}

	// Verify tenant authorization if not platform admin/staff
	if !actor.IsPlatformAdmin() && !actor.IsStaff {
		orgID := actor.OrganizationID
		if orgID <= 0 {
			orgID = actor.OrgID
		}
		hasAccess := false
		if doc.OrganizationID != nil && orgID > 0 && *doc.OrganizationID == orgID {
			hasAccess = true
		}
		if doc.UserID != nil && actor.UserID > 0 && *doc.UserID == actor.UserID {
			hasAccess = true
		}
		if !hasAccess {
			http.Error(w, i18n.T(lang, "docs.serve.access_forbidden"), http.StatusForbidden)
			return
		}
	}

	rawURL := strings.TrimSpace(doc.FileURL)
	if rawURL == "" {
		rawURL = strings.TrimSpace(doc.StorageKey)
	}

	// 1. Sanitize the URL / Storage Key (NEVER redirect to localhost / private network endpoints)
	cleanKey := rawURL
	if strings.Contains(cleanKey, "://") {
		if u, parseErr := url.Parse(cleanKey); parseErr == nil {
			host := strings.ToLower(u.Hostname())
			if host == "localhost" || host == "127.0.0.1" || host == "minio" || host == "0.0.0.0" ||
				strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "10.") || strings.HasPrefix(host, "172.") {
				p := u.Path
				p = strings.TrimPrefix(p, "/dawa24")
				cleanKey = p
			}
		}
	}

	for strings.HasPrefix(cleanKey, "/") {
		cleanKey = strings.TrimPrefix(cleanKey, "/")
	}
	cleanPath := strings.TrimPrefix(cleanKey, "uploads/")
	for strings.HasPrefix(cleanPath, "/") {
		cleanPath = strings.TrimPrefix(cleanPath, "/")
	}
	baseName := filepath.Base(cleanKey)

	// 2. Check all local disk locations
	candidates := []string{
		filepath.Join(UploadBaseDir, cleanPath),
		filepath.Join(UploadBaseDir, cleanKey),
		filepath.Join(UploadBaseDir, "documents", baseName),
		filepath.Join(UploadBaseDir, "licenses", baseName),
		filepath.Join("data", "uploads", cleanPath),
		filepath.Join("data", "uploads", cleanKey),
		filepath.Join("data", "uploads", "documents", baseName),
		filepath.Join("data", "uploads", "licenses", baseName),
		filepath.Join("internal", "ui", "data", "uploads", cleanPath),
		filepath.Join("internal", "ui", "data", "uploads", "documents", baseName),
		filepath.Join("cmd", "server", "data", "uploads", cleanPath),
		filepath.Join("cmd", "server", "data", "uploads", "documents", baseName),
		filepath.Join("data", cleanPath),
		filepath.Join("data", cleanKey),
		cleanKey,
		cleanPath,
	}

	for _, cand := range candidates {
		if cand == "" || cand == "." || cand == "/" {
			continue
		}
		info, statErr := os.Stat(cand)
		if statErr == nil && !info.IsDir() {
			f, openErr := os.Open(cand)
			if openErr == nil {
				defer f.Close()

				mimeType := doc.MimeType
				ext := strings.ToLower(filepath.Ext(cand))
				if mimeType == "" || mimeType == "application/octet-stream" {
					switch ext {
					case ".pdf":
						mimeType = "application/pdf"
					case ".png":
						mimeType = "image/png"
					case ".jpg", ".jpeg":
						mimeType = "image/jpeg"
					case ".webp":
						mimeType = "image/webp"
					case ".svg":
						mimeType = "image/svg+xml"
					default:
						mimeType = "application/pdf"
					}
				}

				filename := doc.OriginalName
				if filename == "" {
					filename = filepath.Base(cand)
				}

				disposition := "inline"
				if download {
					disposition = "attachment"
				}

				w.Header().Set("Content-Type", mimeType)
				w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
				w.Header().Set("Cache-Control", "private, max-age=3600")
				http.ServeContent(w, r, filename, info.ModTime(), f)
				return
			}
		}
	}

	// 3. Storage client (S3/MinIO) direct proxy stream (fetching object directly from storage without unroutable redirects)
	if h.storage != nil {
		storageKeys := []string{
			cleanKey,
			cleanPath,
			doc.StorageKey,
			fmt.Sprintf("documents/%s", baseName),
			fmt.Sprintf("uploads/documents/%s", baseName),
			fmt.Sprintf("licenses/%s", baseName),
			fmt.Sprintf("uploads/%s", cleanPath),
		}
		if doc.OrganizationID != nil && *doc.OrganizationID > 0 {
			storageKeys = append(storageKeys,
				fmt.Sprintf("orgs/%d/%s", *doc.OrganizationID, cleanPath),
				fmt.Sprintf("orgs/%d/documents/%s", *doc.OrganizationID, baseName),
			)
		}

		for _, sKey := range storageKeys {
			if sKey == "" {
				continue
			}
			body, cType, sErr := h.storage.Get(ctx, sKey)
			if sErr == nil && body != nil {
				defer body.Close()

				mimeType := cType
				if mimeType == "" || mimeType == "application/octet-stream" {
					mimeType = doc.MimeType
				}
				if mimeType == "" || mimeType == "application/octet-stream" {
					ext := strings.ToLower(filepath.Ext(baseName))
					switch ext {
					case ".pdf":
						mimeType = "application/pdf"
					case ".png":
						mimeType = "image/png"
					case ".jpg", ".jpeg":
						mimeType = "image/jpeg"
					case ".webp":
						mimeType = "image/webp"
					case ".svg":
						mimeType = "image/svg+xml"
					default:
						mimeType = "application/pdf"
					}
				}

				filename := doc.OriginalName
				if filename == "" {
					filename = baseName
				}

				disposition := "inline"
				if download {
					disposition = "attachment"
				}

				w.Header().Set("Content-Type", mimeType)
				w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
				w.Header().Set("Cache-Control", "private, max-age=3600")
				_, _ = io.Copy(w, body)
				return
			}
		}
	}

	// 4. If file is unavailable anywhere, render the clear, polished Document Unavailable Error Page
	h.renderMissingDocError(w, r, doc, i18n.T(lang, "docs.serve.file_missing"), lang, dir)
}
