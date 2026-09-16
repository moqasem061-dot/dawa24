package ingest_test

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
)

func TestApplyWatermark_JPEG(t *testing.T) {
	// Create a dummy 400x300 image
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{200, 220, 240, 255}}, image.Point{}, draw.Src)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("encode dummy jpeg: %v", err)
	}

	watermarked, err := ingest.ApplyWatermark(buf.Bytes(), "jpg")
	if err != nil {
		t.Fatalf("ApplyWatermark error: %v", err)
	}
	if len(watermarked) == 0 {
		t.Fatal("ApplyWatermark returned empty buffer")
	}

	decoded, _, err := image.Decode(bytes.NewReader(watermarked))
	if err != nil {
		t.Fatalf("failed to decode watermarked image: %v", err)
	}
	if decoded.Bounds().Dx() != 400 || decoded.Bounds().Dy() != 300 {
		t.Fatalf("bounds mismatch: got %dx%d, want 400x300", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

func TestApplyWatermark_PNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 250, 250))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{100, 150, 200, 255}}, image.Point{}, draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode dummy png: %v", err)
	}

	watermarked, err := ingest.ApplyWatermark(buf.Bytes(), "png")
	if err != nil {
		t.Fatalf("ApplyWatermark error: %v", err)
	}
	if len(watermarked) == 0 {
		t.Fatal("ApplyWatermark returned empty buffer")
	}

	decoded, _, err := image.Decode(bytes.NewReader(watermarked))
	if err != nil {
		t.Fatalf("failed to decode watermarked image: %v", err)
	}
	if decoded.Bounds().Dx() != 250 || decoded.Bounds().Dy() != 250 {
		t.Fatalf("bounds mismatch: got %dx%d, want 250x250", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

func TestApplyWatermark_SingleInstance_Centered(t *testing.T) {
	// 500x500 white canvas
	baseColor := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	img := image.NewRGBA(image.Rect(0, 0, 500, 500))
	draw.Draw(img, img.Bounds(), &image.Uniform{baseColor}, image.Point{}, draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode dummy png: %v", err)
	}

	watermarked, err := ingest.ApplyWatermark(buf.Bytes(), "png")
	if err != nil {
		t.Fatalf("ApplyWatermark error: %v", err)
	}

	decoded, _, err := image.Decode(bytes.NewReader(watermarked))
	if err != nil {
		t.Fatalf("decode watermarked image: %v", err)
	}

	// 1. Top-left corner (10, 10) must remain pure base color (no tiling or corner box)
	cornerPixel := decoded.At(10, 10)
	cr, cg, cb, _ := cornerPixel.RGBA()
	if uint8(cr>>8) != 255 || uint8(cg>>8) != 255 || uint8(cb>>8) != 255 {
		t.Fatalf("corner pixel (10,10) was altered (expected pure white 255,255,255): got %d,%d,%d", cr>>8, cg>>8, cb>>8)
	}

	// 2. Top-right corner (490, 10) must remain pure base color
	trPixel := decoded.At(490, 10)
	trR, trG, trB, _ := trPixel.RGBA()
	if uint8(trR>>8) != 255 || uint8(trG>>8) != 255 || uint8(trB>>8) != 255 {
		t.Fatalf("top-right pixel (490,10) was altered: got %d,%d,%d", trR>>8, trG>>8, trB>>8)
	}

	// 3. Center area must contain watermark modifications
	hasWatermarkColor := false
	for y := 200; y <= 300; y++ {
		for x := 150; x <= 350; x++ {
			p := decoded.At(x, y)
			r, g, b, _ := p.RGBA()
			if uint8(r>>8) < 250 || uint8(g>>8) < 250 || uint8(b>>8) < 250 {
				hasWatermarkColor = true
				break
			}
		}
		if hasWatermarkColor {
			break
		}
	}
	if !hasWatermarkColor {
		t.Fatal("expected center area to have watermark logo blended, but it remained entirely white")
	}
}
