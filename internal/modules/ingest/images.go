package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidURL       = errors.New("invalid image URL")
	ErrPrivateIPBlocked = errors.New("outbound request to private/local IP blocked (SSRF guard)")
	ErrUnsupportedType  = errors.New("unsupported image content type")
	ErrImageTooLarge    = errors.New("image exceeds size limit (max 10MB)")
)

var privateIPBlocks []*net.IPNet

func init() {
	cidrs := []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // Link-local
		"0.0.0.0/8",      // Current network
		"224.0.0.0/4",    // Multicast
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // Unique local address
		"fe80::/10",      // Link-local unicast
		"ff00::/8",       // Multicast
	}
	for _, cidr := range cidrs {
		_, block, _ := net.ParseCIDR(cidr)
		if block != nil {
			privateIPBlocks = append(privateIPBlocks, block)
		}
	}
}

// IsPrivateIP checks if an IP belongs to private, loopback, or reserved ranges.
func IsPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return true
	}
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// SecureHTTPClient creates an HTTP client with DNS resolution and SSRF protection.
func SecureHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}

			for _, ip := range ips {
				if IsPrivateIP(ip) {
					return nil, ErrPrivateIPBlocked
				}
			}

			if len(ips) == 0 {
				return nil, fmt.Errorf("no IP resolved for host %s", host)
			}

			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
}

// DownloadProductImage securely downloads an image from a remote URL with SSRF guards and size limits.
func DownloadProductImage(ctx context.Context, imageURL string) ([]byte, string, error) {
	imageURL = strings.Trim(strings.TrimSpace(imageURL), "\"'`")
	if strings.HasPrefix(imageURL, "//") {
		imageURL = "https:" + imageURL
	} else if strings.HasPrefix(strings.ToLower(imageURL), "www.") {
		imageURL = "https://" + imageURL
	}
	parsed, err := url.Parse(imageURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, "", ErrInvalidURL
	}

	client := SecureHTTPClient()
	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,ar;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("remote server returned status %d", resp.StatusCode)
	}

	// Limit reader to 10MB
	limitReader := io.LimitReader(resp.Body, 10<<20+1)
	data, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, "", fmt.Errorf("read response body: %w", err)
	}
	if len(data) > 10<<20 {
		return nil, "", ErrImageTooLarge
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	var ext string
	switch {
	case strings.Contains(contentType, "image/jpeg") || strings.Contains(contentType, "image/jpg"):
		ext = "jpg"
	case strings.Contains(contentType, "image/png"):
		ext = "png"
	case strings.Contains(contentType, "image/webp"):
		ext = "webp"
	case strings.Contains(contentType, "image/gif"):
		ext = "gif"
	}

	// Fallback 1: Inspect magic bytes via http.DetectContentType
	if ext == "" && len(data) > 0 {
		sniff := http.DetectContentType(data)
		switch {
		case strings.Contains(sniff, "image/jpeg"):
			ext = "jpg"
		case strings.Contains(sniff, "image/png"):
			ext = "png"
		case strings.Contains(sniff, "image/webp"):
			ext = "webp"
		case strings.Contains(sniff, "image/gif"):
			ext = "gif"
		}
	}

	// Fallback 2: Check URL path suffix
	if ext == "" {
		lowerPath := strings.ToLower(parsed.Path)
		switch {
		case strings.HasSuffix(lowerPath, ".jpg") || strings.HasSuffix(lowerPath, ".jpeg"):
			ext = "jpg"
		case strings.HasSuffix(lowerPath, ".png"):
			ext = "png"
		case strings.HasSuffix(lowerPath, ".webp"):
			ext = "webp"
		case strings.HasSuffix(lowerPath, ".gif"):
			ext = "gif"
		}
	}

	if ext == "" {
		return nil, "", ErrUnsupportedType
	}

	return data, ext, nil
}

// GenerateProductImageStorageKey produces standard storage key `products/<uuid>.<ext>`.
func GenerateProductImageStorageKey(ext string) string {
	return fmt.Sprintf("products/%s.%s", uuid.New().String(), ext)
}
