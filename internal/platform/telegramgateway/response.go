package telegramgateway

import (
	"fmt"
	"io"
)

// Gateway replies are small metadata, never attachments. Bound them before decoding.
func readGatewayResponse(body io.Reader) ([]byte, error) {
	const maxResponseBytes = 64 << 10
	content, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: reading response", ErrServiceUnavailable)
	}
	if len(content) > maxResponseBytes {
		return nil, fmt.Errorf("%w: oversized response", ErrServiceUnavailable)
	}
	return content, nil
}
