// Package filescan provides signature verification, magic byte validation,
// and heuristic malware/exploit detection for file uploads.
package filescan

import (
	"bytes"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Verdict indicates the outcome of a file scan.
type Verdict string

const (
	VerdictClean            Verdict = "clean"
	VerdictHeaderMismatch   Verdict = "header_mismatch"
	VerdictExecutableBinary Verdict = "executable_binary"
	VerdictEmbeddedScript   Verdict = "embedded_script"
	VerdictDangerousContent Verdict = "dangerous_content"
	VerdictCorrupted        Verdict = "corrupted"
)

// Result contains the detailed outcome of the file analysis.
type Result struct {
	Passed       bool    `json:"passed"`
	Verdict      Verdict `json:"verdict"`
	Reason       string  `json:"reason"`
	DetectedMIME string  `json:"detected_mime"`
}

// Executable magic signatures
var (
	sigPE    = []byte("MZ")
	sigELF   = []byte("\x7fELF")
	sigMachO = [][]byte{
		{0xfe, 0xed, 0xfa, 0xce},
		{0xce, 0xfa, 0xed, 0xfe},
		{0xfe, 0xed, 0xfa, 0xcf},
		{0xcf, 0xfa, 0xed, 0xfe},
		{0xca, 0xfe, 0xba, 0xbe},
	}
	sigPDF   = []byte("%PDF-")
	sigPNG   = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	sigJPEG  = []byte{0xFF, 0xD8, 0xFF}
	sigGIF87 = []byte("GIF87a")
	sigGIF89 = []byte("GIF89a")
	sigZIP   = []byte("PK\x03\x04")
)

// Scan analyzes the provided byte slice, filename, and declared MIME type
// to determine whether the file is safe to store and process.
func Scan(data []byte, filename, declaredMIME string) Result {
	if len(data) == 0 {
		return Result{
			Passed:  false,
			Verdict: VerdictCorrupted,
			Reason:  "empty file payload",
		}
	}

	detectedMIME := http.DetectContentType(data[:min(512, len(data))])

	// 1. Block executable binaries regardless of declared extension or MIME
	if isExecutable(data) {
		return Result{
			Passed:       false,
			Verdict:      VerdictExecutableBinary,
			Reason:       "executable binary signatures detected",
			DetectedMIME: detectedMIME,
		}
	}

	// 2. Block embedded server-side scripts (e.g. PHP tags in images or docs)
	if containsServerScript(data) {
		return Result{
			Passed:       false,
			Verdict:      VerdictDangerousContent,
			Reason:       "embedded server-side script tag (PHP/ASP) detected",
			DetectedMIME: detectedMIME,
		}
	}

	ext := strings.ToLower(filepath.Ext(filename))
	declaredMIME = strings.ToLower(strings.TrimSpace(declaredMIME))

	// 3. Extension and content format validation
	switch ext {
	case ".pdf":
		return scanPDF(data, detectedMIME)
	case ".png":
		return scanPNG(data, detectedMIME)
	case ".jpg", ".jpeg":
		return scanJPEG(data, detectedMIME)
	case ".webp":
		return scanWebP(data, detectedMIME)
	case ".gif":
		return scanGIF(data, detectedMIME)
	case ".svg":
		return scanSVG(data, detectedMIME)
	case ".xlsx", ".docx":
		return scanOfficeXML(data, detectedMIME)
	case ".csv":
		return scanCSV(data, detectedMIME)
	default:
		// Generic sanity check
		return Result{
			Passed:       true,
			Verdict:      VerdictClean,
			DetectedMIME: detectedMIME,
		}
	}
}

func isExecutable(data []byte) bool {
	if bytes.HasPrefix(data, sigPE) || bytes.HasPrefix(data, sigELF) {
		return true
	}
	for _, m := range sigMachO {
		if bytes.HasPrefix(data, m) {
			return true
		}
	}
	return false
}

func containsServerScript(data []byte) bool {
	lower := bytes.ToLower(data[:min(8192, len(data))])
	return bytes.Contains(lower, []byte("<?php")) ||
		bytes.Contains(lower, []byte("<?=")) ||
		bytes.Contains(lower, []byte("<%@")) ||
		bytes.Contains(lower, []byte("<script language=\"php\""))
}

func scanPDF(data []byte, detectedMIME string) Result {
	// PDF magic bytes must exist within first 1024 bytes
	limit := min(1024, len(data))
	if !bytes.Contains(data[:limit], sigPDF) {
		return Result{
			Passed:       false,
			Verdict:      VerdictHeaderMismatch,
			Reason:       "missing %PDF- magic signature in header",
			DetectedMIME: detectedMIME,
		}
	}

	// Scan for active execution objects in PDF
	contentLower := bytes.ToLower(data)
	dangerousPDFTokens := [][]byte{
		[]byte("/javascript"),
		[]byte("/js "),
		[]byte("/js<"),
		[]byte("/js("),
		[]byte("/launch"),
		[]byte("/embeddedfiles"),
	}
	for _, tok := range dangerousPDFTokens {
		if bytes.Contains(contentLower, tok) {
			return Result{
				Passed:       false,
				Verdict:      VerdictEmbeddedScript,
				Reason:       fmt.Sprintf("active PDF action token %q detected", string(tok)),
				DetectedMIME: detectedMIME,
			}
		}
	}

	return Result{Passed: true, Verdict: VerdictClean, DetectedMIME: "application/pdf"}
}

func scanPNG(data []byte, detectedMIME string) Result {
	if !bytes.HasPrefix(data, sigPNG) {
		return Result{
			Passed:       false,
			Verdict:      VerdictHeaderMismatch,
			Reason:       "corrupted or invalid PNG header signature",
			DetectedMIME: detectedMIME,
		}
	}
	return Result{Passed: true, Verdict: VerdictClean, DetectedMIME: "image/png"}
}

func scanJPEG(data []byte, detectedMIME string) Result {
	if !bytes.HasPrefix(data, sigJPEG) {
		return Result{
			Passed:       false,
			Verdict:      VerdictHeaderMismatch,
			Reason:       "corrupted or invalid JPEG header signature",
			DetectedMIME: detectedMIME,
		}
	}
	return Result{Passed: true, Verdict: VerdictClean, DetectedMIME: "image/jpeg"}
}

func scanWebP(data []byte, detectedMIME string) Result {
	if len(data) < 12 || !bytes.HasPrefix(data, []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WEBP")) {
		return Result{
			Passed:       false,
			Verdict:      VerdictHeaderMismatch,
			Reason:       "invalid WebP RIFF container signature",
			DetectedMIME: detectedMIME,
		}
	}
	return Result{Passed: true, Verdict: VerdictClean, DetectedMIME: "image/webp"}
}

func scanGIF(data []byte, detectedMIME string) Result {
	if !bytes.HasPrefix(data, sigGIF87) && !bytes.HasPrefix(data, sigGIF89) {
		return Result{
			Passed:       false,
			Verdict:      VerdictHeaderMismatch,
			Reason:       "invalid GIF header signature",
			DetectedMIME: detectedMIME,
		}
	}
	return Result{Passed: true, Verdict: VerdictClean, DetectedMIME: "image/gif"}
}

func scanSVG(data []byte, detectedMIME string) Result {
	lower := bytes.ToLower(data)
	forbidden := [][]byte{
		[]byte("<script"),
		[]byte("javascript:"),
		[]byte("onload="),
		[]byte("onerror="),
		[]byte("onclick="),
		[]byte("<iframe"),
		[]byte("<embed"),
		[]byte("<object"),
		[]byte("xlink:href=\"javascript:"),
	}
	for _, tok := range forbidden {
		if bytes.Contains(lower, tok) {
			return Result{
				Passed:       false,
				Verdict:      VerdictEmbeddedScript,
				Reason:       fmt.Sprintf("dangerous SVG token %q detected", string(tok)),
				DetectedMIME: detectedMIME,
			}
		}
	}
	return Result{Passed: true, Verdict: VerdictClean, DetectedMIME: "image/svg+xml"}
}

func scanOfficeXML(data []byte, detectedMIME string) Result {
	if !bytes.HasPrefix(data, sigZIP) {
		return Result{
			Passed:       false,
			Verdict:      VerdictHeaderMismatch,
			Reason:       "invalid Office OpenXML (ZIP) container signature",
			DetectedMIME: detectedMIME,
		}
	}
	return Result{Passed: true, Verdict: VerdictClean, DetectedMIME: detectedMIME}
}

func scanCSV(data []byte, detectedMIME string) Result {
	sample := data[:min(8192, len(data))]

	// Reject null bytes (binary disguised as CSV)
	if bytes.Contains(sample, []byte{0x00}) {
		return Result{
			Passed:       false,
			Verdict:      VerdictDangerousContent,
			Reason:       "binary null bytes detected in CSV text",
			DetectedMIME: detectedMIME,
		}
	}

	if !utf8.Valid(sample) {
		return Result{
			Passed:       false,
			Verdict:      VerdictCorrupted,
			Reason:       "invalid UTF-8 encoding in CSV text",
			DetectedMIME: detectedMIME,
		}
	}

	// Heuristic formula injection check on line starts
	lines := bytes.Split(sample, []byte("\n"))
	for _, l := range lines {
		trimmed := bytes.TrimSpace(l)
		if len(trimmed) > 0 {
			first := trimmed[0]
			if (first == '=' || first == '+' || first == '-' || first == '@') &&
				(bytes.Contains(trimmed, []byte("|")) || bytes.Contains(bytes.ToLower(trimmed), []byte("cmd")) || bytes.Contains(bytes.ToLower(trimmed), []byte("powershell"))) {
				return Result{
					Passed:       false,
					Verdict:      VerdictDangerousContent,
					Reason:       "potential spreadsheet DDE/command injection formula detected",
					DetectedMIME: detectedMIME,
				}
			}
		}
	}

	return Result{Passed: true, Verdict: VerdictClean, DetectedMIME: "text/csv"}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
