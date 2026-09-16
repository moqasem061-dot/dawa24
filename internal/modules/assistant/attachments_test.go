package assistant

import (
	"archive/zip"
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// Word and Excel were advertised as supported and refused in practice. Every
// OOXML file is a zip, DetectContentType says so, and application/zip is in
// nobody's allowlist — so the extension has to finish the job that sniffing
// starts.
func TestSniffAcceptsOOXMLWorkbooksAndDocuments(t *testing.T) {
	content := zipBytes(t)

	for _, tc := range []struct{ name, want string }{
		{"prices.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"contract.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	} {
		mime, kind, err := SniffAndValidate(content, tc.name)
		if err != nil {
			t.Fatalf("%s was refused: %v", tc.name, err)
		}
		if mime != tc.want {
			t.Errorf("%s: mime = %q, want %q", tc.name, mime, tc.want)
		}
		if kind != KindDocument {
			t.Errorf("%s: kind = %q, want document", tc.name, kind)
		}
	}
}

// A zip that is not an OOXML file stays refused: the extension only promotes a
// type, it does not invent one.
func TestSniffStillRefusesPlainArchives(t *testing.T) {
	if _, _, err := SniffAndValidate(zipBytes(t), "payload.zip"); err == nil {
		t.Fatal("a .zip must not be accepted")
	}
}

// A HEIC photograph is the default on an iPhone and looks like any other
// picture in the camera roll. It has to be refused, and it has to be refused
// with a cause the user can act on.
func TestSniffNamesHEICSpecifically(t *testing.T) {
	heic := append([]byte{0, 0, 0, 0x18}, []byte("ftypheic")...)
	heic = append(heic, make([]byte, 32)...)

	_, _, err := SniffAndValidate(heic, "IMG_0001.heic")
	if !errors.Is(err, ErrHEIC) {
		t.Fatalf("want ErrHEIC, got %v", err)
	}
	if Fail(CodeAttachmentHEIC).Message == Fail(CodeAttachmentRejected).Message {
		t.Fatal("HEIC needs its own message; the generic one tells the user nothing to do")
	}
}

func TestSniffAcceptsOrdinaryImages(t *testing.T) {
	for name, content := range map[string][]byte{
		"box.png":  pngBytes(t, 8, 8),
		"box.jpeg": jpegBytes(t, 8, 8),
	} {
		_, kind, err := SniffAndValidate(content, name)
		if err != nil || kind != KindImage {
			t.Errorf("%s: kind=%q err=%v", name, kind, err)
		}
	}
}

func TestSniffRefusesExecutablesByExtension(t *testing.T) {
	if _, _, err := SniffAndValidate(pngBytes(t, 4, 4), "invoice.exe"); err == nil {
		t.Fatal("a forbidden extension must be refused whatever the bytes say")
	}
}

// A phone photograph is several thousand pixels wide. Sending it untouched is
// what made a turn with an attachment slow enough to hit its own deadline.
func TestPrepareImageDownscalesLargePhotographs(t *testing.T) {
	big := jpegBytes(t, 3000, 2000)

	mime, out := PrepareImageForModel("image/jpeg", big)
	if mime != "image/jpeg" {
		t.Fatalf("mime = %q", mime)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("result does not decode: %v", err)
	}
	if cfg.Width > visionMaxEdge || cfg.Height > visionMaxEdge {
		t.Fatalf("long edge still %dx%d, want <= %d", cfg.Width, cfg.Height, visionMaxEdge)
	}
	if len(out) >= len(big) {
		t.Fatalf("downscale did not shrink: %d -> %d bytes", len(big), len(out))
	}
}

// A picture that is already small must arrive byte-for-byte. Re-encoding it
// would lose detail for nothing.
func TestPrepareImageLeavesSmallImagesAlone(t *testing.T) {
	small := pngBytes(t, 64, 64)
	mime, out := PrepareImageForModel("image/png", small)
	if mime != "image/png" || !bytes.Equal(out, small) {
		t.Fatalf("a small image was rewritten: mime=%q len=%d/%d", mime, len(out), len(small))
	}
}

// WebP has no decoder in the standard library. Forwarding it unchanged is
// deliberate: the model may well read it, and a picture we declined to send is
// certainly not read.
func TestPrepareImagePassesThroughUndecodableFormats(t *testing.T) {
	raw := []byte("RIFF____WEBPVP8 not really an image at all")
	mime, out := PrepareImageForModel("image/webp", raw)
	if mime != "image/webp" || !bytes.Equal(out, raw) {
		t.Fatal("an undecodable image must be forwarded unchanged")
	}
}

func TestDataURLShape(t *testing.T) {
	got := DataURL("image/png", []byte{1, 2, 3})
	if !bytes.HasPrefix([]byte(got), []byte("data:image/png;base64,")) {
		t.Fatalf("unexpected data URL: %q", got)
	}
}

// ---------------------------------------------------------------------------

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 5), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 251), G: uint8(y % 241), B: uint8((x + y) % 233), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func zipBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("[Content_Types].xml")
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := f.Write([]byte(`<?xml version="1.0"?><Types/>`)); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}
