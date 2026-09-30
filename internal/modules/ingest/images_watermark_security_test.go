package ingest_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	"github.com/moqasem061-dot/dawa24/internal/platform/media"
)

func TestWatermarkRejectsOversizedImageBeforeDecode(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	header := encoded.Bytes()
	binary.BigEndian.PutUint32(header[16:20], 60000)
	binary.BigEndian.PutUint32(header[20:24], 60000)
	binary.BigEndian.PutUint32(header[29:33], crc32.ChecksumIEEE(header[12:29]))
	_, err := ingest.ApplyWatermark(header, "png")
	if !errors.Is(err, media.ErrSourceTooLarge) {
		t.Fatalf("want pixel ceiling rejection, got %v", err)
	}
}
