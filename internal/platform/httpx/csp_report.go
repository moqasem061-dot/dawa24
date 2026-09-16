package httpx

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// CSPReportPayload is the legacy report-uri body (application/csp-report).
type CSPReportPayload struct {
	CSPReport struct {
		DocumentURI        string `json:"document-uri"`
		Referrer           string `json:"referrer"`
		ViolatedDirective  string `json:"violated-directive"`
		EffectiveDirective string `json:"effective-directive"`
		OriginalPolicy     string `json:"original-policy"`
		Disposition        string `json:"disposition"`
		BlockedURI         string `json:"blocked-uri"`
		LineNumber         int    `json:"line-number"`
		SourceFile         string `json:"source-file"`
		StatusCode         int    `json:"status-code"`
		ScriptSample       string `json:"script-sample"`
	} `json:"csp-report"`
}

// reportingAPIReport is one entry of a Reporting API body
// (application/reports+json), which report-to delivers as an array.
type reportingAPIReport struct {
	Type string `json:"type"`
	URL  string `json:"url"`
	Body struct {
		DocumentURL        string `json:"documentURL"`
		BlockedURL         string `json:"blockedURL"`
		EffectiveDirective string `json:"effectiveDirective"`
		Disposition        string `json:"disposition"`
		Sample             string `json:"sample"`
		SourceFile         string `json:"sourceFile"`
		LineNumber         int    `json:"lineNumber"`
		StatusCode         int    `json:"statusCode"`
		// Deprecation / intervention reports.
		ID      string `json:"id"`
		Message string `json:"message"`
	} `json:"body"`
}

// violation is the one shape both formats are logged in.
type violation struct {
	documentURI, blockedURI, directive, disposition, sourceFile, sample string
	line                                                                int
}

// CSPReportHandler receives browser violation reports, bounds request size,
// and logs structured alerts for real-time exploit and injection monitoring.
//
// allowedOrigins are the site origins whose pages report here. The endpoint
// normally lives on its own origin (CSP_REPORT_URL), and the Reporting API
// delivers application/reports+json with CORS, so it must answer the
// preflight for those origins or the browser drops every report.
func CSPReportHandler(log *slog.Logger, allowedOrigins ...string) http.HandlerFunc {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o != "" {
			allowed[strings.TrimRight(strings.ToLower(o), "/")] = true
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && allowed[strings.ToLower(origin)] {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "POST")
				h.Set("Access-Control-Allow-Headers", "Content-Type")
				h.Set("Access-Control-Max-Age", "86400")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		// Cap incoming payload size to 32 KB to avoid memory exhaustion
		r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Payload Too Large or Unreadable", http.StatusBadRequest)
			return
		}

		if log != nil {
			for _, v := range parseViolations(log, r, data) {
				log.WarnContext(r.Context(), "content security policy violation reported",
					"blocked_uri", v.blockedURI,
					"effective_directive", v.directive,
					"document_uri", v.documentURI,
					"source_file", v.sourceFile,
					"line_number", v.line,
					"sample", v.sample,
					"disposition", v.disposition,
					"client_ip", clientIP(r),
				)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// maxReportsPerBody bounds how many entries of one Reporting API batch are
// logged, so a forged body cannot turn one request into thousands of lines.
const maxReportsPerBody = 20

func parseViolations(log *slog.Logger, r *http.Request, data []byte) []violation {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil
	}

	if data[0] == '[' {
		var batch []reportingAPIReport
		if json.Unmarshal(data, &batch) != nil {
			return nil
		}
		var out []violation
		for i, rep := range batch {
			if i == maxReportsPerBody {
				break
			}
			if rep.Type != "csp-violation" {
				log.DebugContext(r.Context(), "browser report received",
					"type", rep.Type, "url", rep.URL, "id", rep.Body.ID, "message", rep.Body.Message)
				continue
			}
			b := rep.Body
			out = append(out, violation{
				documentURI: b.DocumentURL, blockedURI: b.BlockedURL,
				directive: b.EffectiveDirective, disposition: b.Disposition,
				sourceFile: b.SourceFile, sample: b.Sample, line: b.LineNumber,
			})
		}
		return out
	}

	var payload CSPReportPayload
	if json.Unmarshal(data, &payload) != nil || payload.CSPReport.DocumentURI == "" {
		return nil
	}
	rep := payload.CSPReport
	directive := rep.EffectiveDirective
	if directive == "" {
		directive = rep.ViolatedDirective
	}
	return []violation{{
		documentURI: rep.DocumentURI, blockedURI: rep.BlockedURI,
		directive: directive, disposition: rep.Disposition,
		sourceFile: rep.SourceFile, sample: rep.ScriptSample, line: rep.LineNumber,
	}}
}
