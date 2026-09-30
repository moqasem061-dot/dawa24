package i18n

func loadUploadSecurityKeys(e *engine) {
	addKey(e, "chunk.too_large", "upload", "يتجاوز الملف الحد الأقصى البالغ ٢٠٠ ميجابايت أو يتجاوز الجزء ١٠ ميجابايت.", "Upload exceeds the 200 MB total or 10 MB chunk limit.", "Chunk upload size validation")
	addKey(e, "upload.filename_invalid", "upload", "اسم الملف غير صالح.", "Invalid upload filename.", "Upload filename validation")
	addKey(e, "upload.storage_key_invalid", "upload", "يجب أن يكون الملف تابعاً لمنشأتك الحالية.", "Upload must belong to the current organization.", "Upload ownership validation")
}
