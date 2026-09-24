package httpx

import (
	"net/http"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// MaxRequestBody caps every request body at limit bytes.
//
// Individual handlers set tighter limits of their own, but about twenty
// multipart handlers call ParseMultipartForm with no cap at all, and anything
// past the in-memory budget spills to temporary files. This is the outer bound
// that holds for all of them: a body larger than any upload the platform
// accepts is refused before it is read.
func MaxRequestBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				Error(w, r, nil, apperr.New(apperr.KindValidation, "request.too_large", "Request body too large."))
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}
