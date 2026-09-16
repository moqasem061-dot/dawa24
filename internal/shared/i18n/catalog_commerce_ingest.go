package i18n

func loadCommerceAndIngestKeys(e *engine) {
	// --- Ingest & Vendor Catalog ---
	addKey(e, "ingest.ai_match_prefix", "ingest", "مطابقة بالذكاء الاصطناعي: ", "AI match: ", "Prefix on an import row matched by the AI stage")
	addKey(e, "ingest.ai_dispute", "ingest", "المراجعة الذكية لم تؤكد هذا التطابق؛ يلزم التأكيد يدوياً.", "The AI review did not confirm this match; please confirm it manually.", "Shown on an import row where the deterministic matcher and the AI review disagreed")
	addKey(e, "ingest.upload_file", "ingest", "رفع الملف", "Upload File", "Step 1 of catalog import")
	addKey(e, "ingest.column_mapping", "ingest", "ربط الأعمدة والتحقق", "Column Mapping & Validation", "Step 2 of catalog import")
	addKey(e, "ingest.settings", "ingest", "إعدادات الاستيراد", "Import Settings", "Step 3 of catalog import")
	addKey(e, "ingest.review", "ingest", "مراجعة الأصناف والمطابقة", "Review Items & Matching", "Step 4 of catalog import")
	addKey(e, "ingest.results", "ingest", "النتائج النهائية", "Final Results", "Step 5 of catalog import")
	addKey(e, "ingest.select_warehouse", "ingest", "اختر المخزن", "Select Warehouse", "Warehouse selection dropdown")
	addKey(e, "ingest.warehouse_target", "ingest", "المخزن الذي ستُسجَّل فيه الأرصدة والكميات المستوردة", "Target Warehouse for Stock Balances", "Warehouse selection label")
	addKey(e, "ingest.warehouse_note", "ingest", "سيتم إيداع كميات جميع الأصناف المستوردة والمطابقة مباشرة في رصيد هذا المخزن.", "Imported stock quantities will be deposited directly into this warehouse.", "Warehouse note")
	addKey(e, "ingest.core_fields_status", "ingest", "حالة الحقول الأساسية والمخزون", "Core Fields & Stock Status", "Core fields title")
	addKey(e, "ingest.required_core_field", "ingest", "إلزامي / أساسي", "Required / Core", "Required badge")
	addKey(e, "ingest.mapped_to", "ingest", "مربوط بـ", "Mapped to", "Mapped badge prefix")
	addKey(e, "ingest.unmapped_warning", "ingest", "لم يُربط تلقائياً — حدده من الجدول أدناه", "Not auto-mapped — select from table below", "Unmapped warning")
	addKey(e, "ingest.stock_quantity", "ingest", "الكمية (رصيد المخزون)", "Stock Quantity", "Quantity field name")
	addKey(e, "ingest.unit_price", "ingest", "سعر الجمهور", "Public Price", "Price field name")
	addKey(e, "ingest.item_name", "ingest", "اسم الصنف", "Item Name", "Name field name")
	addKey(e, "ingest.item_sku", "ingest", "كود الصنف (SKU)", "Item SKU", "SKU field name")
	addKey(e, "ingest.barcode", "ingest", "الباركود الدولي", "International Barcode", "Barcode field name")
	addKey(e, "ingest.expiry_date", "ingest", "تاريخ الصلاحية", "Expiry Date", "Expiry date field name")
	addKey(e, "ingest.discount_percent", "ingest", "نسبة الخصم %", "Discount %", "Discount field name")
	addKey(e, "ingest.save_mapping_and_continue", "ingest", "اعتماد الربط والمتابعة ←", "Approve Mapping & Continue →", "Submit button")

	// --- Commit stage: what the run did, and what it deliberately did not ---
	addKey(e, "ingest.commit.retire_skipped_empty", "ingest",
		"لم يتم إيقاف عرض أي صنف: لم يُحفظ أي صنف من هذا الملف، ولا يصح اعتبار ملف لم يُكتب منه شيء كتالوجاً كاملاً.",
		"Nothing was delisted: this run saved no items at all, and a file that wrote nothing cannot be treated as a whole catalog.",
		"Replace-mode retirement refused: the commit wrote nothing")
	addKey(e, "ingest.commit.retired_format", "ingest",
		"تم إزالة مخزون %d صنف غير موجود بالملف من المخزن المختار بالكامل مع الحفاظ على أصناف الكتالوج.",
		"%d items absent from this file had their stock completely removed from the chosen warehouse while preserving master catalog variants.",
		"Replace-mode retirement summary")
	addKey(e, "ingest.commit.retire_failed", "ingest",
		"تم حفظ الأصناف، لكن تعذّر إيقاف عرض الأصناف الغائبة عن الملف. راجع كتالوجك يدوياً.",
		"Items were saved, but the absent items could not be delisted. Please review your catalog manually.",
		"Replace-mode retirement failed at the database")
	addKey(e, "ingest.commit.row_errors_format", "ingest",
		"تعذّر حفظ %d صنف. افتح تبويب «أخطاء» لمعرفة السبب لكل صف.",
		"%d items could not be saved. Open the \"Errors\" tab to see why for each row.",
		"Commit finished with per-row write failures")
	addKey(e, "ingest.commit.running", "ingest",
		"جارٍ حفظ الأصناف في الكتالوج والمخزن…",
		"Saving items into the catalog and warehouse…",
		"Progress note while a commit is running")
	addKey(e, "ingest.commit.failed", "ingest",
		"تعذّر إكمال حفظ الأصناف. لم يكتمل الاستيراد، ويمكنك إعادة المحاولة من شاشة المراجعة.",
		"Saving the items could not be completed. The import did not finish; you can retry from the review screen.",
		"A detached commit run failed")

	// --- Commerce & Orders ---
	addKey(e, "commerce.order_id", "commerce", "رقم الطلب", "Order ID", "Order ID label")
	addKey(e, "commerce.order_date", "commerce", "تاريخ الطلب", "Order Date", "Order date")
	addKey(e, "commerce.order_status", "commerce", "حالة الطلب", "Order Status", "Order status")
	addKey(e, "commerce.total_amount", "commerce", "إجمالي الطلب", "Total Amount", "Total amount")
	addKey(e, "commerce.subtotal", "commerce", "المجموع الفرعي", "Subtotal", "Subtotal")
	addKey(e, "commerce.discount", "commerce", "الخصم", "Discount", "Discount amount")
	addKey(e, "commerce.shipping_fee", "commerce", "رسوم التوصيل", "Shipping Fee", "Shipping fee")
	addKey(e, "commerce.tax", "commerce", "الضريبة", "Tax", "Tax amount")
	addKey(e, "commerce.payment_method", "commerce", "طريقة الدفع", "Payment Method", "Payment method")
	addKey(e, "commerce.payment_status", "commerce", "حالة الدفع", "Payment Status", "Payment status")
	addKey(e, "commerce.cash_on_delivery", "commerce", "الدفع عند الاستلام", "Cash on Delivery", "COD option")
	addKey(e, "commerce.credit_terms", "commerce", "آجل (تسهيلات ائتمانية)", "Credit Terms", "Credit terms option")
	addKey(e, "commerce.card_payment", "commerce", "بطاقة بنكية", "Debit/Credit Card", "Card payment")
	addKey(e, "commerce.cart", "commerce", "سلة المشتريات", "Shopping Cart", "Cart title")
	addKey(e, "commerce.checkout", "commerce", "إتمام الطلب", "Checkout", "Checkout button")
	addKey(e, "commerce.order_details", "commerce", "تفاصيل الطلب", "Order Details", "Order details page title")
	addKey(e, "commerce.shipments", "commerce", "الشحنات والتسليم", "Shipments & Delivery", "Shipments section")
	addKey(e, "commerce.shipment_number", "commerce", "رقم الشحنة", "Shipment Number", "Shipment number")
	addKey(e, "commerce.vendor", "commerce", "المورد", "Vendor", "Vendor label")
	addKey(e, "commerce.customer", "commerce", "الصيدلية / العميل", "Customer / Pharmacy", "Customer label")
	addKey(e, "commerce.invoices", "commerce", "الفواتير والمطالبات", "Invoices & Billing", "Invoices page title")
	addKey(e, "commerce.invoice_number", "commerce", "رقم الفاتورة", "Invoice Number", "Invoice number")

	// --- Status Labels ---
	addKey(e, "status.pending", "status", "قيد المراجعة", "Pending", "Pending")
	addKey(e, "status.confirmed", "status", "تم التأكيد", "Confirmed", "Confirmed")
	addKey(e, "status.processing", "status", "جاري التجهيز", "Processing", "Processing")
	addKey(e, "status.shipped", "status", "تم الشحن", "Shipped", "Shipped")
	addKey(e, "status.delivered", "status", "تم التسليم", "Delivered", "Delivered")
	addKey(e, "status.cancelled", "status", "ملغي", "Cancelled", "Cancelled")
	addKey(e, "status.unpaid", "status", "غير مدفوع", "Unpaid", "Unpaid")
	addKey(e, "status.paid", "status", "مدفوع", "Paid", "Paid")

	// --- Ingest Review & Actions ---
	addKey(e, "vendor.ingest.invalid_row_id", "vendor", "معرّف الصف غير صالح.", "Invalid row ID.", "Validation error")
	addKey(e, "vendor.ingest.item_updated_success", "vendor", "تم تحديث بيانات الصنف بنجاح.", "Item data updated successfully.", "Success notice")
	addKey(e, "common.import_service_unavailable", "common", "خدمة الاستيراد غير متاحة حالياً.", "Import service is currently unavailable.", "Service error")
	addKey(e, "vendor.ingest.enter_valid_quantity", "vendor", "يرجى إدخال كمية صحيحة أكبر من أو تساوي الصفر.", "Please enter a valid quantity greater than or equal to zero.", "Validation error")
	addKey(e, "vendor.ingest.batch_quantity_success", "vendor", "تم تعيين الكمية (%d) لجميع أصناف الملف بنجاح.", "Quantity (%d) set for all file items successfully.", "Success notice")
	addKey(e, "vendor.ingest.item_linked_success", "vendor", "تم ربط الصنف بالكتالوج المركزي بنجاح.", "Item linked to master catalog successfully.", "Success notice")
	addKey(e, "vendor.ingest.item_unlinked_success", "vendor", "تم إلغاء ربط الصنف بالكتالوج.", "Item unlinked from master catalog.", "Success notice")
	addKey(e, "vendor.ingest.commit_success", "vendor", "تم بنجاح حفظ %d صنفاً في الكتالوج والمخزن.", "Successfully saved %d items in catalog and warehouse.", "Success notice")
	addKey(e, "vendor.ingest.commit_skipped", "vendor", " ولم يُحفظ %d صنف لأنه ما زال يحتاج مراجعة أو غير مطابق — يمكنك اعتماده من شاشة المراجعة.", " and %d items were not saved because they still need review or are unmatched — you can approve them from the review screen.", "Notice suffix")

	// --- Vendor Catalog Operations ---
	addKey(e, "vendor.catalog.select_master_product_first", "vendor", "يرجى اختيار صنف من الكتالوج العام أولاً.", "Please select an item from the master catalog first.", "Validation error")
	addKey(e, "vendor.catalog.default_variant_name", "vendor", "صنف توريد معتمد", "Verified Supply Item", "Default variant name")
	addKey(e, "common.catalog_service_unavailable", "common", "خدمة الكتالوج غير متاحة حالياً.", "Catalog service is currently unavailable.", "Service error")
	addKey(e, "vendor.catalog.add_variant_error_prefix", "vendor", "حدث خطأ أثناء إضافة الصنف: ", "An error occurred while adding the item: ", "Error prefix")
	addKey(e, "vendor.catalog.variant_added_success", "vendor", "تمت إضافة الصنف من الكتالوج العام ونشره في قائمة أصناف التوريد بنجاح.", "Item added from master catalog and published to your supply list successfully.", "Success notice")
	addKey(e, "vendor.catalog.invalid_variant_id", "vendor", "معرف الصنف غير صالح.", "Invalid item ID.", "Validation error")
	addKey(e, "vendor.catalog.invalid_price", "vendor", "قيمة سعر غير صالحة. استخدم أرقاماً فقط، مثال: 125.50", "Invalid price. Use digits only, for example 125.50", "Validation error")
	addKey(e, "vendor.catalog.invalid_discount", "vendor", "نسبة الخصم يجب أن تكون بين 0 و 100.", "The discount must be between 0 and 100.", "Validation error")
	addKey(e, "vendor.catalog.invalid_min_qty", "vendor", "الحد الأدنى للطلب يجب أن يكون رقماً أكبر من صفر.", "The minimum order quantity must be greater than zero.", "Validation error")
	addKey(e, "vendor.catalog.invalid_expiry", "vendor", "تاريخ الصلاحية غير صالح. استخدم صيغة سنة-شهر-يوم.", "Invalid expiry date. Use the year-month-day format.", "Validation error")
	addKey(e, "vendor.catalog.invalid_status", "vendor", "حالة الصنف غير صالحة.", "Invalid item status.", "Validation error")
	addKey(e, "vendor.catalog.duplicate_sku", "vendor", "كود الصنف (SKU) مستخدم بالفعل لصنف آخر في منشأتك.", "That SKU already belongs to another item in your organization.", "Validation error")
	addKey(e, "vendor.catalog.variant_not_found", "vendor", "لم يتم العثور على الصنف المطلوب تعديله.", "The requested item to edit was not found.", "Not found error")
	addKey(e, "vendor.catalog.update_variant_error_prefix", "vendor", "حدث خطأ أثناء تعديل الصنف: ", "An error occurred while updating the item: ", "Error prefix")
	addKey(e, "vendor.catalog.variant_updated_success", "vendor", "تم تحديث بيانات وسعر صنف التوريد بنجاح.", "Supply item data and price updated successfully.", "Success notice")
	addKey(e, "vendor.catalog.delete_variants_error_prefix", "vendor", "حدث خطأ أثناء حذف الأصناف: ", "An error occurred while deleting items: ", "Error prefix")
	addKey(e, "vendor.catalog.deleted_all_success", "vendor", "تم حذف %d من أصناف التوريد الخاصة بك بنجاح.", "Successfully deleted %d of your supply items.", "Success notice")

	// --- Vendor Orders & Negotiations ---
	addKey(e, "vendor.orders.update_shipment_status_error_prefix", "vendor", "تعذر تحديث حالة الشحنة: ", "Failed to update shipment status: ", "Error prefix")
	addKey(e, "vendor.orders.shipment_status_updated_success", "vendor", "تم تحديث حالة الشحنة بنجاح.", "Shipment status updated successfully.", "Success notice")
	addKey(e, "vendor.orders.order_not_found", "vendor", "الطلب غير موجود.", "Order not found.", "Not found error")
	addKey(e, "vendor.orders.unauthorized_order_management", "vendor", "غير مصرح لك بإدارة هذا الطلب.", "You are not authorized to manage this order.", "Auth error")
	addKey(e, "vendor.orders.accept_negotiation_error_prefix", "vendor", "تعذر قبول التفاوض: ", "Failed to accept negotiation: ", "Error prefix")
	addKey(e, "vendor.orders.negotiation_accepted_title", "vendor", "تم قبول السعر المتفاوض عليه للطلب #%s", "Negotiated price accepted for order #%s", "Notification title")
	addKey(e, "vendor.orders.negotiation_accepted_body", "vendor", "قام %s بقبول السعر واعتماد طلب التوريد بنجاح.", "%s accepted the price and confirmed the supply order.", "Notification body")
	addKey(e, "vendor.orders.negotiation_accepted_success", "vendor", "تم قبول السعر المتفاوض عليه واعتماد الطلب بنجاح.", "Negotiated price accepted and order confirmed successfully.", "Success notice")
	addKey(e, "vendor.orders.negotiation_default_reject_reason", "vendor", "تم رفض السعر المقترح من قبل إدارة المبيعات", "Proposed price was rejected by sales management", "Default reject reason")
	addKey(e, "vendor.orders.reject_negotiation_error_prefix", "vendor", "تعذر رفض التفاوض: ", "Failed to reject negotiation: ", "Error prefix")
	addKey(e, "vendor.orders.negotiation_rejected_title", "vendor", "تم رفض السعر المقترح للطلب #%s", "Proposed price rejected for order #%s", "Notification title")
	addKey(e, "vendor.orders.negotiation_rejected_body", "vendor", "تم رفض السعر المقترح من قِبل %s. السبب: %s", "Proposed price was rejected by %s. Reason: %s", "Notification body")
	addKey(e, "vendor.orders.negotiation_rejected_success", "vendor", "تم رفض طلب التفاوض وإلغاء الطلب.", "Negotiation request rejected and order cancelled.", "Success notice")

	// --- Ingest Models & Fields ---
	addKey(e, "ingest.field.name", "ingest", "اسم الصنف", "Product Name", "Product name field")
	addKey(e, "ingest.field.name_desc", "ingest", "الاسم التجاري أو الوصف في الملف", "Commercial name or description in file", "Product name desc")
	addKey(e, "ingest.field.price", "ingest", "سعر الجمهور", "Public Price", "Price field")
	addKey(e, "ingest.field.price_desc", "ingest", "سعر الجمهور الرسمي للصنف", "Official public retail price for the product", "Price desc")
	addKey(e, "ingest.field.quantity", "ingest", "الكمية (رصيد المخزون)", "Quantity (Stock Balance)", "Quantity field")
	addKey(e, "ingest.field.quantity_desc", "ingest", "الكمية المتوفرة لتسجيلها في المستودع المحدد", "Available quantity to record in the selected warehouse", "Quantity desc")
	addKey(e, "ingest.field.sku", "ingest", "كود الصنف (SKU)", "Product SKU Code", "SKU field")
	addKey(e, "ingest.field.sku_desc", "ingest", "رمز الصنف الداخلي لديك", "Your internal item code", "SKU desc")
	addKey(e, "ingest.field.barcode", "ingest", "الباركود الدولي", "International Barcode", "Barcode field")
	addKey(e, "ingest.field.barcode_desc", "ingest", "رقم الباركود للمطابقة التلقائية الفورية", "Barcode for instant automatic matching", "Barcode desc")
	addKey(e, "ingest.field.expiry", "ingest", "تاريخ الصلاحية", "Expiry Date", "Expiry field")
	addKey(e, "ingest.field.expiry_desc", "ingest", "تاريخ انتهاء صلاحية التشغيلة", "Batch expiration date", "Expiry desc")
	addKey(e, "ingest.field.discount", "ingest", "نسبة الخصم %", "Discount %", "Discount field")
	addKey(e, "ingest.field.discount_desc", "ingest", "نسبة الخصم الممنوحة على الصنف", "Discount percentage on the item", "Discount desc")
	addKey(e, "ingest.field.column_n", "ingest", "العمود %d", "Column %d", "Column number")
	addKey(e, "ingest.outcome.inserted", "ingest", "تمت الإضافة", "Inserted", "Outcome inserted")
	addKey(e, "ingest.outcome.updated", "ingest", "تم التحديث", "Updated", "Outcome updated")
	addKey(e, "ingest.outcome.error", "ingest", "خطأ", "Error", "Outcome error")
	addKey(e, "ingest.outcome.skipped", "ingest", "تم التخطي", "Skipped", "Outcome skipped")
	addKey(e, "common.file_size_mb", "common", "%.1f ميجابايت", "%.1f MB", "File size MB")
	addKey(e, "common.file_size_kb", "common", "%.0f كيلوبايت", "%.0f KB", "File size KB")
	addKey(e, "common.file_size_bytes", "common", "%d بايت", "%d B", "File size bytes")

	// --- Ingest Upload & Execution ---
	addKey(e, "vendor.ingest.file_too_large", "vendor", "تعذر قراءة الملف المرفوع — قد يتجاوز حجمه الحد المسموح (25 ميجابايت).", "Unable to read uploaded file — it may exceed the size limit (25 MB).", "Upload error")
	addKey(e, "vendor.ingest.invalid_file_format", "vendor", "يرجى اختيار ملف صالح للاستيراد (.xlsx أو .xls أو CSV).", "Please choose a valid file for import (.xlsx, .xls, or CSV).", "Upload error")
	addKey(e, "vendor.ingest.read_file_error", "vendor", "تعذر قراءة محتوى الملف.", "Unable to read file content.", "Upload error")
	addKey(e, "vendor.ingest.main_warehouse", "vendor", "المخزن الرئيسي", "Main Warehouse", "Default warehouse name")
	addKey(e, "vendor.ingest.cancelled_notice", "vendor", "تم إلغاء عملية الاستيراد.", "Import process cancelled.", "Notice")
	addKey(e, "ingest.csv.row_number", "ingest", "رقم الصف", "Row Number", "CSV column")
	addKey(e, "ingest.csv.item_name", "ingest", "اسم الصنف في الملف", "File Item Name", "CSV column")
	addKey(e, "ingest.csv.matched_catalog_item", "ingest", "الصنف المطابق المعتمد بالكتالوج", "Matched Catalog Item", "CSV column")
	addKey(e, "ingest.csv.item_code", "ingest", "كود الصنف بالملف", "File Item Code", "CSV column")
	addKey(e, "ingest.csv.outcome", "ingest", "النتيجة", "Outcome", "CSV column")
	addKey(e, "ingest.csv.match_score", "ingest", "درجة المطابقة", "Match Score", "CSV column")
	addKey(e, "ingest.csv.notes", "ingest", "الملاحظة", "Note", "CSV column")

	// --- Ingest Bulk Operations ---
	addKey(e, "vendor.ingest.no_items_selected", "vendor", "لم يتم تحديد أي صنف. اختر الأصناف أولاً ثم كرّر العملية.", "No items selected. Please select items first and try again.", "Validation error")
	addKey(e, "vendor.ingest.bulk_confirmed", "vendor", "تم اعتماد %d صنف كمطابق ونقلها إلى قائمة الاستيراد.", "Confirmed %d items as matched and moved to import list.", "Success notice")
	addKey(e, "vendor.ingest.bulk_confirmed_skipped", "vendor", " %d صنف لم يُعتمد لعدم وجود صنف مقترح — اربطه يدوياً أولاً.", " %d items were not confirmed due to no suggested match — link them manually first.", "Notice suffix")
	addKey(e, "vendor.ingest.bulk_unlinked", "vendor", "تم إلغاء الربط عن %d صنف.", "Unlinked %d items.", "Success notice")
	addKey(e, "vendor.ingest.bulk_excluded", "vendor", "تم استبعاد %d صنف من الاستيراد.", "Excluded %d items from import.", "Success notice")
	addKey(e, "vendor.ingest.bulk_included", "vendor", "تمت إعادة %d صنف إلى الاستيراد.", "Restored %d items to import.", "Success notice")
	addKey(e, "vendor.ingest.unknown_action", "vendor", "إجراء غير معروف.", "Unknown action.", "Action error")

	// --- Vendor Variants ---
	addKey(e, "vendor.variant.initial_stock_warning", "vendor", "تم نشر الصنف، لكن تعذر تسجيل الكمية الافتتاحية. يرجى إضافة مستودع أولاً ثم تحديد الكمية من صفحة المخزون.", "Item published, but opening stock could not be recorded. Please add a warehouse first then set quantity in the inventory page.", "Warning notice")
	addKey(e, "vendor.variant.published_success", "vendor", "تم نشر عرض التوريد بنجاح في الكتالوج.", "Supply offer published successfully in catalog.", "Success notice")
	addKey(e, "vendor.variant.delete_error_prefix", "vendor", "تعذر حذف الصنف: ", "Failed to delete item: ", "Error prefix")
	addKey(e, "vendor.variant.deleted_success", "vendor", "تم حذف الصنف ومسح الأرصدة المرتبطة به بالمخازن بنجاح.", "Item deleted and associated warehouse stock cleared successfully.", "Success notice")

	// --- Vendor Purchase Requests ---
	addKey(e, "vendor.purchase_request.invalid_id", "vendor", "معرف طلب غير صالح.", "Invalid purchase request ID.", "Validation error")
	addKey(e, "vendor.purchase_request.not_found", "vendor", "طلب الشراء غير موجود.", "Purchase request not found.", "Not found error")
	addKey(e, "vendor.purchase_request.status_updated_success", "vendor", "تم تحديث حالة طلب الشراء بنجاح.", "Purchase request status updated successfully.", "Success notice")
	addKey(e, "vendor.purchase_request.invalid_line_id", "vendor", "معرف سطر غير صالح.", "Invalid line item ID.", "Validation error")
	addKey(e, "vendor.purchase_request.line_offer_updated_success", "vendor", "تم تحديث عرض السطر بنجاح.", "Line item offer updated successfully.", "Success notice")

	// --- Saving Products Import ---
	addKey(e, "saving.import.session_not_found", "catalog", "جلسة الاستيراد غير موجودة أو انتهت صلاحيتها.", "Import session not found or expired.", "Session error")
	addKey(e, "saving.import.save_failed_prefix", "catalog", "فشل الحفظ: ", "Save failed: ", "Error prefix")
	addKey(e, "saving.import.cancelled_success", "catalog", "تم إلغاء جلسة الاستيراد بنجاح.", "Import session cancelled successfully.", "Success notice")

	// --- Vendor Inventory ---
	addKey(e, "vendor.inventory.warehouse_prefix", "vendor", "مخزن ", "Warehouse ", "Warehouse name prefix")

	// --- Saving AI Matching ---
	addKey(e, "saving.ai.unavailable_reason", "catalog", "خدمة الذكاء الاصطناعي غير مهيّأة على هذا الخادم؛ تعمل المطابقة الحتمية وحدها.", "AI matching service is not configured on this server; deterministic matching runs alone.", "AI unavailable reason")

	// --- Smart Order Actions & Handlers ---
	addKey(e, "smartorder.invalid_quantity", "smartorder", "أدخل كمية صحيحة.", "Enter a valid quantity.", "Validation error")
	addKey(e, "smartorder.select_supplier_from_list", "smartorder", "اختر موردًا من القائمة.", "Select a supplier from the list.", "Validation error")
	addKey(e, "smartorder.finalizer_unavailable", "smartorder", "اعتماد الطلبات غير مُفعّل حاليًا.", "Order finalization is currently disabled.", "Service error")
	addKey(e, "smartorder.notice_run_already_in_progress", "smartorder", "المطابقة قيد التشغيل بالفعل لهذا الملف.", "Matching is already in progress for this file.", "Info notice")
	addKey(e, "smartorder.notice_matching_started", "smartorder", "بدأت عملية مطابقة الذكاء الاصطناعي في الخلفية.", "Smart matching started in background.", "Success notice")
	addKey(e, "smartorder.notice_draft_cancelled", "smartorder", "تم إلغاء المسودة.", "Draft cancelled.", "Success notice")

	// --- Smart Order Status Labels ---
	addKey(e, "smartorder.status_placed", "smartorder", "مكتمل ومعتمد", "Completed & Placed", "Completed smart order status")
	addKey(e, "smartorder.status_processing", "smartorder", "جاري المعالجة", "Processing", "Processing smart order status")
	addKey(e, "smartorder.status_mapping", "smartorder", "بانتظار تعيين الأعمدة", "Mapping Columns", "Mapping columns smart order status")
	addKey(e, "smartorder.status_draft", "smartorder", "مسودة", "Draft", "Draft smart order status")
	addKey(e, "smartorder.status_stale", "smartorder", "بحاجة لإعادة تشغيل", "Needs Rerun", "Stale smart order status")
	addKey(e, "smartorder.status_failed", "smartorder", "تعذّر الاكتمال", "Failed", "Failed smart order status")

	addKey(e, "smartorder.ai_not_enabled", "smartorder", "مطابقة الذكاء الاصطناعي غير مفعّلة على هذه المنصة.", "Smart matching is not enabled on this platform.", "AI status")
	addKey(e, "smartorder.ai_gateway_down", "smartorder", "بوابة الذكاء الاصطناعي متوقفة حاليًا. ستعمل المطابقة الحتمية وحدها.", "AI Gateway is currently down. Deterministic matching runs alone.", "AI status")
	addKey(e, "smartorder.ai_org_members_only", "smartorder", "مطابقة الذكاء الاصطناعي متاحة لأعضاء المؤسسات فقط.", "Smart matching is available for organization members only.", "AI permission")
	addKey(e, "smartorder.ai_subscription_check_failed", "smartorder", "تعذّر التحقق من اشتراك مؤسستك في خدمات الذكاء الاصطناعي.", "Could not verify organization's AI subscription.", "AI error")
	addKey(e, "smartorder.ai_key_missing", "smartorder", "لم يتم تفعيل الذكاء الاصطناعي لمؤسستك بعد. تواصل مع الإدارة لتفعيله.", "AI is not enabled for your organization yet. Contact support to enable it.", "AI error")
	addKey(e, "smartorder.upload_size_limit", "smartorder", "تعذّر رفع الملف. تأكد من أن حجمه لا يتجاوز 64 ميجابايت وأن الرفع اكتمل.", "Could not upload file. Make sure size does not exceed 64 MB and upload completed.", "Upload error")
	addKey(e, "smartorder.file_required", "smartorder", "اختر ملف الأصناف المطلوبة.", "Please select required items file.", "Validation error")
	addKey(e, "smartorder.empty_file", "smartorder", "الملف فارغ أو تعذّرت قراءته.", "File is empty or unreadable.", "Validation error")
	addKey(e, "smartorder.read_file_failed_format", "smartorder", "تعذّرت قراءة الملف: %s", "Could not read file: %s", "File error")
	addKey(e, "smartorder.save_file_failed", "smartorder", "تعذّر حفظ الملف المرفوع. حاول مرة أخرى.", "Could not save uploaded file. Please try again.", "Save error")
	addKey(e, "smartorder.file_no_longer_available", "smartorder", "لم يعد الملف المرفوع متاحًا. يرجى رفعه مرة أخرى.", "Uploaded file is no longer available. Please upload again.", "File error")
	addKey(e, "smartorder.stage_lines_failed_format", "smartorder", "تعذّر تجهيز الصفوف: %s", "Could not stage rows: %s", "Staging error")
	addKey(e, "smartorder.no_items_found", "smartorder", "لم يُعثر على أي صنف في الملف بعد تطبيق التعيين.", "No items found in file after applying column mapping.", "Validation error")
	addKey(e, "smartorder.save_lines_failed", "smartorder", "تعذّر حفظ الصفوف.", "Could not save rows.", "Save error")
	addKey(e, "smartorder.err_branch_required", "smartorder", "يرجى اختيار فرع الاستلام قبل المتابعة.", "Please select delivery branch before proceeding.", "Domain error")
	addKey(e, "smartorder.err_branch_no_location", "smartorder", "لم يتم تحديد موقع فرع الاستلام على الخريطة، يرجى تحديده لتتمكن من الطلب.", "Delivery branch location is not set on map. Please set it to place orders.", "Domain error")
	addKey(e, "smartorder.err_branch_not_owned", "smartorder", "فرع الاستلام المحدد لا يتبع حسابك.", "Selected delivery branch does not belong to your account.", "Domain error")
	addKey(e, "smartorder.err_branch_no_institutional_works", "smartorder", "فرع الاستلام غير مرتبط بأي عمل مؤسسي، ولن يُقبل أي طلب منه. يرجى تفعيل عمل مؤسسي للفرع أولاً من إعدادات الفروع.", "The delivery branch has no institutional work, so no order from it can be accepted. Enable an institutional work for the branch first, from the branch settings.", "Domain error")
	addKey(e, "smartorder.blocked_institutional_hint", "smartorder", "العمل المؤسسي لفرع الاستلام غير متصل بالأعمال المؤسسية لفروع هذا المورد وفقاً لإعدادات المنصة، لذلك لا يمكن الشراء منه.", "The delivery branch's institutional work is not connected to this supplier's branch institutional works under the platform settings, so it cannot be bought from.", "Domain error")
	addKey(e, "smartorder.err_nothing_to_order", "smartorder", "لا توجد أصناف صالحة للطلب ومحددة بمورد وكمية أكبر من صفر.", "No valid items to order with selected supplier and quantity greater than zero.", "Domain error")
	addKey(e, "smartorder.err_customer_required", "smartorder", "بيانات العميل غير مكتملة.", "Customer data is incomplete.", "Domain error")
	addKey(e, "smartorder.err_empty_cart", "smartorder", "سلة الطلب فارغة.", "Order cart is empty.", "Domain error")
	addKey(e, "smartorder.err_missing_documents", "smartorder", "يرجى استكمال المستندات الإلزامية للمؤسسة قبل إتمام الطلب.", "Please complete required organization documents before placing order.", "Domain error")
	addKey(e, "smartorder.err_min_order_not_met", "smartorder", "إجمالي الطلب أقل من الحد الأدنى للطلب لدى المورد.", "Order total is below supplier's minimum order requirement.", "Domain error")
	addKey(e, "smartorder.err_line_unavailable", "smartorder", "بعض الأصناف المختارة غير متاحة للطلب حالياً أو خارج نطاق التغطية.", "Some selected items are currently unavailable or out of coverage.", "Domain error")
	addKey(e, "smartorder.err_out_of_stock", "smartorder", "بعض الأصناف نفدت من المخزون لدى المورد.", "Some items are out of stock with supplier.", "Domain error")
	addKey(e, "smartorder.err_insufficient_stock", "smartorder", "الكمية المطلوبة تتجاوز المخزون المتاح لدى المورد.", "Requested quantity exceeds available supplier stock.", "Domain error")
	addKey(e, "smartorder.err_below_minimum", "smartorder", "الكمية المطلوبة أقل من الحد الأدنى للطلب من الصنف.", "Requested quantity is below minimum order quantity for item.", "Domain error")
	addKey(e, "smartorder.err_mapping_incomplete", "smartorder", "عيّن عمودًا لاسم الصنف أو كوده أو الباركود قبل بدء المطابقة.", "Map a column for item name, SKU, or barcode before matching.", "Domain error")
	addKey(e, "smartorder.err_already_finalized", "smartorder", "تم اعتماد هذا الطلب من قبل.", "This order has already been finalized.", "Domain error")
	addKey(e, "smartorder.err_stale", "smartorder", "تغيّرت الإعدادات بعد إنشاء النتائج. أعد التشغيل قبل الاعتماد.", "Settings changed after results were generated. Re-run before finalization.", "Domain error")
	addKey(e, "smartorder.err_operation_failed_format", "smartorder", "تعذّر إكمال العملية: %s", "Could not complete operation: %s", "Domain error")
	addKey(e, "smartorder.staging_caption", "smartorder", "جارٍ تجهيز الأصناف", "Preparing items", "Progress caption")
	addKey(e, "smartorder.matching_message", "smartorder", "جارٍ مطابقة الأصناف والبحث عن الموردين…", "Matching items and searching for suppliers…", "Progress message")
	addKey(e, "smartorder.unexpected_error", "smartorder", "حدث خطأ غير متوقع أثناء المعالجة.", "An unexpected error occurred during processing.", "Error message")
	addKey(e, "smartorder.export_error", "smartorder", "تعذّر تصدير النتائج", "Could not export results", "Export error")

	// --- Courier Delivery ---
	addKey(e, "courier.shipment_not_found", "commerce", "لم يتم العثور على شحنة مطابقة لرقم التتبع المدخل. يرجى التحقق من الرقم والمحاولة مرة أخرى.", "No shipment matches the tracking number provided. Please check and try again.", "Error notice")
	addKey(e, "courier.fetch_error", "commerce", "تعذر جلب بيانات الشحنة حالياً. يرجى المحاولة لاحقاً.", "Could not retrieve shipment details at this time. Please try again later.", "Error notice")
	addKey(e, "courier.invalid_data", "commerce", "بيانات غير صالحة.", "Invalid data.", "Validation error")
	addKey(e, "courier.code_required", "commerce", "يرجى إدخال كود الاستلام المطلوب.", "Please enter the required delivery confirmation code.", "Validation error")
	addKey(e, "courier.service_unavailable", "commerce", "الخدمة غير متاحة حالياً.", "Service is currently unavailable.", "Service error")
	addKey(e, "courier.delivery_failed", "commerce", "تعذر إتمام عملية التسليم، يرجى التأكد من كود الاستلام والمحاولة مجدداً.", "Could not complete delivery. Please verify confirmation code and try again.", "Error notice")
	addKey(e, "courier.customer_notif_title", "commerce", "تم تسليم شحنتك بنجاح (%s)", "Your shipment has been delivered successfully (%s)", "Customer notification title")
	addKey(e, "courier.customer_notif_body", "commerce", "قام مندوب الشحن بتسليم طلبك #%s من المورد (%s) وتوثيق استلام الأصناف.", "The courier delivered your order #%s from supplier (%s) and confirmed receipt.", "Customer notification body")
	addKey(e, "courier.vendor_notif_title", "commerce", "تم تسليم الشحنة وتحصيل القيمة (%s)", "Shipment delivered and value collected (%s)", "Vendor notification title")
	addKey(e, "courier.vendor_notif_body", "commerce", "تم تسليم الشحنة #%s للصيدلية بنجاح وتأكيد كود الاستلام.", "Shipment #%s was successfully delivered to the pharmacy and verified with code.", "Vendor notification body")
	addKey(e, "courier.delivery_success", "commerce", "تم تأكيد الاستلام وتسليم الشحنة بنجاح وتوثيق العملية في النظام.", "Delivery confirmation verified and recorded successfully in the system.", "Success notice")
}
