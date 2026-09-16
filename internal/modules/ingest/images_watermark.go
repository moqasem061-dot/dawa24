package ingest

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"strings"
	"sync"
)

//go:embed assets/logo.png
var platformLogoBytes []byte

var (
	platformLogoImg  image.Image
	platformLogoOnce sync.Once
)

// getPlatformLogo loads and caches the embedded platform logo.
func getPlatformLogo() image.Image {
	platformLogoOnce.Do(func() {
		img, err := png.Decode(bytes.NewReader(platformLogoBytes))
		if err == nil {
			platformLogoImg = img
		}
	})
	return platformLogoImg
}

// scaleBilinear resizes an image using bilinear interpolation for smooth antialiased rendering.
func scaleBilinear(src image.Image, targetW, targetH int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, targetW, targetH))
	srcBounds := src.Bounds()
	sw := srcBounds.Dx()
	sh := srcBounds.Dy()

	if targetW <= 0 || targetH <= 0 || sw <= 0 || sh <= 0 {
		return dst
	}

	scaleX := float64(sw) / float64(targetW)
	scaleY := float64(sh) / float64(targetH)

	for y := 0; y < targetH; y++ {
		srcY := (float64(y)+0.5)*scaleY - 0.5
		if srcY < 0 {
			srcY = 0
		}
		y0 := int(math.Floor(srcY))
		y1 := y0 + 1
		if y1 >= sh {
			y1 = sh - 1
		}
		wy := srcY - float64(y0)

		for x := 0; x < targetW; x++ {
			srcX := (float64(x)+0.5)*scaleX - 0.5
			if srcX < 0 {
				srcX = 0
			}
			x0 := int(math.Floor(srcX))
			x1 := x0 + 1
			if x1 >= sw {
				x1 = sw - 1
			}
			wx := srcX - float64(x0)

			c00 := src.At(srcBounds.Min.X+x0, srcBounds.Min.Y+y0)
			c10 := src.At(srcBounds.Min.X+x1, srcBounds.Min.Y+y0)
			c01 := src.At(srcBounds.Min.X+x0, srcBounds.Min.Y+y1)
			c11 := src.At(srcBounds.Min.X+x1, srcBounds.Min.Y+y1)

			r00, g00, b00, a00 := c00.RGBA()
			r10, g10, b10, a10 := c10.RGBA()
			r01, g01, b01, a01 := c01.RGBA()
			r11, g11, b11, a11 := c11.RGBA()

			w00 := (1 - wx) * (1 - wy)
			w10 := wx * (1 - wy)
			w01 := (1 - wx) * wy
			w11 := wx * wy

			fr := (float64(r00)*w00 + float64(r10)*w10 + float64(r01)*w01 + float64(r11)*w11) / 257.0
			fg := (float64(g00)*w00 + float64(g10)*w10 + float64(g01)*w01 + float64(g11)*w11) / 257.0
			fb := (float64(b00)*w00 + float64(b10)*w10 + float64(b01)*w01 + float64(b11)*w11) / 257.0
			fa := (float64(a00)*w00 + float64(a10)*w10 + float64(a01)*w01 + float64(a11)*w11) / 257.0

			dst.Set(x, y, color.NRGBA{
				R: uint8(math.Round(fr)),
				G: uint8(math.Round(fg)),
				B: uint8(math.Round(fb)),
				A: uint8(math.Round(fa)),
			})
		}
	}
	return dst
}

// ApplyWatermark composites a single, prominent, elegant platform logo watermark
// centered onto the image. It replaces any tiled repetition with one crisp, clear mark.
func ApplyWatermark(imgData []byte, ext string) ([]byte, error) {
	if len(imgData) == 0 {
		return imgData, nil
	}

	src, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return imgData, nil
	}

	logo := getPlatformLogo()
	if logo == nil {
		return imgData, nil
	}

	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width < 80 || height < 60 {
		return imgData, nil
	}

	logoBounds := logo.Bounds()
	logoW := logoBounds.Dx()
	logoH := logoBounds.Dy()
	if logoW <= 0 || logoH <= 0 {
		return imgData, nil
	}

	// Determine single watermark sizing:
	// Target ~50% of the image width, capped at 42% of height
	targetW := int(float64(width) * 0.50)
	targetH := int(float64(targetW) * float64(logoH) / float64(logoW))

	maxH := int(float64(height) * 0.42)
	if targetH > maxH && maxH > 10 {
		targetH = maxH
		targetW = int(float64(targetH) * float64(logoW) / float64(logoH))
	}

	if targetW < 30 || targetH < 10 {
		return imgData, nil
	}

	scaledLogo := scaleBilinear(logo, targetW, targetH)

	// Center the watermark exactly once on the image
	startX := (width - targetW) / 2
	startY := (height - targetH) / 2

	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	// Watermark opacity: 0.50 (clear, visible, anti-theft yet transparent enough for details)
	const wmAlpha = 0.50

	for ly := 0; ly < targetH; ly++ {
		py := bounds.Min.Y + startY + ly
		if py < bounds.Min.Y || py >= bounds.Max.Y {
			continue
		}
		for lx := 0; lx < targetW; lx++ {
			px := bounds.Min.X + startX + lx
			if px < bounds.Min.X || px >= bounds.Max.X {
				continue
			}

			lColor := scaledLogo.NRGBAAt(lx, ly)
			if lColor.A == 0 {
				continue
			}

			// Compute blended alpha
			alpha := (float64(lColor.A) / 255.0) * wmAlpha
			baseColor := dst.At(px, py)
			br, bg, bb, ba := baseColor.RGBA()

			baseR := float64(br >> 8)
			baseG := float64(bg >> 8)
			baseB := float64(bb >> 8)
			baseA := float64(ba >> 8)

			outR := uint8(math.Round(float64(lColor.R)*alpha + baseR*(1.0-alpha)))
			outG := uint8(math.Round(float64(lColor.G)*alpha + baseG*(1.0-alpha)))
			outB := uint8(math.Round(float64(lColor.B)*alpha + baseB*(1.0-alpha)))
			outA := uint8(baseA)
			if outA < 255 {
				outA = 255
			}

			dst.Set(px, py, color.RGBA{R: outR, G: outG, B: outB, A: outA})
		}
	}

	var outBuf bytes.Buffer
	cleanExt := strings.ToLower(strings.TrimPrefix(ext, "."))
	if cleanExt == "png" {
		if err := png.Encode(&outBuf, dst); err != nil {
			return imgData, nil
		}
	} else {
		if err := jpeg.Encode(&outBuf, dst, &jpeg.Options{Quality: 82}); err != nil {
			return imgData, nil
		}
	}

	return outBuf.Bytes(), nil
}
