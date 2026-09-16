package i18n

func loadChatAndAttachmentsKeys(e *engine) {
	// --- Chat & Instant Messaging ---
	addKey(e, "chat.title", "chat", "المحادثات والتواصل الفوري", "Chat & Direct Messages", "Chat page title")
	addKey(e, "chat.subtitle", "chat", "قنوات اتصال مباشرة وآمنة بين الصيدليات والموردين لتنسيق الطلبيات والاستفسارات.", "Secure real-time communication between pharmacies and suppliers for order coordination.", "Chat subtitle")
	addKey(e, "chat.search_conversations", "chat", "بحث في المحادثات...", "Search conversations...", "Chat search placeholder")
	addKey(e, "chat.type_message", "chat", "اكتب رسالتك هنا...", "Type your message here...", "Chat input placeholder")
	addKey(e, "chat.send", "chat", "إرسال", "Send", "Send chat button")
	addKey(e, "chat.online", "chat", "متصل الآن", "Online", "Online status")
	addKey(e, "chat.offline", "chat", "غير متصل", "Offline", "Offline status")
	addKey(e, "chat.no_conversations", "chat", "لا توجد محادثات سابقة", "No conversation history", "Empty chat state")
	addKey(e, "chat.unread_messages", "chat", "%d رسائل جديدة غير مقروءة", "%d new unread messages", "Unread counter")
	addKey(e, "chat.attach_file", "chat", "إرفاق ملف أو صورة", "Attach File / Image", "Attachment tooltip")

	// --- Attachments & Verification Documents ---
	addKey(e, "attachment.title", "attachments", "المستندات والملفات المرفقة", "Documents & Attachments", "Attachments title")
	addKey(e, "attachment.upload_document", "attachments", "+ رفع وثيقة جديدة", "+ Upload New Document", "Upload document button")
	addKey(e, "attachment.commercial_register_doc", "attachments", "مستخرج السجل التجاري الحديث", "Commercial Registration Document", "Commercial register doc")
	addKey(e, "attachment.tax_card_doc", "attachments", "البطاقة الضريبية", "Tax Card Document", "Tax card doc")
	addKey(e, "attachment.pharmacy_license_doc", "attachments", "ترخيص الصيدلية / المنشأة", "Pharmacy / Facility License", "Facility license doc")
	addKey(e, "attachment.pharmacist_card_doc", "attachments", "كارنيه نقابة الصيادلة / مزاولة المهنة", "Syndicate ID / Practice License", "Syndicate card doc")
	addKey(e, "attachment.document_title", "attachments", "عنوان الوثيقة", "Document Title", "Document title field")
	addKey(e, "attachment.document_type", "attachments", "نوع الوثيقة", "Document Type", "Document type field")
	addKey(e, "attachment.upload_date", "attachments", "تاريخ الرفع", "Upload Date", "Upload date column")
	addKey(e, "attachment.file_size", "attachments", "حجم الملف", "File Size", "File size column")
	addKey(e, "attachment.download", "attachments", "تحميل الملف", "Download File", "Download action")
	addKey(e, "attachment.delete", "attachments", "حذف الوثيقة", "Delete Document", "Delete action")
	addKey(e, "attachment.drag_drop", "attachments", "اسحب الملف وأفلته هنا، أو اضغط للاختيار", "Drag and drop file here, or click to browse", "Drag drop prompt")
	addKey(e, "attachment.max_size_hint", "attachments", "الحد الأقصى لحجم الملف: 15 ميجابايت (PDF, JPG, PNG)", "Max file size: 15MB (PDF, JPG, PNG)", "Max size hint")
	addKey(e, "chat.org_required_to_message", "chat", "تحتاج إلى حساب مؤسسة معتمد لإرسال الرسائل.", "You need a verified organization account to send messages.", "Validation error")
	addKey(e, "chat.cannot_start_conversation", "chat", "تعذر بدء المحادثة.", "Unable to start conversation.", "Chat error")
}
