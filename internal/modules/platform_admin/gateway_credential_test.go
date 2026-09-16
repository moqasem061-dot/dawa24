package platformadmin

import "testing"

func TestValidateAdminCredentialRejectsWhatWasActuallyStored(t *testing.T) {
	// The exact shape found in the live configuration: a database role and its
	// password, in the field that is sent as Basic auth to the Gateway host.
	if err := ValidateAdminCredential("postgres:FAKE-PLACEHOLDER-NOT-A-REAL-PASSWORD"); err == nil {
		t.Fatal("accepted a database credential in the Gateway administrator field")
	}
}

func TestValidateAdminCredentialRejectsConnectionStrings(t *testing.T) {
	for _, dsn := range []string{
		"postgres://user:pass@host:5432/db?sslmode=require",
		"POSTGRESQL://user:pass@host/db",
		"redis://:pass@host:6379/0",
		"https://api.example.com",
	} {
		if err := ValidateAdminCredential(dsn); err == nil {
			t.Errorf("accepted connection string %q", dsn)
		}
	}
}

func TestValidateAdminCredentialAcceptsRealCredentials(t *testing.T) {
	// Errs towards accepting: a false rejection stops an operator configuring
	// AI at all, which is a worse outcome than the check missing something.
	for _, value := range []string{
		"",
		"a-long-admin-password",
		"admin:a-long-admin-password",
		"gateway-operator:s3cret",
		"sk-virt-d5f35172f1f354d15b5110eae7bc1d55",
	} {
		if err := ValidateAdminCredential(value); err != nil {
			t.Errorf("rejected a plausible credential %q: %v", value, err)
		}
	}
}

func TestCredentialLooksMisconfiguredWarnsAboutStoredValues(t *testing.T) {
	// Deployments already hold the wrong value, and the operator opening the
	// screen is not the person who typed it. They still need to be told.
	gw := &GatewaySettings{APIKey: "postgres:FAKE-PLACEHOLDER-NOT-A-REAL-PASSWORD"}
	if !gw.CredentialLooksMisconfigured() {
		t.Error("stored database credential not reported as misconfigured")
	}

	ok := &GatewaySettings{APIKey: "admin:a-long-admin-password"}
	if ok.CredentialLooksMisconfigured() {
		t.Error("a valid credential was reported as misconfigured")
	}
}
