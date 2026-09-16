package platformadmin

import "testing"

// Reusing the database password as the Gateway credential is a legitimate
// configuration. It is flagged for the operator, never refused: a veto here
// would leave AI unconfigurable for a deployment that genuinely uses one
// secret for both.
func TestDatabasePasswordWarnsButDoesNotBlock(t *testing.T) {
	SetKnownDatabaseSecret("s3cr3t-db-pass")
	t.Cleanup(func() { SetKnownDatabaseSecret("") })

	for _, value := range []string{"s3cr3t-db-pass", "admin:s3cr3t-db-pass", "  s3cr3t-db-pass  "} {
		if err := ValidateAdminCredential(value); err != nil {
			t.Errorf("ValidateAdminCredential(%q) = %v, want nil (must not block)", value, err)
		}
		if !MatchesDatabaseSecret(value) {
			t.Errorf("MatchesDatabaseSecret(%q) = false, want true", value)
		}
		if gw := (&GatewaySettings{APIKey: value}); !gw.CredentialLooksMisconfigured() {
			t.Errorf("CredentialLooksMisconfigured(%q) = false, want true", value)
		}
	}

	// A connection string is still refused outright — that one is never a
	// credential, whatever the operator intended.
	if err := ValidateAdminCredential("postgres://u:p@host:5432/db"); err == nil {
		t.Error("ValidateAdminCredential(dsn) = nil, want rejection")
	}

	if MatchesDatabaseSecret("an-unrelated-password") {
		t.Error("MatchesDatabaseSecret(unrelated) = true, want false")
	}
}

// With no secret registered nothing is flagged.
func TestMatchesDatabaseSecretWithoutKnownSecret(t *testing.T) {
	SetKnownDatabaseSecret("")
	if MatchesDatabaseSecret("anything-at-all") {
		t.Error("MatchesDatabaseSecret = true, want false")
	}
}
