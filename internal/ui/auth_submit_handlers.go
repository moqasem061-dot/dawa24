package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/hr"
	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/mailer"
	"github.com/moqasem061-dot/dawa24/internal/platform/telegramgateway"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func (h *UIHandler) RegisterSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	// Process license attachment file if uploaded. Metadata (original name,
	// MIME, size) is kept so the persisted document row stays previewable;
	// a missing file is not an error — the document is optional at this step.
	// A file that was attached but could not be stored fails the form below,
	// once it is built, rather than registering the account without it.
	licenseMeta, licenseErr := optionalUploadFull(r, "license_file", "licenses")
	licenseURL := licenseMeta.URL

	var latPtr, lonPtr *float64
	if latStr := r.PostFormValue("branch_lat"); latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil && lat != 0 {
			latPtr = &lat
		}
	}
	if lonStr := r.PostFormValue("branch_lon"); lonStr != "" {
		if lon, err := strconv.ParseFloat(lonStr, 64); err == nil && lon != 0 {
			lonPtr = &lon
		}
	}

	accountType := r.PostFormValue("account_type")
	switch accountType {
	case "job_seeker", "seeker":
		accountType = "job_seeker"
	case "supplier", "vendor":
		accountType = "vendor"
	default:
		accountType = "customer"
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	email := strings.TrimSpace(r.PostFormValue("email"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	verifiedPhoneToken := strings.TrimSpace(r.PostFormValue("verified_phone_token"))
	verifiedEmailToken := strings.TrimSpace(r.PostFormValue("verified_email_token"))
	legalName := strings.TrimSpace(r.PostFormValue("legal_name"))
	tradeNameAr := strings.TrimSpace(r.PostFormValue("trade_name_ar"))
	tradeNameEn := strings.TrimSpace(r.PostFormValue("trade_name_en"))
	cr := strings.TrimSpace(r.PostFormValue("commercial_register"))
	taxNum := strings.TrimSpace(r.PostFormValue("tax_number"))
	licenseNum := strings.TrimSpace(r.PostFormValue("pharmacist_license"))
	address := strings.TrimSpace(r.PostFormValue("address"))

	if legalName == "" {
		if tradeNameAr != "" {
			legalName = tradeNameAr
		} else if name != "" {
			legalName = name
		} else {
			legalName = i18n.T(langOf(r), "auth.register.default_legal_name")
		}
	}
	if tradeNameAr == "" {
		tradeNameAr = legalName
	}
	if name == "" {
		name = legalName
	}
	if address == "" {
		address = i18n.T(langOf(r), "auth.register.default_address")
	}

	form := pages.RegisterFormData{
		AccountType:        accountType,
		Name:               name,
		Email:              email,
		Phone:              phone,
		VerifiedPhoneToken: verifiedPhoneToken,
		VerifiedEmailToken: verifiedEmailToken,
		LegalName:          legalName,
		TradeNameAr:        tradeNameAr,
		TradeNameEn:        tradeNameEn,
		CommercialRegister: cr,
		TaxNumber:          taxNum,
		PharmacistLicense:  licenseNum,
		LicenseDocumentURL: licenseURL,
		GovernorateID: func() string {
			if v := r.PostFormValue("branch_governorate_id"); v != "" {
				return v
			}
			return r.PostFormValue("governorate_id")
		}(),
		CityID: func() string {
			if v := r.PostFormValue("branch_city_id"); v != "" {
				return v
			}
			return r.PostFormValue("city_id")
		}(),
		BranchCount:              r.PostFormValue("branch_count"),
		Address:                  address,
		Latitude:                 r.PostFormValue("branch_lat"),
		Longitude:                r.PostFormValue("branch_lon"),
		GoogleMapsURL:            r.PostFormValue("branch_google_maps_url"),
		Specialisation:           strings.TrimSpace(r.PostFormValue("specialisation")),
		YearsExperience:          strings.TrimSpace(r.PostFormValue("years_experience")),
		Bio:                      strings.TrimSpace(r.PostFormValue("bio")),
		ExpectedSalary:           strings.TrimSpace(r.PostFormValue("expected_salary")),
		TurnstileSiteKey:         h.turnstileSiteKey(),
		RequireEmailVerification: h.requireEmailVerification,
		RequirePhoneVerification: h.requirePhoneVerification,
	}

	if err := h.verifyTurnstile(r, "register"); err != nil {
		h.log.WarnContext(ctx, "registration rejected: turnstile verification failed", "error", err)
		form.Error = i18n.T(lang, "auth.turnstile.failed")
		h.renderPage(ctx, w, "render register page validation error", pages.RegisterPage(lang, dir, form, h.listCities(ctx), h.listGovernorates(ctx)))
		return
	}
	if licenseErr != nil {
		h.log.WarnContext(ctx, "registration licence file could not be saved", "error", licenseErr)
		form.Error = i18n.T(lang, "upload.attachment_failed")
		h.renderPage(ctx, w, "render register page validation error", pages.RegisterPage(lang, dir, form, h.listCities(ctx), h.listGovernorates(ctx)))
		return
	}

	password := r.PostFormValue("password")
	if err := identity.ValidatePassword(password); err != nil {
		form.Error = err.Error()
		h.renderPage(ctx, w, "render register page validation error", pages.RegisterPage(lang, dir, form, h.listCities(ctx), h.listGovernorates(ctx)))
		return
	}

	// Phone verification (required if h.requirePhoneVerification, otherwise optional)
	var normPhone string
	if phone != "" {
		if norm, err := telegramgateway.NormalizePhone(phone); err == nil { // nolint:errswallow: an unnormalisable number stays empty and is rejected by the validation below when required
			normPhone = norm
		}
	}

	if h.requirePhoneVerification {
		if phone == "" {
			form.Error = i18n.T(lang, "auth.telegram.phone_required")
			h.renderPage(ctx, w, "render register page validation error", pages.RegisterPage(lang, dir, form, h.listCities(ctx), h.listGovernorates(ctx)))
			return
		}
		if normPhone == "" {
			form.Error = i18n.T(lang, "auth.telegram.invalid_phone")
			h.renderPage(ctx, w, "render register page validation error", pages.RegisterPage(lang, dir, form, h.listCities(ctx), h.listGovernorates(ctx)))
			return
		}
		isVerified, verifyErr := telegramgateway.VerifyPhoneToken(verifiedPhoneToken, normPhone, h.phoneSecret())
		if !isVerified || verifyErr != nil {
			form.Error = i18n.T(lang, "auth.telegram.phone_required")
			form.PhoneVerified = false
			form.VerifiedPhoneToken = ""
			h.renderPage(ctx, w, "render register page validation error", pages.RegisterPage(lang, dir, form, h.listCities(ctx), h.listGovernorates(ctx)))
			return
		}
		form.PhoneVerified = true
	} else if verifiedPhoneToken != "" && normPhone != "" {
		isVerified, _ := telegramgateway.VerifyPhoneToken(verifiedPhoneToken, normPhone, h.phoneSecret())
		form.PhoneVerified = isVerified
	}

	// Email verification (required if h.requireEmailVerification, otherwise optional)
	cleanEmail := identity.NormalizeEmail(email)
	if cleanEmail == "" {
		form.Error = i18n.T(lang, "auth.email.email_required")
		h.renderPage(ctx, w, "render register page validation error", pages.RegisterPage(lang, dir, form, h.listCities(ctx), h.listGovernorates(ctx)))
		return
	}
	if h.idSvc == nil {
		http.Redirect(w, r, "/auth/register?error=service_unavailable", http.StatusSeeOther)
		return
	}
	registered, lookupErr := h.emailAlreadyRegistered(ctx, cleanEmail)
	if lookupErr != nil {
		h.log.ErrorContext(ctx, "failed to check registration email", "error", lookupErr)
		form.Error = i18n.T(lang, "common.service_unavailable")
		h.renderPage(ctx, w, "render register page email lookup error", pages.RegisterPage(lang, dir, form, h.listCities(ctx), h.listGovernorates(ctx)))
		return
	}
	if registered {
		form.Error = i18n.T(lang, "auth.email.already_registered")
		h.renderPage(ctx, w, "render register page duplicate email", pages.RegisterPage(lang, dir, form, h.listCities(ctx), h.listGovernorates(ctx)))
		return
	}

	if h.requireEmailVerification {
		isEmailVerified, emailVerifyErr := mailer.VerifyEmailToken(verifiedEmailToken, cleanEmail, h.phoneSecret())
		if !isEmailVerified || emailVerifyErr != nil {
			form.Error = i18n.T(lang, "auth.email.email_required")
			form.EmailVerified = false
			form.VerifiedEmailToken = ""
			h.renderPage(ctx, w, "render register page validation error", pages.RegisterPage(lang, dir, form, h.listCities(ctx), h.listGovernorates(ctx)))
			return
		}
		form.EmailVerified = true
	} else if verifiedEmailToken != "" {
		isEmailVerified, _ := mailer.VerifyEmailToken(verifiedEmailToken, cleanEmail, h.phoneSecret())
		form.EmailVerified = isEmailVerified
	}

	if normPhone != "" {
		form.Phone = normPhone
	}
	form.Email = cleanEmail
	nowUTC := time.Now().UTC()
	var emailVerifiedAt, phoneVerifiedAt *time.Time
	if form.EmailVerified {
		emailVerifiedAt = &nowUTC
	}
	if form.PhoneVerified {
		phoneVerifiedAt = &nowUTC
	}

	cityIDStr := r.PostFormValue("branch_city_id")
	if cityIDStr == "" {
		cityIDStr = r.PostFormValue("city_id")
	}

	// The city id arrives from a form and was taken on trust. It decides which
	// suppliers cover this pharmacy, so it has to name a real city — and, when
	// a governorate was chosen too, a city inside it. Otherwise the two pickers
	// can disagree and the account is created against a coverage area nobody
	// selected.
	var cityIDPtr *int64
	if id, err := strconv.ParseInt(cityIDStr, 10, 64); err == nil && id > 0 {
		govIDStr := r.PostFormValue("branch_governorate_id")
		if govIDStr == "" {
			govIDStr = r.PostFormValue("governorate_id")
		}
		if !h.cityBelongsToGovernorate(ctx, id, govIDStr) {
			form.Error = i18n.T(lang, "auth.register.city_governorate_mismatch")
			h.renderPage(ctx, w, "render register page validation error",
				pages.RegisterPage(lang, dir, form, h.listCities(ctx), h.listGovernorates(ctx)))
			return
		}
		cityIDPtr = &id
	} else if latPtr != nil && lonPtr != nil {
		if nearestID := h.findNearestCityID(ctx, *latPtr, *lonPtr); nearestID > 0 {
			cityIDPtr = &nearestID
		}
	}

	branchCount := 1
	if bc, err := strconv.Atoi(r.PostFormValue("branch_count")); err == nil && bc > 1 {
		branchCount = bc
	}

	// 1. If Job Seeker, create direct user + job seeker profile
	if form.AccountType == "job_seeker" {
		cvURL, _ := saveUploadedFile(r, "cv_file", "cvs")
		user, sess, err := h.idSvc.Register(ctx, identity.RegisterInput{
			Email:           form.Email,
			Password:        password,
			NameAr:          form.Name,
			NameEn:          form.Name,
			Role:            identity.RoleJobSeeker,
			Phone:           form.Phone,
			PhoneVerifiedAt: phoneVerifiedAt,
			EmailVerifiedAt: emailVerifiedAt,
		})
		if err != nil {
			h.log.WarnContext(ctx, "ui job seeker registration failed", "email", form.Email, "error", err)
			form.Error = h.safeMessage(err, lang)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if rerr := pages.RegisterPage(lang, dir, form, h.listCities(ctx), h.listGovernorates(ctx)).Render(renderContext(ctx), w); rerr != nil {
				h.log.ErrorContext(ctx, "render register page after error", "error", rerr)
			}
			return
		}

		if h.hrSvc != nil && user != nil {
			exp, _ := strconv.Atoi(form.YearsExperience)
			sal, _ := money.Parse(form.ExpectedSalary)
			spec := form.Specialisation
			if spec == "" {
				spec = "pharmacist"
			}
			_ = h.hrSvc.SaveJobSeekerProfile(ctx, &hr.JobSeekerProfile{
				UserID:          user.ID,
				Specialisation:  spec,
				YearsExperience: exp,
				IsOpenToWork:    true,
				ExpectedSalary:  sal,
				PreferredCityID: cityIDPtr,
				Bio:             form.Bio,
			})
			_ = cvURL
		}

		if user != nil {
			h.safeGo("notify-account-registered", func() {
				h.notifyAccountRegistered(context.Background(), user.ID, nil)
			})
			h.safeGo("notify-admins-new-registration", func() {
				h.notifyAdminsNewRegistration(context.Background(), user.ID, 0, form.Name, form.AccountType)
			})
		}

		if sess != nil {
			http.SetCookie(w, &http.Cookie{
				Name:     h.cookieName(),
				Value:    sess.Token,
				Path:     "/",
				HttpOnly: true,
				Secure:   h.secureCookie,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   86400 * 30,
			})
			http.Redirect(w, r, "/jobs", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/onboarding/pending?type=job_seeker", http.StatusSeeOther)
		return
	}

	// 2. Organization Registration (Customer / Pharmacy or Vendor / Supplier)
	_, sess, regResult, err := h.idSvc.RegisterOrganization(ctx, identity.RegisterOrganizationInput{
		Email:           form.Email,
		Password:        password,
		NameAr:          form.Name,
		NameEn:          form.Name,
		Phone:           form.Phone,
		PhoneVerifiedAt: phoneVerifiedAt,
		EmailVerifiedAt: emailVerifiedAt,
		Org: identity.RegisterOrgInput{
			Type:                form.AccountType,
			LegalName:           form.LegalName,
			TradeNameAr:         form.TradeNameAr,
			TradeNameEn:         form.TradeNameEn,
			CommercialRegister:  form.CommercialRegister,
			TaxNumber:           form.TaxNumber,
			PharmacistLicense:   form.PharmacistLicense,
			LicenseDocumentURL:  form.LicenseDocumentURL,
			LicenseOriginalName: licenseMeta.OriginalName,
			LicenseMimeType:     licenseMeta.MimeType,
			LicenseSizeBytes:    licenseMeta.SizeBytes,
			CityID:              cityIDPtr,
			BranchCount:         &branchCount,
			Address:             form.Address,
			Latitude:            latPtr,
			Longitude:           lonPtr,
			GoogleMapsURL:       form.GoogleMapsURL,
		},
	})

	if err != nil {
		h.log.WarnContext(ctx, "ui registration failed", "email", form.Email, "error", err)
		form.Error = h.safeMessage(err, lang)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if rerr := pages.RegisterPage(lang, dir, form, h.listCities(ctx), h.listGovernorates(ctx)).Render(renderContext(ctx), w); rerr != nil {
			h.log.ErrorContext(ctx, "render register page after error", "error", rerr)
		}
		return
	}

	if regResult != nil && regResult.OrganizationID > 0 {
		h.ensureCompanyRoles(database.AsSystem(ctx), regResult.OrganizationID, form.AccountType)
		if sess != nil {
			h.safeGo("notify-account-registered", func() {
				h.notifyAccountRegistered(context.Background(), sess.UserID, &regResult.OrganizationID)
			})
			h.safeGo("notify-admins-new-registration", func() {
				h.notifyAdminsNewRegistration(context.Background(), sess.UserID, regResult.OrganizationID, form.LegalName, form.AccountType)
			})
		}
	}

	if sess != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     h.cookieName(),
			Value:    sess.Token,
			Path:     "/",
			HttpOnly: true,
			Secure:   h.secureCookie,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400 * 30,
		})
	}

	http.Redirect(w, r, landingPathForSession(sess), http.StatusSeeOther)
}
