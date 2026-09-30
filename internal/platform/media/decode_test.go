package media

import (
	"errors"
	"testing"
)

func TestDecodeRejectsOversizedHeader(t *testing.T) {
	_, _, err := Decode(bombPNG(t, 60000, 60000))
	if !errors.Is(err, ErrSourceTooLarge) {
		t.Fatalf("want size rejection before decode, got %v", err)
	}
}
