package i18n

func loadErrorsAndValidationKeys(e *engine) {
	// --- Validation Messages ---
	addKey(e, "validation.field_required", "validation", "هذا الحقل إلزامي ولا يمكن تركه فارغاً.", "This field is required and cannot be empty.", "Required validation")
	addKey(e, "validation.invalid_phone", "validation", "يرجى إدخال رقم هاتف صحيح (مثال: 01012345678).", "Please enter a valid mobile phone number.", "Invalid phone validation")
	addKey(e, "validation.invalid_national_id", "validation", "الرقم القومي المصري يجب أن يتكون من 14 رقماً صحيحاً.", "National ID must be exactly 14 digits.", "Invalid national ID")
	addKey(e, "validation.password_too_short", "validation", "كلمة المرور يجب ألا تقل عن 8 أحرف وأرقام.", "Password must be at least 8 characters long.", "Password short validation")
	addKey(e, "validation.password_mismatch", "validation", "كلمتا المرور غير متطابقتين.", "Passwords do not match.", "Password mismatch")
	addKey(e, "validation.invalid_coords", "validation", "يرجى تحديد إحداثيات جغرافية صحيحة (خط الطول وخط العرض).", "Please provide valid GPS coordinates.", "Invalid GPS coordinates")
	addKey(e, "validation.invalid_url", "validation", "يرجى إدخال رابط إنترنت صحيح يبدأ بـ https://", "Please enter a valid URL starting with https://", "Invalid URL")
	addKey(e, "validation.min_order_exceeded", "validation", "قيمة الطلب أقل من الحد الأدنى للشراء المحدد من المورد (%s ج.م).", "Order total is below the minimum required by supplier (%s EGP).", "Min order validation")
	addKey(e, "validation.stock_insufficient", "validation", "الكمية المطلوبة من الصنف (%s) تتجاوز الرصيد المتاح حالياً بالمخزن (%d).", "Requested quantity for (%s) exceeds available stock (%d).", "Insufficient stock error")
	addKey(e, "validation.out_of_coverage", "validation", "عنوان التوصيل المحدد يقع خارج نطاق تغطية هذا المورد.", "The selected delivery address is outside supplier's coverage zone.", "Out of coverage validation")

	// --- HTTP & System Errors ---
	addKey(e, "errors.400_bad_request", "errors", "طلب غير صالح، يرجى التحقق من صحة المدخلات.", "Bad request, please verify input values.", "400 error")
	addKey(e, "errors.401_unauthorized", "errors", "يرجى تسجيل الدخول للوصول إلى هذه الصفحة.", "Unauthorized, please sign in first.", "401 error")
	addKey(e, "errors.403_forbidden", "errors", "غير مصرح لك بتنفيذ هذا الإجراء أو الوصول لهذه الصفحة.", "Forbidden, you lack permission for this action.", "403 error")
	addKey(e, "errors.404_not_found", "errors", "العنصر أو المسار المطلوب غير موجود.", "Not found, the requested resource does not exist.", "404 error")
	addKey(e, "errors.409_conflict", "errors", "تعارض في البيانات، قد يكون السجل مسجلاً مسبقاً في النظام.", "Conflict, this record might already exist.", "409 error")
	addKey(e, "errors.429_rate_limited", "errors", "تم تجاوز الحد المسموح به من الطلبات، يرجى الانتظار قليلاً.", "Too many requests, please slow down.", "429 error")
	addKey(e, "errors.500_internal", "errors", "حدث خطأ غير متوقع في الخادم. تم تسجيل الخطأ للفحص.", "Internal server error. The incident has been logged.", "500 error")
	addKey(e, "errors.database_error", "errors", "تعذر الاتصال بقاعدة البيانات حالياً.", "Database connection error.", "Database error")
	addKey(e, "errors.ai_gateway_error", "errors", "تعذر استجابة خادم الذكاء الاصطناعي حالياً، يرجى المحاولة لاحقاً.", "AI Gateway is currently unavailable, please try again later.", "AI Gateway error")
	addKey(e, "errors.foreign_key_mismatch_format", "errors", "تعذر إتمام العملية بسبب عدم تطابق البيانات المرجعية (%s).", "Operation failed due to reference data mismatch (%s).", "Reference error")
	addKey(e, "errors.processing_error_format", "errors", "حدث خطأ أثناء المعالجة: %s", "An error occurred during processing: %s", "Processing error")
	addKey(e, "errors.data_load_failed", "errors", "حدث خطأ أثناء تحميل البيانات", "Error loading data", "Error title")
	addKey(e, "errors.generic_title", "errors", "عذراً، حدث خطأ", "Sorry, an error occurred", "Error page title")
	addKey(e, "common.retry", "common", "إعادة المحاولة", "Retry", "Button label")

	addKey(e, "validation.email_already_registered", "validation", "البريد الإلكتروني مسجل مسبقاً في النظام. يرجى تسجيل الدخول أو استخدام بريد آخر.", "Email is already registered in the system. Please sign in or use another email.", "Email exists error")
	addKey(e, "validation.cr_already_registered", "validation", "رقم السجل التجاري مسجل مسبقاً لمنشأة أخرى.", "Commercial register number is already registered for another organization.", "CR exists error")
	addKey(e, "validation.city_invalid", "validation", "بيانات الموقع أو المدينة غير صالحة. يرجى إعادة اختيار المدينة من الخريطة.", "Location or city data is invalid. Please reselect city from map.", "City invalid error")
	addKey(e, "validation.supplier_missing_cart", "validation", "تعذر تحديد بيانات شركة التوريد المسؤولة عن هذا الصنف (رمز المورد غير مسجل). يرجى مراجعة الأصناف بالسلة.", "Could not determine supplier company for this item. Please review cart items.", "Supplier missing error")
	addKey(e, "validation.pharmacy_branch_invalid", "validation", "فرع الصيدلية المحدد غير صالح أو تم حذفه. يرجى اختيار فرع صيدلية نشط.", "Selected pharmacy branch is invalid or deleted. Please select an active branch.", "Branch invalid error")
	addKey(e, "validation.vendor_branch_invalid", "validation", "فرع التوريد المحدد للمورد غير صالح أو غير مسجل.", "Selected supplier branch is invalid or unregistered.", "Vendor branch invalid error")

	// --- Anti-scraping refusals (internal/platform/antiscrape) ---
	// Deliberately vague. A refused caller is told what to do next, never which
	// signal caught it: naming the signal is telling a scraper what to change.
	addKey(e, "security.antiscrape.blocked_title", "security", "تعذر عرض هذه الصفحة", "This page could not be shown", "Anti-scraping block page title")
	addKey(e, "security.antiscrape.blocked_body", "security", "تم رفض هذا الطلب تلقائياً. إذا كنت تتصفح من متصفح عادي يرجى مراسلة الدعم الفني، وإذا كنت تبني تكاملاً برمجياً فاستخدم واجهة البرمجة الموثقة بدلاً من قراءة الصفحات.", "This request was refused automatically. If you are browsing from an ordinary browser, please contact support; if you are building an integration, use the documented API rather than reading the pages.", "Anti-scraping block page body")
	addKey(e, "security.antiscrape.throttled_title", "security", "عدد الطلبات كبير جداً", "Too many requests", "Anti-scraping throttle page title")
	addKey(e, "security.antiscrape.throttled_body", "security", "وصلت منك طلبات أكثر مما يسمح به النظام خلال فترة قصيرة. يرجى الانتظار قليلاً ثم المحاولة مرة أخرى. تسجيل الدخول يرفع هذا الحد.", "You have sent more requests than the system allows in a short period. Please wait a moment and try again. Signing in raises this limit.", "Anti-scraping throttle page body")
	addKey(e, "security.antiscrape.use_the_api", "security", "استخدم واجهة البرمجة (API)", "Use the API", "Anti-scraping page action pointing integrators at the documented API")
	addKey(e, "security.antiscrape.back_home", "security", "العودة للصفحة الرئيسية", "Back to home", "Anti-scraping page action")
	addKey(e, "catalog.guest_depth_notice", "catalog", "لعرض نتائج أكثر والوصول إلى كامل الكتالوج، يرجى تسجيل الدخول.", "Sign in to browse deeper into the catalogue.", "Guest pagination cap notice")
}
