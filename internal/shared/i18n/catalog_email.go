package i18n

// loadEmailAuthKeys registers translations for email OTP verification and password reset.
func loadEmailAuthKeys(e *engine) {
	const ns = "auth"

	// Registration email verification
	addKey(e, "auth.email.send_otp", ns, "تأكيد البريد", "Verify Email", "Button to send email OTP")
	addKey(e, "auth.email.resend_otp", ns, "إعادة الإرسال", "Resend Code", "Resend email OTP button")
	addKey(e, "auth.email.verify_code", ns, "تأكيد الرمز", "Confirm Code", "Confirm email OTP button")
	addKey(e, "auth.email.verified", ns, "تم تأكيد البريد الإلكتروني بنجاح", "Email verified successfully", "Verified email badge")
	addKey(e, "auth.email.email_required", ns, "يرجى تأكيد البريد الإلكتروني للمتابعة", "Please verify your email to proceed", "Validation error")
	addKey(e, "auth.email.invalid_email", ns, "البريد الإلكتروني غير صالح، يرجى كتابته بشكل صحيح", "Invalid email address, please enter a valid email", "Validation error")
	addKey(e, "auth.email.already_registered", ns, "هذا البريد الإلكتروني مسجل بالفعل، يرجى تسجيل الدخول أو استخدام بريد آخر", "This email is already registered, please sign in or use another email", "Validation error")
	addKey(e, "auth.email.invalid_code", ns, "رمز التأكيد غير صحيح أو منتهي الصلاحية", "Invalid or expired verification code", "Validation error")
	addKey(e, "auth.email.code_sent", ns, "تم إرسال رمز التأكيد إلى بريدك الإلكتروني بنجاح", "Verification code sent to your email successfully", "Success notice")
	addKey(e, "auth.email.cooldown", ns, "يرجى الانتظار قبل طلب رمز تأكيد جديد", "Please wait before requesting a new code", "Cooldown notice")
	addKey(e, "auth.email.rate_limit", ns, "تم تجاوز الحد المسموح لطلبات التأكيد. حاول مجدداً بعد ساعة.", "Too many requests. Please try again after an hour.", "Rate limit error")
	addKey(e, "auth.email.max_attempts", ns, "تم تجاوز الحد الأقصى للمحاولات الخاطئة. يرجى طلب رمز جديد.", "Max attempts exceeded. Please request a new code.", "Attempt limit error")
	addKey(e, "auth.email.error_sending", ns, "تعذر إرسال البريد الإلكتروني حالياً، يرجى المحاولة بعد قليل", "Failed to send email, please try again shortly", "Send error")
	addKey(e, "auth.email.change_email", ns, "تغيير البريد", "Change email", "Change email button")
	addKey(e, "auth.email.enter_code", ns, "أدخل رمز التأكيد المكون من 6 أرقام المرسل لبريدك الإلكتروني:", "Enter the 6-digit verification code sent to your email:", "Prompt")

	// Password reset flow
	addKey(e, "auth.reset.title", ns, "استعادة كلمة المرور", "Reset Password", "Page title")
	addKey(e, "auth.reset.desc", ns, "أدخل بريدك الإلكتروني المسجل وسنرسل لك رمز تحقق لإعادة تعيين كلمة المرور", "Enter your registered email and we will send you a verification code to reset your password", "Page subtitle")
	addKey(e, "auth.reset.email_sent", ns, "إذا كان هذا البريد مسجلاً، فقد تم إرسال رمز التحقق إليه بنجاح", "If this email is registered, a verification code has been sent", "Notice after forgot submit")
	addKey(e, "auth.reset.enter_code_title", ns, "إدخال رمز التحقق", "Enter Verification Code", "Verify OTP title")
	addKey(e, "auth.reset.enter_code_desc", ns, "أدخل رمز التحقق المكون من 6 أرقام الذي تم إرساله إلى بريدك الإلكتروني", "Enter the 6-digit verification code sent to your email address", "Verify OTP subtitle")
	addKey(e, "auth.reset.new_password", ns, "كلمة المرور الجديدة", "New Password", "Input label")
	addKey(e, "auth.reset.confirm_password", ns, "تأكيد كلمة المرور الجديدة", "Confirm New Password", "Input label")
	addKey(e, "auth.reset.password_mismatch", ns, "كلمتا المرور غير متطابقتين", "Passwords do not match", "Error message")
	addKey(e, "auth.reset.save_password", ns, "حفظ كلمة المرور الجديدة", "Save New Password", "Submit button")
	addKey(e, "auth.reset.success", ns, "تم تغيير كلمة المرور بنجاح. يمكنك الآن تسجيل الدخول.", "Password reset successfully. You can now log in.", "Success message")
	addKey(e, "auth.reset.invalid_token", ns, "رابط أو رمز إعادة التعيين غير صالح أو منتهي الصلاحية. يرجى البدء من جديد.", "Reset link or token is invalid or expired. Please start over.", "Error message")
}
