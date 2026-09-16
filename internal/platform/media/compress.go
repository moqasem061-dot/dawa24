package media

import (
	"bytes"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
)

// DefaultMaxEdge is the standard maximum pixel dimension for uploaded images (1200px).
// 1200px provides crisp detail on 2x/Retina and 4K displays while dramatically reducing storage.
const DefaultMaxEdge = 1200

// Compress inspects data and, if it is a supported image format (JPEG, PNG, WebP, GIF),
// resizes it if its largest edge exceeds maxEdge (defaulting to DefaultMaxEdge if <= 0),
// strips bloat and EXIF metadata, and encodes it with optimal compression:
//   - PNG with BestCompression if transparent
//   - JPEG with quality 82 if opaque
//
// If the data is not a supported image (e.g. PDF, CSV, Excel), or if the compressed result
// is not smaller than the input, it returns the original data and wasCompressed = false.
func Compress(src []byte, maxEdge int) (out []byte, ext string, contentType string, wasCompressed bool) {
	if len(src) == 0 {
		return src, "", "", false
	}
	if maxEdge <= 0 {
		maxEdge = DefaultMaxEdge
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return src, "", "", false
	}
	if cfg.Width*cfg.Height > maxSourcePixels {
		return src, "", "", false
	}

	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return src, "", "", false
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	longest := w
	if h > longest {
		longest = h
	}

	// Transparency check: preserve alpha for images that genuinely use transparency.
	keepAlpha := (format == "png" || format == "gif" || format == "webp") && hasAlpha(img)

	var processed *image.RGBA
	if longest > maxEdge {
		processed = scale(img, maxEdge, keepAlpha)
	} else {
		processed = image.NewRGBA(image.Rect(0, 0, w, h))
		if !keepAlpha {
			draw.Draw(processed, processed.Bounds(), image.White, image.Point{}, draw.Src)
		}
		draw.Draw(processed, processed.Bounds(), img, bounds.Min, draw.Over)
	}

	var buf bytes.Buffer
	var outExt string
	var outContentType string

	if keepAlpha {
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, processed); err != nil {
			return src, "", "", false
		}
		outExt = ".png"
		outContentType = "image/png"
	} else {
		if err := jpeg.Encode(&buf, processed, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return src, "", "", false
		}
		outExt = ".jpg"
		outContentType = "image/jpeg"
	}

	compressed := buf.Bytes()
	// Adopt compressed version if it saved space or reduced dimensions.
	if len(compressed) < len(src) || longest > maxEdge {
		return compressed, outExt, outContentType, true
	}

	return src, outExt, outContentType, false
}
