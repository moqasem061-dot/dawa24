package i18n

// loadTelegramKeys holds the copy for the Telegram card on the settings page.
func loadTelegramKeys(e *engine) {
	const ns = "settings"
	addKey(e, "settings.tab_telegram", ns, "تيليجرام", "Telegram", "Settings tab")
	addKey(e, "settings.tg_title", ns, "ربط تيليجرام", "Telegram connection", "Card title")
	addKey(e, "settings.tg_desc", ns,
		"استخدم المساعد كبسولة واستقبل إشعارات منشأتك على تيليجرام، بنفس صلاحياتك في Dawa24 تماماً. لن نطلب كلمة المرور في تيليجرام أبداً.",
		"Use the Capsule assistant and receive your organization's notifications on Telegram, with exactly your Dawa24 permissions. We never ask for your password in Telegram.",
		"Card description")
	addKey(e, "settings.tg_disabled", ns, "لم يُفعَّل بوت تيليجرام على المنصة بعد.", "The Telegram bot is not enabled on this platform yet.", "Integration off")
	addKey(e, "settings.tg_connect", ns, "ربط تيليجرام", "Connect Telegram", "Start linking button")
	addKey(e, "settings.tg_step1", ns, "افتح الرابط في تيليجرام واضغط «ابدأ» (Start).", "Open the link in Telegram and press Start.", "Step 1")
	addKey(e, "settings.tg_step2", ns, "ارجع إلى هذه الصفحة وأكّد أن حساب تيليجرام الظاهر هو حسابك.", "Come back to this page and confirm the Telegram account shown is yours.", "Step 2")
	addKey(e, "settings.tg_open", ns, "فتح تيليجرام", "Open Telegram", "Deep link button")
	addKey(e, "settings.tg_code_expires", ns, "الرابط يعمل مرة واحدة لمدة 10 دقائق. لا تشاركه مع أحد.", "The link works once, for 10 minutes. Do not share it.", "Link validity")
	addKey(e, "settings.tg_waiting", ns, "بانتظار فتح الرابط في تيليجرام…", "Waiting for the link to be opened in Telegram…", "Polling state")
	addKey(e, "settings.tg_pending_title", ns, "هل هذا حسابك في تيليجرام؟", "Is this your Telegram account?", "Confirm title")
	addKey(e, "settings.tg_pending_desc", ns,
		"فتح حساب تيليجرام التالي رابط الربط الخاص بك. أكّد فقط إذا كان هذا حسابك؛ إن لم تكن أنت فاضغط إلغاء.",
		"The Telegram account below opened your link. Confirm only if it is yours; if it was not you, cancel.",
		"Confirm description")
	addKey(e, "settings.tg_confirm", ns, "نعم، تأكيد الربط", "Yes, confirm", "Confirm button")
	addKey(e, "settings.tg_reject", ns, "ليس حسابي — إلغاء", "Not me — cancel", "Reject button")
	addKey(e, "settings.tg_linked_title", ns, "تيليجرام مربوط بحسابك", "Telegram is connected", "Linked state")
	addKey(e, "settings.tg_blocked", ns, "البوت محظور من حسابك في تيليجرام؛ لن تصلك رسائل حتى تراسله من جديد.", "You blocked the bot; nothing will be delivered until you message it again.", "Blocked state")
	addKey(e, "settings.tg_unlink", ns, "إلغاء الربط", "Disconnect", "Unlink button")
	addKey(e, "settings.tg_unlink_confirm", ns, "إلغاء ربط تيليجرام؟ سيتوقف البوت عن الرد وعن إرسال الإشعارات.", "Disconnect Telegram? The bot will stop answering and sending notifications.", "Unlink confirm")
	addKey(e, "settings.tg_notify_title", ns, "الإشعارات على تيليجرام", "Notifications on Telegram", "Notify section")
	addKey(e, "settings.tg_notify_desc", ns, "تصلك فقط الإشعارات التي تسمح بها صلاحياتك في منشأتك.", "Only notifications your permissions allow are delivered.", "Notify description")
	addKey(e, "settings.tg_save", ns, "حفظ", "Save", "Save button")
	addKey(e, "settings.tg_too_many", ns, "طلبت روابط كثيرة. حاول بعد بضع دقائق.", "Too many links requested. Try again in a few minutes.", "Rate limited")
	addKey(e, "settings.tg_error", ns, "تعذر تنفيذ العملية. حاول مرة أخرى.", "Something went wrong. Please try again.", "Generic error")
	addKey(e, "settings.tg_confirm_expired", ns, "انتهت مهلة التأكيد. أنشئ رابطاً جديداً.", "The confirmation expired. Create a new link.", "Confirm expired")
	addKey(e, "settings.tg_saved", ns, "تم الحفظ.", "Saved.", "Saved")
	addKey(e, "settings.tg_linked_ok", ns, "تم ربط تيليجرام بنجاح.", "Telegram connected.", "Linked")
	addKey(e, "settings.tg_unlinked_ok", ns, "تم إلغاء ربط تيليجرام.", "Telegram disconnected.", "Unlinked")
	addKey(e, "settings.tg_cat_orders", ns, "الطلبات وطلبات التسعير", "Orders and quotes", "Category")
	addKey(e, "settings.tg_cat_payments", ns, "المدفوعات والمحفظة والاشتراك", "Payments, wallet and subscription", "Category")
	addKey(e, "settings.tg_cat_delivery", ns, "الشحن والتوصيل", "Shipping and delivery", "Category")
	addKey(e, "settings.tg_cat_offers", ns, "العروض والإعلانات", "Offers and ads", "Category")
	addKey(e, "settings.tg_cat_account", ns, "الحساب والمنشأة والفروع", "Account, organization and branches", "Category")
	addKey(e, "settings.tg_cat_general", ns, "إشعارات أخرى", "Other notifications", "Category")

	const authNs = "auth"
	addKey(e, "auth.telegram.send_otp", authNs, "تأكيد عبر تيليجرام", "Verify via Telegram", "Button to send Telegram OTP")
	addKey(e, "auth.telegram.resend_otp", authNs, "إعادة الإرسال", "Resend Code", "Resend button")
	addKey(e, "auth.telegram.verify_code", authNs, "تأكيد الرمز", "Verify Code", "Verify button")
	addKey(e, "auth.telegram.verified", authNs, "تم تأكيد الرقم عبر تيليجرام بنجاح", "Phone verified via Telegram", "Verified state badge")
	addKey(e, "auth.telegram.phone_required", authNs, "يرجى تأكيد رقم الهاتف عبر تيليجرام للمتابعة", "Please verify your phone number via Telegram to proceed", "Validation error")
	addKey(e, "auth.telegram.invalid_phone", authNs, "رقم الهاتف غير صالح، يرجى إدخال رقم هاتف صحيح", "Invalid phone number, please enter a valid phone number", "Validation error")
	addKey(e, "auth.telegram.invalid_code", authNs, "رمز التأكيد غير صحيح، يرجى التحقق وإعادة المحاولة", "Invalid verification code, please check and try again", "Validation error")
	addKey(e, "auth.telegram.code_sent", authNs, "تم إرسال رمز التأكيد إلى حساب التيليجرام الخاص بهذا الرقم", "Verification code sent to your Telegram account", "Success notice")
	addKey(e, "auth.telegram.cooldown", authNs, "يرجى الانتظار قبل طلب رمز تأكيد جديد", "Please wait before requesting a new code", "Cooldown notice")
	addKey(e, "auth.telegram.rate_limit", authNs, "تم تجاوز الحد المسموح لطلبات التأكيد. حاول مجدداً بعد ساعة.", "Too many requests. Please try again after an hour.", "Rate limit error")
	addKey(e, "auth.telegram.max_attempts", authNs, "تم تجاوز الحد الأقصى للمحاولات الخاطئة. يرجى طلب رمز جديد.", "Max attempts exceeded. Please request a new code.", "Attempt limit error")
	addKey(e, "auth.telegram.error_sending", authNs, "تعذر إرسال الرمز حالياً، يرجى المحاولة بعد قليل", "Failed to send code, please try again shortly", "Send error")
	addKey(e, "auth.telegram.error_verifying", authNs, "تعذر التحقق من الرمز حالياً، يرجى المحاولة لاحقاً", "Failed to verify code, please try again later", "Verify error")
	addKey(e, "auth.telegram.generic_error", authNs, "حدث خطأ غير متوقع، يرجى إعادة المحاولة", "An unexpected error occurred, please try again", "Generic error")
	addKey(e, "auth.telegram.change_phone", authNs, "تغيير الرقم", "Change number", "Change phone button")
	addKey(e, "auth.telegram.enter_code", authNs, "أدخل رمز التأكيد المكون من 6 أرقام المستلم على تيليجرام:", "Enter the 6-digit verification code received on Telegram:", "Prompt")
	addKey(e, "auth.telegram.mock_notice", authNs, "(وضع التطوير: يمكنك استخدام الرمز 123456)", "(Dev Mode: You can use code 123456)", "Dev mode notice")
}
