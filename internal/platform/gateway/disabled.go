package gateway

import (
	"context"
)

// Disabled is a Client that always reports the Gateway as switched off.
//
// It exists for three situations, and each one matters:
//
//  1. Local development, where no virtual key is issued.
//  2. The phases of the rebuild before the Gateway is wired up at all — the
//     Store must be fully functional without it.
//  3. The black-hole test suite, which runs the complete order and import flows
//     with AI unavailable and asserts that nothing user-facing breaks.
//
// Because every caller must handle ErrDisabled via ShouldFallback, wiring this
// implementation in is a complete, supported configuration rather than a
// degraded one.
type Disabled struct{}

func (Disabled) Invoke(context.Context, Request) (*Response, error) { return nil, ErrDisabled }
func (Disabled) Stream(context.Context, ChatRequest) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 1)
	ch <- StreamEvent{Err: ErrDisabled}
	close(ch)
	return ch, nil
}
func (Disabled) Transcribe(context.Context, TranscribeRequest) (string, error) {
	return "", ErrDisabled
}
func (Disabled) Capabilities(context.Context, Role) (ModelCapabilities, error) {
	return ConservativeDefaultCapabilities(), ErrDisabled
}
func (Disabled) Health(context.Context) error { return ErrDisabled }
func (Disabled) Enabled() bool                { return false }

// Ensure both implementations satisfy the interface at compile time.
var (
	_ Client = (*HTTPClient)(nil)
	_ Client = Disabled{}
)
