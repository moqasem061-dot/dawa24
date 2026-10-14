package ui

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
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
	if !actor.IsPlatformAdmin() && (!actor.IsStaff || !actor.Can("hr.document.view")) {
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
	if rawURL == "" {
		rawURL = strings.TrimSpace(doc.OriginalName)
	}

	// 1. Sanitize the URL / Storage Key (extract clean path from any absolute URL)
	cleanKey := rawURL
	if strings.Contains(cleanKey, "://") {
		if u, parseErr := url.Parse(cleanKey); parseErr == nil {
			p := u.Path
			p = strings.TrimPrefix(p, "/dawa24")
			cleanKey = p
		}
	}

	for strings.HasPrefix(cleanKey, "/") {
		cleanKey = strings.TrimPrefix(cleanKey, "/")
	}
	cleanPath := strings.TrimPrefix(cleanKey, "uploads/")
	for strings.HasPrefix(cleanPath, "uploads/") {
		cleanPath = strings.TrimPrefix(cleanPath, "uploads/")
	}
	for strings.HasPrefix(cleanPath, "/") {
		cleanPath = strings.TrimPrefix(cleanPath, "/")
	}
	baseName := filepath.Base(cleanKey)

	origName := strings.TrimSpace(doc.OriginalName)
	for strings.HasPrefix(origName, "/") {
		origName = strings.TrimPrefix(origName, "/")
	}
	origBaseName := filepath.Base(origName)
	storageKeyClean := strings.TrimPrefix(strings.TrimSpace(doc.StorageKey), "/")
	if !documentStorageKeyAllowed(doc, cleanPath) {
		h.renderMissingDocError(w, r, doc, i18n.T(lang, "docs.serve.file_missing"), lang, dir)
		return
	}
	scopedKey := ""
	if strings.HasPrefix(cleanPath, "orgs/") || strings.HasPrefix(cleanPath, "users/") {
		scopedKey = cleanPath
	}

	// 2. Check all local disk locations respecting UPLOAD_DIR and DATA_DIR
	baseDir := GetUploadBaseDir()
	candidates := []string{
		filepath.Join(baseDir, cleanPath),
		filepath.Join(baseDir, cleanKey),
		filepath.Join(baseDir, "documents", baseName),
		filepath.Join(baseDir, "licenses", baseName),
		filepath.Join(baseDir, string(doc.DocumentType), baseName),
		filepath.Join(baseDir, baseName),
		filepath.Join("data", "uploads", cleanPath),
		filepath.Join("data", "uploads", cleanKey),
		filepath.Join("data", "uploads", "documents", baseName),
		filepath.Join("data", "uploads", "licenses", baseName),
		filepath.Join("data", "uploads", string(doc.DocumentType), baseName),
		filepath.Join("internal", "ui", "data", "uploads", cleanPath),
		filepath.Join("internal", "ui", "data", "uploads", "documents", baseName),
		filepath.Join("internal", "ui", "data", "uploads", "licenses", baseName),
		filepath.Join("cmd", "server", "data", "uploads", cleanPath),
		filepath.Join("cmd", "server", "data", "uploads", "documents", baseName),
		filepath.Join("cmd", "server", "data", "uploads", "licenses", baseName),
		filepath.Join("data", cleanPath),
		filepath.Join("data", cleanKey),
		cleanKey,
		cleanPath,
	}

	if origName != "" {
		candidates = append(candidates,
			filepath.Join(baseDir, origName),
			filepath.Join(baseDir, "documents", origName),
			filepath.Join(baseDir, "licenses", origName),
			filepath.Join(baseDir, string(doc.DocumentType), origName),
			filepath.Join(baseDir, origBaseName),
			filepath.Join(baseDir, "documents", origBaseName),
			filepath.Join(baseDir, "licenses", origBaseName),
			filepath.Join("data", "uploads", origName),
			filepath.Join("data", "uploads", "documents", origName),
			filepath.Join("data", "uploads", "licenses", origName),
			filepath.Join("data", "uploads", origBaseName),
			filepath.Join("data", "uploads", "documents", origBaseName),
			filepath.Join("data", "uploads", "licenses", origBaseName),
			filepath.Join("internal", "ui", "data", "uploads", origName),
			filepath.Join("internal", "ui", "data", "uploads", "documents", origName),
			filepath.Join("cmd", "server", "data", "uploads", origName),
			filepath.Join("cmd", "server", "data", "uploads", "documents", origName),
			origName,
		)
	}

	if storageKeyClean != "" && storageKeyClean != cleanKey {
		candidates = append(candidates,
			filepath.Join(baseDir, storageKeyClean),
			filepath.Join(baseDir, "documents", filepath.Base(storageKeyClean)),
			filepath.Join(baseDir, "licenses", filepath.Base(storageKeyClean)),
			filepath.Join("data", "uploads", storageKeyClean),
			storageKeyClean,
		)
	}

	if doc.OrganizationID != nil && *doc.OrganizationID > 0 {
		orgPfx := fmt.Sprintf("orgs/%d", *doc.OrganizationID)
		candidates = append(candidates,
			filepath.Join(baseDir, orgPfx, cleanPath),
			filepath.Join(baseDir, orgPfx, baseName),
			filepath.Join(baseDir, orgPfx, string(doc.DocumentType), baseName),
			filepath.Join("data", "uploads", orgPfx, cleanPath),
			filepath.Join("data", "uploads", orgPfx, baseName),
		)
		if origName != "" {
			candidates = append(candidates,
				filepath.Join(baseDir, orgPfx, origName),
				filepath.Join(baseDir, orgPfx, string(doc.DocumentType), origName),
				filepath.Join(baseDir, orgPfx, origBaseName),
				filepath.Join("data", "uploads", orgPfx, origName),
			)
		}
	}

	// Namespaced keys are minted for one tenant/user. A basename or original
	// filename fallback would let caller-supplied metadata alias legacy private files.
	if scopedKey != "" {
		candidates = scopedDocumentPaths(scopedKey)
	}
	for _, cand := range candidates {
		if cand == "" || cand == "." || cand == "/" {
			continue
		}
		f, openErr := openDocumentUpload(cand)
		if openErr == nil {
			info, statErr := f.Stat()
			if statErr == nil && info.Mode().IsRegular() {
				defer f.Close()

				mimeType := resolveDocumentMime(doc.MimeType, cand, doc.OriginalName, cleanKey)

				filename := doc.OriginalName
				if filename == "" {
					filename = filepath.Base(cand)
				}

				disposition := "inline"
				if download {
					disposition = "attachment"
				}

				w.Header().Set("Content-Type", mimeType)
				w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filename}))
				sandboxDocumentContent(w, mimeType)
				w.Header().Set("Cache-Control", "private, max-age=3600")
				http.ServeContent(w, r, filename, info.ModTime(), f)
				return
			}
			_ = f.Close()
		}
	}

	// 3. Storage client (S3/MinIO) direct proxy stream (fetching object directly from storage without unroutable redirects)
	if h.storage != nil {
		storageKeys := []string{
			cleanKey,
			cleanPath,
			storageKeyClean,
			fmt.Sprintf("documents/%s", baseName),
			fmt.Sprintf("uploads/documents/%s", baseName),
			fmt.Sprintf("licenses/%s", baseName),
			fmt.Sprintf("uploads/licenses/%s", baseName),
			fmt.Sprintf("%s/%s", doc.DocumentType, baseName),
			fmt.Sprintf("uploads/%s", cleanPath),
		}
		if origName != "" {
			storageKeys = append(storageKeys,
				origName,
				origBaseName,
				fmt.Sprintf("documents/%s", origName),
				fmt.Sprintf("licenses/%s", origName),
				fmt.Sprintf("uploads/documents/%s", origName),
				fmt.Sprintf("uploads/licenses/%s", origName),
				fmt.Sprintf("%s/%s", doc.DocumentType, origName),
			)
		}
		if doc.OrganizationID != nil && *doc.OrganizationID > 0 {
			storageKeys = append(storageKeys,
				fmt.Sprintf("orgs/%d/%s", *doc.OrganizationID, cleanPath),
				fmt.Sprintf("orgs/%d/documents/%s", *doc.OrganizationID, baseName),
				fmt.Sprintf("orgs/%d/licenses/%s", *doc.OrganizationID, baseName),
				fmt.Sprintf("orgs/%d/%s/%s", *doc.OrganizationID, doc.DocumentType, baseName),
			)
			if origName != "" {
				storageKeys = append(storageKeys,
					fmt.Sprintf("orgs/%d/%s", *doc.OrganizationID, origName),
					fmt.Sprintf("orgs/%d/%s/%s", *doc.OrganizationID, doc.DocumentType, origName),
				)
			}
		}

		if scopedKey != "" {
			storageKeys = []string{scopedKey, "uploads/" + scopedKey}
		}
		for _, sKey := range storageKeys {
			if !documentStorageKeyAllowed(doc, sKey) {
				continue
			}
			body, cType, sErr := h.storage.Get(ctx, sKey)
			if sErr == nil && body != nil {
				defer body.Close()

				mimeType := resolveDocumentMime(cType, doc.MimeType, sKey, doc.OriginalName, baseName)

				filename := doc.OriginalName
				if filename == "" {
					filename = baseName
				}

				disposition := "inline"
				if download {
					disposition = "attachment"
				}

				w.Header().Set("Content-Type", mimeType)
				w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filename}))
				sandboxDocumentContent(w, mimeType)
				w.Header().Set("Cache-Control", "private, max-age=3600")
				_, _ = io.Copy(w, body)
				return
			}
		}
	}

	// 4. If file is unavailable anywhere, render the clear, polished Document Unavailable Error Page
	h.renderMissingDocError(w, r, doc, i18n.T(lang, "docs.serve.file_missing"), lang, dir)
}

func resolveDocumentMime(docMime string, filenames ...string) string {
	for _, fn := range filenames {
		ext := strings.ToLower(filepath.Ext(strings.TrimSpace(fn)))
		switch ext {
		case ".png":
			return "image/png"
		case ".jpg", ".jpeg":
			return "image/jpeg"
		case ".webp":
			return "image/webp"
		case ".svg":
			return "image/svg+xml"
		case ".gif":
			return "image/gif"
		case ".pdf":
			return "application/pdf"
		}
	}
	if docMime != "" && docMime != "application/octet-stream" && docMime != "binary/octet-stream" {
		return docMime
	}
	return "application/pdf"
}
