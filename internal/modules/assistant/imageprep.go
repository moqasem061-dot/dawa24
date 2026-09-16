package assistant

import (
	"bytes"
	"image"
	"image/draw"
	_ "image/gif" // registers the GIF decoder for image.Decode
	"image/jpeg"
	_ "image/png" // registers the PNG decoder for image.Decode
)

// Getting a photograph to a vision model in a shape it will actually read.
//
// A phone camera produces a 4000×3000 JPEG of three or four megabytes. Base64
// makes that a third larger again, and it travels inside a JSON body alongside
// the conversation, the tool schemas and the question. Three things went wrong
// with sending it untouched, and all three looked to the user like "the
// assistant cannot see my picture":
//
//   - the request exceeded the Gateway's body ceiling and came back 413, which
//     the drawer reported as a generic failure;
//   - it did not exceed the ceiling but took long enough to encode, send and
//     tokenise that the turn's deadline fired first;
//   - it arrived intact and the model spent most of its input budget on image
//     tokens, leaving too little for the answer.
//
// None of that improves the answer. Vision models see in patches of a few tens
// of pixels and gain nothing above roughly 1500 pixels on the long edge — a
// photographed medicine box is as legible at 1568 as at 4000, and costs about a
// twentieth as much to send.
//
// So an oversized image is downscaled once, on the way to the model. The
// original is untouched in storage: what the user uploaded is what they see in
// the transcript and what a later download returns.

// visionMaxEdge is the longest edge the model is sent.
const visionMaxEdge = 1568

// visionMaxBytes is the size above which an image is re-encoded even when its
// dimensions are already modest — a lightly-compressed screenshot can be large
// without being big.
const visionMaxBytes = 1 << 20

// visionJPEGQuality trades a little fidelity for a lot of size. Eighty-two is
// visually indistinguishable on photographs of packaging and text at this
// resolution.
const visionJPEGQuality = 82

// PrepareImageForModel returns the bytes and MIME type to send for one image.
//
// It returns the input unchanged whenever there is nothing to gain, and — this
// matters — whenever anything goes wrong. A format Go cannot decode (WebP has
// no decoder in the standard library) is forwarded as it arrived rather than
// refused: the model may well read it, and a picture the Store declined to
// forward is definitely not read.
func PrepareImageForModel(mime string, content []byte) (string, []byte) {
	if len(content) == 0 {
		return mime, content
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return mime, content
	}
	longEdge := cfg.Width
	if cfg.Height > longEdge {
		longEdge = cfg.Height
	}
	if longEdge <= visionMaxEdge && len(content) <= visionMaxBytes {
		return mime, content
	}

	src, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return mime, content
	}

	out := downscale(src, visionMaxEdge)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: visionJPEGQuality}); err != nil {
		return mime, content
	}
	// A re-encode that came out larger is a re-encode worth discarding, which
	// happens on small images with fine detail.
	if buf.Len() >= len(content) {
		return mime, content
	}
	return "image/jpeg", buf.Bytes()
}

// downscale reduces an image so its longest edge is at most maxEdge.
//
// It is a box filter — average the source pixels covering each destination
// pixel — written out rather than pulled from a dependency. Nearest-neighbour,
// which is what a naive loop gives, aliases printed text into noise at these
// ratios, and text on packaging is the whole reason these photographs are being
// sent. Averaging is a few lines more and keeps small print readable.
func downscale(src image.Image, maxEdge int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	scale := float64(maxEdge) / float64(max(w, h))
	if scale >= 1 {
		return src
	}
	dw := max(int(float64(w)*scale), 1)
	dh := max(int(float64(h)*scale), 1)

	// Work from an RGBA copy: At() on a paletted or YCbCr image converts on
	// every call, and this loop reads every source pixel exactly once.
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		y0 := y * h / dh
		y1 := max((y+1)*h/dh, y0+1)
		for x := 0; x < dw; x++ {
			x0 := x * w / dw
			x1 := max((x+1)*w/dw, x0+1)

			var r, g, bl, a, n uint32
			for sy := y0; sy < y1; sy++ {
				row := rgba.PixOffset(0, sy)
				for sx := x0; sx < x1; sx++ {
					i := row + sx*4
					r += uint32(rgba.Pix[i])
					g += uint32(rgba.Pix[i+1])
					bl += uint32(rgba.Pix[i+2])
					a += uint32(rgba.Pix[i+3])
					n++
				}
			}
			if n == 0 {
				continue
			}
			o := dst.PixOffset(x, y)
			dst.Pix[o] = uint8(r / n)
			dst.Pix[o+1] = uint8(g / n)
			dst.Pix[o+2] = uint8(bl / n)
			dst.Pix[o+3] = uint8(a / n)
		}
	}
	return dst
}
