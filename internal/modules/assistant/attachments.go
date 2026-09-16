package assistant

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Allowed MIME classifications
const (
	MaxAttachmentBytes    = 10 << 20
	MaxAttachmentsPerTurn = 5

	KindImage    = "image"
	KindAudio    = "audio"
	KindVideo    = "video"
	KindDocument = "document"
	KindUnknown  = "unknown"
)

var allowedMIMEs = map[string]string{
	// Images
	"image/png":  KindImage,
	"image/jpeg": KindImage,
	"image/jpg":  KindImage,
	"image/webp": KindImage,
	"image/gif":  KindImage,

	// Audio
	"audio/mpeg":  KindAudio,
	"audio/mp3":   KindAudio,
	"audio/wav":   KindAudio,
	"audio/x-wav": KindAudio,
	"audio/webm":  KindAudio,
	"audio/ogg":   KindAudio,
	"audio/m4a":   KindAudio,
	"audio/x-m4a": KindAudio,

	// Video
	"video/mp4":  KindVideo,
	"video/webm": KindVideo,

	// Documents
	"application/pdf": KindDocument,
	"text/plain":      KindDocument,
	"text/csv":        KindDocument,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       KindDocument,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": KindDocument,
}

var forbiddenExtensions = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".bat": true, ".cmd": true, ".sh": true,
	".zip": true, ".tar": true, ".gz": true, ".rar": true, ".7z": true, ".iso": true,
	".bin": true, ".com": true, ".msi": true, ".js": true, ".vbs": true,
}

// ClassifyMIME returns the kind classification for a MIME type or KindUnknown.
func ClassifyMIME(mime string) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if idx := strings.Index(mime, ";"); idx != -1 {
		mime = strings.TrimSpace(mime[:idx])
	}
	if kind, ok := allowedMIMEs[mime]; ok {
		return kind
	}
	return KindUnknown
}

// ErrHEIC names the one rejection worth telling the user how to fix.
//
// An iPhone photographs in HEIC by default. Nothing in this stack decodes it
// and the models behind the Gateway do not accept it, so it has to be refused —
// but refusing it as "unsupported file type" left people re-attaching the same
// photograph, because from the camera roll it looks exactly like a JPEG.
var ErrHEIC = errors.New("assistant: heic image")

// SniffAndValidate checks file bytes and declared filename against the security allowlist.
//
// The declared Content-Type is never trusted: it is attacker-supplied, and a
// .exe announcing itself as image/png would otherwise walk straight through.
// What the extension IS used for is disambiguation, because content sniffing
// alone cannot tell some formats apart — every OOXML file is a zip, and a CSV
// is a text file. Those two cases are why Word and Excel uploads used to be
// refused outright despite being advertised as supported: DetectContentType
// answers "application/zip" for both, which is in nobody's allowlist.
func SniffAndValidate(content []byte, declaredFilename string) (string, string, error) {
	if len(content) == 0 {
		return "", "", errors.New("empty file content")
	}

	ext := strings.ToLower(filepath.Ext(declaredFilename))
	if forbiddenExtensions[ext] {
		return "", "", fmt.Errorf("forbidden file extension: %s", ext)
	}

	if isHEIC(content) {
		return "", "", ErrHEIC
	}

	detectedMIME := http.DetectContentType(content)
	if idx := strings.Index(detectedMIME, ";"); idx != -1 {
		detectedMIME = strings.TrimSpace(detectedMIME[:idx])
	}
	detectedMIME = refineByExtension(detectedMIME, ext, content)

	kind := ClassifyMIME(detectedMIME)
	if kind == KindUnknown {
		return "", "", fmt.Errorf("unsupported file type: %s", detectedMIME)
	}

	return detectedMIME, kind, nil
}

// refineByExtension resolves the formats sniffing cannot name on its own.
func refineByExtension(detected, ext string, content []byte) string {
	switch ext {
	case ".csv":
		if detected == "text/plain" || detected == "application/octet-stream" {
			return "text/csv"
		}
	case ".pdf":
		if detected == "application/octet-stream" || bytes.HasPrefix(content, []byte("%PDF")) {
			return "application/pdf"
		}
	case ".xlsx":
		// An OOXML workbook is a zip. So is a jar, which is why the extension
		// only promotes a type that sniffing already agrees is a zip.
		if detected == "application/zip" {
			return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		}
	case ".docx":
		if detected == "application/zip" {
			return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		}
	case ".txt", ".md", ".log":
		if detected == "application/octet-stream" && utf8.Valid(content) {
			return "text/plain"
		}
	case ".m4a", ".mp4":
		// DetectContentType calls both video/mp4; an .m4a is audio in the same
		// container, and sending it as video makes a model that reads audio and
		// not video refuse a voice note it could have transcribed.
		if ext == ".m4a" && detected == "video/mp4" {
			return "audio/m4a"
		}
	}
	return detected
}

// isHEIC recognises Apple's photograph container by its ISO-BMFF brand.
//
// The layout is [4-byte size][ftyp][4-byte major brand]; the brands that matter
// are heic, heix, hevc, mif1 and msf1.
func isHEIC(content []byte) bool {
	if len(content) < 12 || !bytes.Equal(content[4:8], []byte("ftyp")) {
		return false
	}
	switch string(content[8:12]) {
	case "heic", "heix", "hevc", "hevx", "mif1", "msf1", "heim", "heis":
		return true
	}
	return false
}

// ComputeContentHash returns the SHA256 hex string of content bytes.
func ComputeContentHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// SanitiseFilename strips directory traversal characters while keeping the safe base name.
func SanitiseFilename(name string) string {
	base := filepath.Base(filepath.Clean(name))
	base = strings.ReplaceAll(base, "\\", "_")
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.TrimSpace(base)
	if base == "" || base == "." {
		return "attachment"
	}
	return base
}

// DataURL renders file bytes for the Gateway's multimodal parts.
//
// It is built at the moment a turn needs it and thrown away afterwards. It is
// deliberately NOT a field on anything persisted: base64 in the database was
// how a single 10 MB upload turned into 13 MB of JSONB replayed on every
// history load.
func DataURL(mime string, content []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(content)
}
