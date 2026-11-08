package http

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
)

// readBody guards everything the sync posts: JSON content type, a 32 MiB
// ceiling on the wire, gzip inflation with its own 128 MiB ceiling (the
// decompression bomb's natural home — a 6 MiB gzip of spaces unrolls past
// 128 MiB), and the raw bytes onward to the decoder. Refusals write their own
// envelope; the returned error tells the caller the body never got through.
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if contentType := r.Header.Get("Content-Type"); contentType != "" &&
		!strings.HasPrefix(contentType, "application/json") {
		return nil, refuse(w, r, erp.NewError("ERP_UNSUPPORTED_MEDIA", 415, nil))
	}
	r.Body = http.MaxBytesReader(w, r.Body, erp.MaxBodyBytes)
	reader := io.Reader(r.Body)
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, refuse(w, r, erp.NewError("ERP_BAD_JSON", 400, map[string]any{"position": 0}))
		}
		defer zr.Close()
		// Limit the inflation itself, then probe one more byte: a body that
		// saturates the limit and still has data is a bomb, not a boundary.
		reader = io.MultiReader(io.LimitReader(zr, erp.MaxInflatedBytes), probeByte{zr})
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if strings.Contains(err.Error(), "gzip: invalid") {
			return nil, refuse(w, r, erp.NewError("ERP_BAD_JSON", 400, map[string]any{"position": 0}))
		}
		if errors.As(err, &tooLarge) {
			return nil, refuse(w, r, erp.NewError("ERP_BODY_TOO_LARGE", 413, map[string]any{"limitBytes": erp.MaxBodyBytes}))
		}
		// An inflation that saturated its limit with data to spare is the bomb
		// case: ReadAll stops at the limit cleanly, so the probe byte is what
		// detects it.
		if strings.Contains(err.Error(), "probe") {
			return nil, refuse(w, r, erp.NewError("ERP_BODY_TOO_LARGE", 413, map[string]any{"limitBytes": erp.MaxInflatedBytes}))
		}
		return nil, refuse(w, r, erp.NewError("ERP_BAD_JSON", 400, nil))
	}
	return data, nil
}

// errBodyRefused marks "the caller already got an envelope" to the decode path.
var errBodyRefused = errors.New("erp: body refused")

// refuse writes the envelope and returns errBodyRefused wrapped with the
// stable code, so callers can errors.Is the sentinel and logs see the code.
func refuse(w http.ResponseWriter, r *http.Request, e *erp.Error) error {
	writeError(w, r, e)
	return fmt.Errorf("%w: %s", errBodyRefused, e.Code)
}

// probeByte yields exactly one byte after the limit, making the read fail once
// the inflated stream still has content (the gzip bomb signature).
type probeByte struct {
	r io.Reader
}

func (p probeByte) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	n, err := p.r.Read(b[:1])
	if n > 0 {
		return n, errors.New("erp: inflated body exceeds the limit; potential decompression bomb; probe")
	}
	return n, err
}
