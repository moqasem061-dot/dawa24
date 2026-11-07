package erp

import (
	"regexp"
	"testing"
)

func TestKeyGenerationAndVerification(t *testing.T) {
	pepper := []byte("test-pepper")
	format := regexp.MustCompile(`^d24_(live|test)_[a-z0-9]{8}_[A-Za-z0-9_-]{43}$`)
	for _, tag := range []string{"live", "test"} {
		full, prefix, hash, err := GenerateKey(tag, pepper)
		if err != nil {
			t.Fatal(err)
		}
		if !format.MatchString(full) {
			t.Errorf("invalid key format")
		}
		gotPrefix, secret, ok := ParseKey(full)
		if !ok || gotPrefix != prefix || !VerifyHash(pepper, secret, hash) {
			t.Error("key did not round trip")
		}
		if VerifyHash(pepper, secret+"x", hash) {
			t.Error("wrong secret verified")
		}
	}
}
func TestKeyPrefixesUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		_, prefix, _, err := GenerateKey("test", []byte("pepper"))
		if err != nil {
			t.Fatal(err)
		}
		if seen[prefix] {
			t.Fatal("duplicate prefix")
		}
		seen[prefix] = true
	}
}
func TestKeyParseRejectsInvalid(t *testing.T) {
	for _, raw := range []string{"short", "d24_bad_abcdefgh_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq", "d24_test_abcdefgh_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq_extra"} {
		if _, _, ok := ParseKey(raw); ok {
			t.Errorf("accepted invalid key")
		}
	}
}
func TestKeyHash(t *testing.T) {
	hash := HashSecret([]byte("pepper"), "secret")
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(hash) {
		t.Error("invalid hash")
	}
	if hash == HashSecret([]byte("other"), "secret") {
		t.Error("pepper does not affect hash")
	}
}
