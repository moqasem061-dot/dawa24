// Package http holds what every chat bridge's n8n endpoints share.
//
// The endpoints are machine-to-machine: no cookie, no session, no CSRF token.
// The only credential is a shared secret presented as a Bearer token, and it
// is the only thing standing between the internet and "I am chat user N". It
// is therefore compared in constant time, against a hash, and a missing or
// wrong one is refused before a byte of the body is read.
package http

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
)

// Authenticator checks the bridge's shared secret.
type Authenticator struct {
	tokenHash [32]byte
	log       *slog.Logger
}

// NewAuthenticator hashes token. An empty token refuses every call.
func NewAuthenticator(token string, log *slog.Logger) Authenticator {
	a := Authenticator{log: log}
	if token != "" {
		a.tokenHash = sha256.Sum256([]byte(token))
	}
	return a
}

// Middleware refuses a request without the shared secret.
func (a Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.Authorized(r.Header.Get("Authorization")) {
			a.log.WarnContext(r.Context(), "chat bridge: refused unauthenticated call",
				"path", r.URL.Path, "remote", r.RemoteAddr)
			WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Authorized reports whether an Authorization header carries the secret.
func (a Authenticator) Authorized(header string) bool {
	var zero [32]byte
	if a.tokenHash == zero {
		return false
	}
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return false
	}
	got := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return subtle.ConstantTimeCompare(got[:], a.tokenHash[:]) == 1
}

// WriteJSON writes an uncacheable JSON response.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ServeExport writes a stored export as a download, or 404 when file is nil.
func ServeExport(w http.ResponseWriter, file *chatbridge.ExportFile) {
	if file == nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	w.Header().Set("Content-Type", file.MIMEType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Filename}))
	w.Header().Set("Content-Length", strconv.Itoa(len(file.Content)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Content)
}
