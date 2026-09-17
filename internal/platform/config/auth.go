package config

// Auth configures authentication and registration verification policies.
type Auth struct {
	// RequireEmailVerification determines whether email must be verified via OTP at registration.
	// When false, email verification at registration is optional.
	RequireEmailVerification bool

	// RequirePhoneVerification determines whether phone must be verified via OTP at registration.
	// When false, phone verification at registration is optional.
	RequirePhoneVerification bool
}

func loadAuth() Auth {
	return Auth{
		RequireEmailVerification: getBool("AUTH_REQUIRE_EMAIL_VERIFICATION", false) || getBool("REQUIRE_EMAIL_VERIFICATION", false),
		RequirePhoneVerification: getBool("AUTH_REQUIRE_PHONE_VERIFICATION", false) || getBool("REQUIRE_PHONE_VERIFICATION", false),
	}
}
