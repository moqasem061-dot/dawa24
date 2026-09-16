package media

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"testing"
)

func createSolidTestImage(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{R: 200, G: 50, B: 50, A: 255}}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 98})
	return buf.Bytes()
}

func createTransparentPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Partially transparent
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x+y)%2 == 0 {
				img.Set(x, y, color.RGBA{R: 0, G: 100, B: 200, A: 128})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestCompress_LargeJPEG(t *testing.T) {
	// 2000x1500 large photo
	raw := createSolidTestImage(2000, 1500)
	compressed, ext, ct, wasCompressed := Compress(raw, 1200)

	if !wasCompressed {
		t.Fatal("expected wasCompressed to be true for 2000x1500 image")
	}
	if ext != ".jpg" {
		t.Fatalf("got ext %q, want .jpg", ext)
	}
	if ct != "image/jpeg" {
		t.Fatalf("got contentType %q, want image/jpeg", ct)
	}
	if len(compressed) >= len(raw) {
		t.Fatalf("expected compressed size (%d) to be smaller than raw (%d)", len(compressed), len(raw))
	}

	// Verify dimensions of result
	cfg, _, err := image.DecodeConfig(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("decode config of compressed image: %v", err)
	}
	if cfg.Width > 1200 || cfg.Height > 1200 {
		t.Fatalf("dimensions %dx%d exceed maxEdge 1200", cfg.Width, cfg.Height)
	}
}

func TestCompress_TransparentPNG(t *testing.T) {
	raw := createTransparentPNG(800, 600)
	compressed, ext, ct, _ := Compress(raw, 1200)

	if ext != ".png" {
		t.Fatalf("got ext %q, want .png for transparent image", ext)
	}
	if ct != "image/png" {
		t.Fatalf("got contentType %q, want image/png", ct)
	}
	if len(compressed) == 0 {
		t.Fatal("compressed data is empty")
	}
}

func TestCompress_NonImage(t *testing.T) {
	pdf := []byte("%PDF-1.4 sample pdf content")
	out, ext, ct, wasCompressed := Compress(pdf, 1200)

	if wasCompressed {
		t.Fatal("expected wasCompressed to be false for non-image")
	}
	if !bytes.Equal(out, pdf) {
		t.Fatal("expected untouched original bytes for non-image")
	}
	if ext != "" || ct != "" {
		t.Fatalf("expected empty ext and ct, got ext=%q, ct=%q", ext, ct)
	}
}
