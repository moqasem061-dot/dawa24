package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
)

// ErrSourceTooLarge reports an image rejected before allocating its pixel buffer.
var ErrSourceTooLarge = errors.New("media: source image exceeds pixel ceiling")

// Decode checks dimensions before decoding externally supplied image bytes.
// Division keeps the size check safe even when header dimensions would overflow.
func Decode(src []byte) (image.Image, string, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrUnsupported, err)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return nil, "", ErrUnsupported
	}
	if config.Width > maxSourcePixels/config.Height {
		return nil, "", ErrSourceTooLarge
	}
	return image.Decode(bytes.NewReader(src))
}
