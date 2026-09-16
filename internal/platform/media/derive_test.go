package media

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func synth(t *testing.T, w, h int, alpha bool) image.Image {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(255)
			if alpha && (x/40+y/40)%2 == 0 {
				a = 0
			}
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), uint8((x + y) % 256), a})
		}
	}
	return img
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return b.Bytes()
}

// TestDeriveShrinksAndPreservesAspect is the property the whole package exists
// for: what comes back is much smaller and is not distorted.
func TestDeriveShrinksAndPreservesAspect(t *testing.T) {
	src := encodeJPEG(t, synth(t, 4000, 3000, false))

	got, err := Derive(src)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(got) != len(Sizes) {
		t.Fatalf("got %d renditions, want %d", len(got), len(Sizes))
	}
	for _, d := range got {
		if d.Width > d.Size.MaxEdge || d.Height > d.Size.MaxEdge {
			t.Errorf("%s is %dx%d, past MaxEdge %d", d.Size.Name, d.Width, d.Height, d.Size.MaxEdge)
		}
		if len(d.Bytes) >= len(src) {
			t.Errorf("%s is %d bytes, not smaller than the %d-byte source",
				d.Size.Name, len(d.Bytes), len(src))
		}
		// 4:3 in, 4:3 out. Allow a pixel of integer-division slack.
		wantH := d.Width * 3 / 4
		if d.Height < wantH-1 || d.Height > wantH+1 {
			t.Errorf("%s is %dx%d; 4:3 would be %dx%d", d.Size.Name, d.Width, d.Height, d.Width, wantH)
		}
		if d.Width == 0 || d.Height == 0 {
			t.Errorf("%s has a zero dimension: %dx%d", d.Size.Name, d.Width, d.Height)
		}
	}
}

// TestDeriveNeverUpscales guards the case that produced bytes and no detail: a
// small avatar asked for a "full" rendition four times its size.
func TestDeriveNeverUpscales(t *testing.T) {
	src := encodeJPEG(t, synth(t, 90, 90, false))

	got, err := Derive(src)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a 90x90 source produced %d renditions; every Size is larger than it", len(got))
	}
}

// TestDeriveKeepsTransparency: a PNG logo encoded as JPEG comes back with a
// black or white box around it, which is a visible regression rather than an
// optimisation.
func TestDeriveKeepsTransparency(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, synth(t, 900, 900, true)); err != nil {
		t.Fatalf("encode: %v", err)
	}

	got, err := Derive(b.Bytes())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("a 900x900 source produced no renditions")
	}
	for _, d := range got {
		if d.ContentType != "image/png" {
			t.Errorf("%s came back as %s; transparency would be lost", d.Size.Name, d.ContentType)
		}
		img, err := png.Decode(bytes.NewReader(d.Bytes))
		if err != nil {
			t.Fatalf("%s did not decode as png: %v", d.Size.Name, err)
		}
		if op, ok := img.(interface{ Opaque() bool }); ok && op.Opaque() {
			t.Errorf("%s is fully opaque; the alpha channel was flattened", d.Size.Name)
		}
	}
}

// TestDeriveRefusesNonImages: uploads are licences, receipts and spreadsheets
// as often as they are photographs. A CSV must come back as ErrUnsupported so
// the caller can carry on, not as a panic.
func TestDeriveRefusesNonImages(t *testing.T) {
	for _, payload := range []string{
		"id,name\n1,paracetamol\n",
		"%PDF-1.4\n%stuff\n",
		"",
	} {
		if _, err := Derive([]byte(payload)); err == nil {
			t.Errorf("Derive(%q) succeeded; want ErrUnsupported", payload)
		}
	}
}

// TestDeriveRefusesDecompressionBomb.
//
// This is the test the scratch check got wrong. Patching a PNG's IHDR without
// recomputing its CRC produces a file the decoder rejects for a BAD CHECKSUM,
// so the refusal proved nothing about the pixel ceiling. Fixing the CRC is what
// makes the header valid and forces the ceiling to be the thing that refuses
// it.
func TestDeriveRefusesDecompressionBomb(t *testing.T) {
	raw := bombPNG(t, 60000, 60000)

	// Sanity: the header must be well-formed, or this tests nothing.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("the crafted PNG header is invalid (%v); the test would pass for the wrong reason", err)
	}
	if cfg.Width != 60000 || cfg.Height != 60000 {
		t.Fatalf("header declares %dx%d, want 60000x60000", cfg.Width, cfg.Height)
	}

	_, err = Derive(raw)
	if err == nil {
		t.Fatal("a 3.6-gigapixel image was accepted; decoding it would ask for ~14 GB")
	}
	if !strings.Contains(err.Error(), "ceiling") {
		t.Errorf("refused with %q; want the pixel ceiling to be the reason", err)
	}
}

// bombPNG builds a PNG whose IHDR validly declares w x h, with a correct CRC,
// so a decoder accepts the header and only a size check can refuse it.
func bombPNG(t *testing.T, w, h uint32) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatalf("encode: %v", err)
	}
	raw := b.Bytes()

	// Layout: 8-byte signature, then the IHDR chunk as
	// [length:4][type:4]["IHDR"][data:13][crc:4]. The CRC covers type+data.
	const (
		typeOff = 12
		dataOff = 16
		dataLen = 13
	)
	binary.BigEndian.PutUint32(raw[dataOff+0:], w)
	binary.BigEndian.PutUint32(raw[dataOff+4:], h)
	crc := crc32.ChecksumIEEE(raw[typeOff : dataOff+dataLen])
	binary.BigEndian.PutUint32(raw[dataOff+dataLen:], crc)
	return raw
}

// TestFitPreservesOrientation covers the arithmetic directly, including the
// rounding case where a very wide image would otherwise scale to zero height.
func TestFitPreservesOrientation(t *testing.T) {
	cases := []struct {
		w, h, maxEdge int
		wantW, wantH  int
	}{
		{4000, 3000, 400, 400, 300},
		{3000, 4000, 400, 300, 400},
		{1000, 1000, 128, 128, 128},
		{10000, 3, 128, 128, 1}, // must not round down to zero
	}
	for _, c := range cases {
		gotW, gotH := fit(c.w, c.h, c.maxEdge)
		if gotW != c.wantW || gotH != c.wantH {
			t.Errorf("fit(%d,%d,%d) = %dx%d; want %dx%d",
				c.w, c.h, c.maxEdge, gotW, gotH, c.wantW, c.wantH)
		}
	}
}
