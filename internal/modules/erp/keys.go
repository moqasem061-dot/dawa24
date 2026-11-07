package erp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/big"
	"regexp"
	"strings"
)

var keyPattern = regexp.MustCompile(`^d24_(live|test)_([a-z0-9]{8})_([A-Za-z0-9_-]{43})$`)

func GenerateKey(tag string, pepper []byte) (full, prefix, hash string, err error) {
	if tag != "live" && tag != "test" {
		return "", "", "", errors.New("invalid key tag")
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	var b strings.Builder
	for i := 0; i < KeyPrefixLen; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", "", "", err
		}
		b.WriteByte(alphabet[n.Int64()])
	}
	prefix = b.String()
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", "", "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	return "d24_" + tag + "_" + prefix + "_" + secret, prefix, HashSecret(pepper, secret), nil
}
func ParseKey(raw string) (prefix, secret string, ok bool) {
	parts := keyPattern.FindStringSubmatch(raw)
	if parts == nil {
		return "", "", false
	}
	return parts[2], parts[3], true
}
func HashSecret(pepper []byte, secret string) string {
	mac := hmac.New(sha256.New, pepper)
	_, _ = mac.Write([]byte(secret))
	return hex.EncodeToString(mac.Sum(nil))
}
func VerifyHash(pepper []byte, secret, hash string) bool {
	expected, err := hex.DecodeString(hash)
	if err != nil {
		return false
	}
	actual, _ := hex.DecodeString(HashSecret(pepper, secret))
	return hmac.Equal(actual, expected)
}
