package identity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

const (
	// TOTPPeriodSeconds defines the standard 30-second time step for TOTP.
	TOTPPeriodSeconds = 30
	// TOTPDigits defines standard 6-digit TOTP codes.
	TOTPDigits = 6
	// TOTPIssuer is the platform brand shown in authenticator apps.
	TOTPIssuer = "Dawa24"
)

var b32Encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateTOTPSecret creates a cryptographically secure 20-byte secret encoded in Base32.
func GenerateTOTPSecret() (string, error) {
	bytes := make([]byte, 20)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate totp secret: %w", err)
	}
	return b32Encoding.EncodeToString(bytes), nil
}

// GenerateOTPAuthURL creates the standard otpauth URI for QR code generation.
func GenerateOTPAuthURL(email, secret string) string {
	label := fmt.Sprintf("%s:%s", TOTPIssuer, email)
	u := url.URL{
		Scheme: "otpauth",
		Host:   "totp",
		Path:   label,
	}
	q := u.Query()
	q.Set("secret", secret)
	q.Set("issuer", TOTPIssuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", TOTPDigits))
	q.Set("period", fmt.Sprintf("%d", TOTPPeriodSeconds))
	u.RawQuery = q.Encode()
	return u.String()
}

// GenerateQRCodeDataURI creates a PNG QR code for an otpauth URL, returned as a Base64 Data URI.
func GenerateQRCodeDataURI(otpauthURL string) (string, error) {
	png, err := qrcode.Encode(otpauthURL, qrcode.Medium, 256)
	if err != nil {
		return "", fmt.Errorf("generate qr code: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

// GenerateTOTPCode computes the 6-digit code for a given timestamp and Base32 secret.
func GenerateTOTPCode(secret string, t time.Time) (string, error) {
	cleanSecret := strings.ToUpper(strings.TrimSpace(secret))
	key, err := b32Encoding.DecodeString(cleanSecret)
	if err != nil {
		// Fallback to standard base32 decoding if padded
		key, err = base32.StdEncoding.DecodeString(cleanSecret)
		if err != nil {
			return "", fmt.Errorf("decode base32 secret: %w", err)
		}
	}

	step := uint64(t.Unix() / TOTPPeriodSeconds)
	var stepBytes [8]byte
	binary.BigEndian.PutUint64(stepBytes[:], step)

	mac := hmac.New(sha1.New, key)
	mac.Write(stepBytes[:])
	hash := mac.Sum(nil)

	offset := hash[len(hash)-1] & 0x0F
	binaryCode := ((int(hash[offset]) & 0x7F) << 24) |
		((int(hash[offset+1]) & 0xFF) << 16) |
		((int(hash[offset+2]) & 0xFF) << 8) |
		(int(hash[offset+3]) & 0xFF)

	otp := binaryCode % 1000000
	return fmt.Sprintf("%06d", otp), nil
}

// ValidateTOTP verifies a user-provided 6-digit code against the secret.
// Allows ±1 step (±30 seconds) clock drift between client phone and server.
func ValidateTOTP(secret, userCode string, t time.Time) bool {
	cleanCode := strings.TrimSpace(userCode)
	if len(cleanCode) != TOTPDigits {
		return false
	}

	// Check current time step, previous step (-30s), and next step (+30s)
	steps := []time.Time{
		t.Add(-TOTPPeriodSeconds * time.Second),
		t,
		t.Add(TOTPPeriodSeconds * time.Second),
	}

	for _, stepTime := range steps {
		validCode, err := GenerateTOTPCode(secret, stepTime)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(cleanCode), []byte(validCode)) == 1 {
			return true
		}
	}

	return false
}

// GenerateRecoveryCodes generates 8 cryptographically secure 8-character single-use recovery codes.
func GenerateRecoveryCodes(count int) ([]string, error) {
	if count <= 0 {
		count = 8
	}
	const chars = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ" // exclude confusing chars 0,1,I,O
	codes := make([]string, count)
	bytes := make([]byte, count*8)
	if _, err := rand.Read(bytes); err != nil {
		return nil, fmt.Errorf("generate recovery codes: %w", err)
	}

	for i := 0; i < count; i++ {
		var sb strings.Builder
		for j := 0; j < 8; j++ {
			idx := bytes[i*8+j] % byte(len(chars))
			sb.WriteByte(chars[idx])
			if j == 3 {
				sb.WriteByte('-')
			}
		}
		codes[i] = sb.String()
	}
	return codes, nil
}
