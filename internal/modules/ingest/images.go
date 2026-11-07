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
	"sync"
	"time"
)

var (
	ErrInvalidURL       = errors.New("invalid image URL")
	ErrPrivateIPBlocked = errors.New("outbound request to private/local IP blocked (SSRF guard)")
	ErrUnsupportedType  = errors.New("unsupported image content type")
	ErrImageTooLarge    = errors.New("image exceeds size limit (max 10MB)")
)

var (
	privateIPBlocks         []*net.IPNet
	defaultSecureHTTPClient *http.Client
	clientOnce              sync.Once
)

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

// SecureHTTPClient returns a thread-safe singleton HTTP client with DNS resolution,
// SSRF protection, and connection pooling to prevent socket exhaustion during
// large bulk catalog image downloads (e.g. 20,000+ rows).
func SecureHTTPClient() *http.Client {
	clientOnce.Do(func() {
		dialer := &net.Dialer{
			Timeout:   8 * time.Second,
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

				// Prioritize IPv4 candidates over IPv6 to prevent timeouts on environments without IPv6 routing
				candidates := make([]net.IP, 0, len(ips))
				for _, ip := range ips {
					if ip.To4() != nil {
						candidates = append(candidates, ip)
					}
				}
				for _, ip := range ips {
					if ip.To4() == nil {
						candidates = append(candidates, ip)
					}
				}

				var lastErr error
				for _, ip := range candidates {
					conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
					if dialErr == nil {
						return conn, nil
					}
					lastErr = dialErr
				}
				return nil, lastErr
			},
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 8 * time.Second,
		}

		defaultSecureHTTPClient = &http.Client{
			Transport: transport,
			Timeout:   25 * time.Second,
		}
	})

	return defaultSecureHTTPClient
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
	req.Header.Set("Referer", fmt.Sprintf("%s://%s/", parsed.Scheme, parsed.Host))
	req.Header.Set("Sec-Fetch-Dest", "image")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

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

	ext := DetectImageExtension(data, resp.Header.Get("Content-Type"), parsed.Path)
	if ext == "" {
		return nil, "", ErrUnsupportedType
	}

	return data, ext, nil
}

// DetectImageExtension determines the standard extension via magic bytes, headers, or URL path.
func DetectImageExtension(data []byte, contentType, urlPath string) string {
	// 1. Magic bytes detection
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "jpg"
	}
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "png"
	}
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		return "gif"
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "webp"
	}
	if len(data) >= 12 && string(data[4:8]) == "ftyp" && (string(data[8:12]) == "avif" || string(data[8:12]) == "avis") {
		return "avif"
	}
	if len(data) >= 2 && string(data[:2]) == "BM" {
		return "bmp"
	}

	// 2. Content-Type header
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "image/jpeg") || strings.Contains(ct, "image/jpg"):
		return "jpg"
	case strings.Contains(ct, "image/png"):
		return "png"
	case strings.Contains(ct, "image/webp"):
		return "webp"
	case strings.Contains(ct, "image/gif"):
		return "gif"
	case strings.Contains(ct, "image/avif"):
		return "avif"
	case strings.Contains(ct, "image/bmp"):
		return "bmp"
	case strings.Contains(ct, "image/svg"):
		return "svg"
	}

	// 3. Fallback to DetectContentType
	if len(data) > 0 {
		sniff := http.DetectContentType(data)
		switch {
		case strings.Contains(sniff, "image/jpeg"):
			return "jpg"
		case strings.Contains(sniff, "image/png"):
			return "png"
		case strings.Contains(sniff, "image/gif"):
			return "gif"
		case strings.Contains(sniff, "image/webp"):
			return "webp"
		}
	}

	// 4. URL path suffix
	lowerPath := strings.ToLower(urlPath)
	if idx := strings.Index(lowerPath, "?"); idx != -1 {
		lowerPath = lowerPath[:idx]
	}
	switch {
	case strings.HasSuffix(lowerPath, ".jpg") || strings.HasSuffix(lowerPath, ".jpeg"):
		return "jpg"
	case strings.HasSuffix(lowerPath, ".png"):
		return "png"
	case strings.HasSuffix(lowerPath, ".webp"):
		return "webp"
	case strings.HasSuffix(lowerPath, ".gif"):
		return "gif"
	case strings.HasSuffix(lowerPath, ".avif"):
		return "avif"
	}

	return ""
}
