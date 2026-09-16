package i18n

// loadWhatsAppKeys holds the copy for the WhatsApp card on the settings page.
// Wording shared with the Telegram card (confirm, reject, save, categories,
// errors) uses the settings.tg_* keys.
func loadWhatsAppKeys(e *engine) {
	const ns = "settings"
	addKey(e, "settings.tab_whatsapp", ns, "واتساب", "WhatsApp", "Settings tab")
	addKey(e, "settings.wa_title", ns, "ربط واتساب", "WhatsApp connection", "Card title")
	addKey(e, "settings.wa_desc", ns,
		"استخدم المساعد كبسولة واستقبل إشعارات منشأتك على واتساب، بنفس صلاحياتك في Dawa24 تماماً. لن نطلب كلمة المرور في واتساب أبداً.",
		"Use the Capsule assistant and receive your organization's notifications on WhatsApp, with exactly your Dawa24 permissions. We never ask for your password in WhatsApp.",
		"Card description")
	addKey(e, "settings.wa_disabled", ns, "لم يُفعَّل واتساب على المنصة بعد.", "WhatsApp is not enabled on this platform yet.", "Integration off")
	addKey(e, "settings.wa_connect", ns, "ربط واتساب", "Connect WhatsApp", "Start linking button")
	addKey(e, "settings.wa_step1", ns, "افتح الرابط في واتساب واضغط «إرسال» دون تعديل الرسالة.", "Open the link in WhatsApp and press Send without editing the message.", "Step 1")
	addKey(e, "settings.wa_step2", ns, "ارجع إلى هذه الصفحة وأكّد أن رقم واتساب الظاهر هو رقمك.", "Come back to this page and confirm the WhatsApp number shown is yours.", "Step 2")
	addKey(e, "settings.wa_open", ns, "فتح واتساب", "Open WhatsApp", "Deep link button")
	addKey(e, "settings.wa_waiting", ns, "بانتظار وصول رسالة الربط من واتساب…", "Waiting for the link message from WhatsApp…", "Polling state")
	addKey(e, "settings.wa_pending_title", ns, "هل هذا رقمك في واتساب؟", "Is this your WhatsApp number?", "Confirm title")
	addKey(e, "settings.wa_pending_desc", ns,
		"أُرسل رمز الربط من الرقم التالي. أكّد فقط إذا كان رقمك؛ إن لم يكن كذلك فألغِ الطلب.",
		"The link code was sent from the number below. Confirm only if it is yours; otherwise cancel.",
		"Confirm description")
	addKey(e, "settings.wa_linked_title", ns, "واتساب مربوط بحسابك", "WhatsApp is connected", "Linked state")
	addKey(e, "settings.wa_blocked", ns, "تعذّر توصيل الرسائل إلى رقمك؛ لن تصلك رسائل حتى تراسلنا على واتساب من جديد.", "Messages could not be delivered to your number; nothing will be sent until you message us on WhatsApp again.", "Blocked state")
	addKey(e, "settings.wa_unlink_confirm", ns, "إلغاء ربط واتساب؟ سيتوقف الرد على أسئلتك وإرسال الإشعارات.", "Disconnect WhatsApp? Answers and notifications will stop.", "Unlink confirm")
	addKey(e, "settings.wa_notify_title", ns, "الإشعارات على واتساب", "Notifications on WhatsApp", "Notify section")
	addKey(e, "settings.wa_linked_ok", ns, "تم ربط واتساب بنجاح.", "WhatsApp connected.", "Linked")
	addKey(e, "settings.wa_unlinked_ok", ns, "تم إلغاء ربط واتساب.", "WhatsApp disconnected.", "Unlinked")
}
