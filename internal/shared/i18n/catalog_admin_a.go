package i18n

func loadAdminCatalogA(e *engine) {
	addKey(e, "admin.dashboard_title", "admin", "لوحة المعلومات والإحصائيات الرئيسية", "Platform Overview & Realtime Analytics", "Admin dashboard title")
	addKey(e, "admin.total_users", "admin", "إجمالي المستخدمين", "Total Users", "Dashboard metric users")
	addKey(e, "admin.total_organizations", "admin", "إجمالي المنشآت", "Total Organizations", "Dashboard metric orgs")
	addKey(e, "admin.total_orders", "admin", "إجمالي الطلبات", "Total Orders", "Dashboard metric orders")
	addKey(e, "admin.total_revenue", "admin", "إجمالي حجم التداول", "Total Trade Volume", "Dashboard metric revenue")
	addKey(e, "admin.jobs_unknown_company", "admin", "منشأة معتمدة", "Verified organization", "Fallback company name on the admin jobs list")

	// Translations Manager
	addKey(e, "admin.translations_title", "admin", "اللغات وإدارة الترجمة والتدويل (i18n)", "Localization & Translations Manager", "Translations manager page title")
	addKey(e, "admin.translations_sub", "admin", "إدارة كافة نصوص ومصطلحات المنصة باللغتين العربية والإنجليزية، وتعديل الترجمات ديناميكياً مع التطبيق الفوري.", "Manage all platform text and terms in Arabic & English with real-time runtime override support.", "Translations page subtitle")
	addKey(e, "admin.translations_sync", "admin", "مزامنة المفاتيح الافتراضية", "Sync Default Keys", "Sync default keys button")
	addKey(e, "admin.translations_total_keys", "admin", "إجمالي المفاتيح", "Total Keys", "Stats total keys")
	addKey(e, "admin.translations_custom_overrides", "admin", "الترجمات المخصصة", "Custom Overrides", "Stats custom overrides")
	addKey(e, "admin.translations_namespaces", "admin", "الأقسام والتصنيفات", "Namespaces", "Stats namespaces")
	addKey(e, "admin.translations_all_namespaces", "admin", "كل التصنيفات (All Namespaces)", "All Namespaces", "Namespace filter all")
	addKey(e, "admin.translations_key_col", "admin", "مفتاح الترجمة (Key)", "Translation Key", "Table key header")
	addKey(e, "admin.translations_ns_col", "admin", "القسم (Namespace)", "Namespace", "Table namespace header")
	addKey(e, "admin.translations_ar_col", "admin", "النص العربي (Arabic)", "Arabic Text", "Table AR header")
	addKey(e, "admin.translations_en_col", "admin", "النص الإنجليزي (English)", "English Text", "Table EN header")
	addKey(e, "admin.translations_type_col", "admin", "النوع", "Type", "Table type header")
	addKey(e, "admin.translations_default_badge", "admin", "افتراضي", "Default", "Default badge")
	addKey(e, "admin.translations_custom_badge", "admin", "مخصص", "Custom", "Custom badge")
	addKey(e, "admin.translations_edit_modal", "admin", "تعديل نص الترجمة", "Edit Translation", "Edit modal title")
	addKey(e, "admin.translations_reset_btn", "admin", "استعادة الافتراضي", "Reset to Default", "Reset button")
	addKey(e, "admin.translations_reset_confirm", "admin", "هل أنت متأكد من استعادة الترجمة الافتراضية لهذا المفتاح؟", "Are you sure you want to revert this key to its code default?", "Reset confirmation")
	addKey(e, "admin.translations_updated_success", "admin", "تم تحديث نص الترجمة وتطبيقه في النظام فوراً.", "Translation updated and applied immediately.", "Update success notice")
	addKey(e, "admin.translations_reset_success", "admin", "تمت استعادة الترجمة الافتراضية بنجاح.", "Reverted to default translation successfully.", "Reset success notice")
	addKey(e, "admin.translations_sync_success", "admin", "تمت مزامنة كافة مفاتيح النظام مع قاعدة البيانات بنجاح.", "Synced all system keys to database successfully.", "Sync success notice")

	// Chat History Audit
	addKey(e, "admin.chat_history_title", "admin", "سجلات محادثات المساعد الذكي دكتور كبسولة AI", "AI Dr. Capsule Assistant Audit & Chat Logs", "Admin chat history page title")
	addKey(e, "admin.chat_history_sub", "admin", "مراقبة وتدقيق كافة جلسات ومحادثات الذكاء الاصطناعي مع المستخدمين (صيدليات وموردين) واستهلاك الرموز وتفاصيل الحوارات.", "Audit all user AI conversations across pharmacies and vendors with token usage and full dialog inspection.", "Admin chat history subtitle")

	// Decision Memory
	addKey(e, "admin.decision_memory.toggle_error", "admin", "حدث خطأ أثناء تحديث حالة ذاكرة القرارات.", "Error updating decision memory state.", "Toggle error")
	addKey(e, "admin.decision_memory.state_enabled", "admin", "تفعيل", "Enable", "Enabled state word")
	addKey(e, "admin.decision_memory.state_disabled", "admin", "إيقاف", "Disable", "Disabled state word")
	addKey(e, "admin.decision_memory.toggle_success", "admin", "تم %s نظام ذاكرة القرارات بنجاح على مستوى المنصة بالكامل.", "Decision memory system was %sd successfully across the platform.", "Toggle success")
	addKey(e, "admin.decision_memory.invalid_id", "admin", "معرف القرار غير صالح.", "Invalid decision ID.", "Validation error")
	addKey(e, "admin.decision_memory.delete_error", "admin", "حدث خطأ أثناء حذف القرار.", "Error deleting decision.", "Delete error")
	addKey(e, "admin.decision_memory.delete_success", "admin", "تم حذف القرار من ذاكرة المطابقة بنجاح.", "Decision deleted from match memory successfully.", "Delete success")
	addKey(e, "admin.decision_memory.clear_error", "admin", "حدث خطأ أثناء مسح ذاكرة القرارات.", "Error clearing decision memory.", "Clear error")
	addKey(e, "admin.decision_memory.clear_success", "admin", "تم مسح كافة قرارات الذاكرة بنجاح.", "All memory decisions cleared successfully.", "Clear success")
	addKey(e, "decision_memory.item_and_drug_required", "catalog", "يرجى إدخال اسم الصنف واختيار الدواء المطابق من الكتالوج.", "Please enter item name and select matched medicine from catalog.", "Validation error")
	addKey(e, "decision_memory.customer_manual_reason", "catalog", "قرار يدوي مضاف من الصيدلية", "Manual decision added by pharmacy", "Default reason")
	addKey(e, "decision_memory.vendor_manual_reason", "catalog", "قرار يدوي مضاف من المورد", "Manual decision added by vendor", "Default reason")
	addKey(e, "decision_memory.save_error", "catalog", "حدث خطأ أثناء حفظ القرار في الذاكرة.", "Error saving decision to memory.", "Save error")
	addKey(e, "decision_memory.customer_saved_success", "catalog", "تم حفظ قرار المطابقة بنجاح في ذاكرة الصيدلية.", "Match decision saved successfully in pharmacy memory.", "Save success")
	addKey(e, "decision_memory.vendor_saved_success", "catalog", "تم حفظ قرار المطابقة بنجاح في ذاكرة المورد.", "Match decision saved successfully in vendor memory.", "Save success")
	addKey(e, "decision_memory.org_cleared_success", "catalog", "تم مسح ذاكرة قرارات المطابقة بنجاح.", "Match decision memory cleared successfully.", "Clear success")

	// --- Admin CMS Content ---
	addKey(e, "admin.content.service_unavailable", "admin", "خدمة المحتوى غير متاحة حالياً.", "Content service is currently unavailable.", "Service error")
	addKey(e, "admin.content.key_required", "admin", "يرجى تحديد المفتاح التعريفي للكتلة.", "Please specify the content block key.", "Validation error")
	addKey(e, "admin.content.saved_success", "admin", "تم حفظ كتلة المحتوى بنجاح وتحديثها في المنصة.", "Content block saved and updated successfully on the platform.", "Success notice")
	addKey(e, "admin.content.invalid_id", "admin", "معرف الكتلة غير صالح.", "Invalid content block ID.", "Validation error")
	addKey(e, "admin.content.status_updated_success", "admin", "تم تحديث حالة تفعيل الكتلة بنجاح.", "Content block status updated successfully.", "Success notice")
	addKey(e, "admin.content.deleted_success", "admin", "تم حذف كتلة المحتوى بنجاح.", "Content block deleted successfully.", "Success notice")

	// --- Platform Audit Trail ---
	addKey(e, "admin.audit.severity_info", "admin", "عادي (Info)", "Info", "Audit severity info")
	addKey(e, "admin.audit.severity_warning", "admin", "متوسط (Warning)", "Warning", "Audit severity warning")
	addKey(e, "admin.audit.severity_critical", "admin", "حرج (Critical)", "Critical", "Audit severity critical")

	addKey(e, "admin.audit.mod_orgs", "admin", "المنشآت", "Organizations", "Audit module organizations")
	addKey(e, "admin.audit.mod_users", "admin", "المستخدمين", "Users", "Audit module users")
	addKey(e, "admin.audit.mod_security", "admin", "الأمان والصلاحيات", "Security & Permissions", "Audit module security")
	addKey(e, "admin.audit.mod_catalog", "admin", "الكتالوج", "Catalogue", "Audit module catalog")
	addKey(e, "admin.audit.mod_offers", "admin", "عروض الموردين", "Supplier Offers", "Audit module offers")
	addKey(e, "admin.audit.mod_orders", "admin", "أوامر التوريد", "Supply Orders", "Audit module orders")
	addKey(e, "admin.audit.mod_institutional", "admin", "الهيكل المؤسسي", "Institutional Structure", "Audit module institutional")
	addKey(e, "admin.audit.mod_system", "admin", "النظام", "System", "Audit module system")

	addKey(e, "admin.audit.action_org_registered", "admin", "تسجيل منشأة جديدة", "Register New Organization", "Audit action")
	addKey(e, "admin.audit.title_org_registered", "admin", "طلب تسجيل منشأة جديدة", "New Organization Registration Request", "Audit title")
	addKey(e, "admin.audit.desc_org_registered", "admin", "تم تقديم ملف ترخيص وسجل تجاري لمنشأة دوائية جديدة", "A license file and commercial register were submitted for a new pharmaceutical organization", "Audit desc")

	addKey(e, "admin.audit.action_org_status", "admin", "تحديث حالة اعتماد المنشأة", "Update Organization Approval Status", "Audit action")
	addKey(e, "admin.audit.title_org_approved", "admin", "اعتماد أو ترخيص منشأة", "Approve or License Organization", "Audit title")
	addKey(e, "admin.audit.desc_org_approved", "admin", "تم التحقق من الوثائق والموافقة على حساب المنشأة", "Documents verified and organization account approved", "Audit desc")

	addKey(e, "admin.audit.action_org_rejected", "admin", "رفض اعتماد المنشأة", "Reject Organization Approval", "Audit action")
	addKey(e, "admin.audit.title_org_rejected", "admin", "رفض اعتماد منشأة", "Reject Organization Approval", "Audit title")
	addKey(e, "admin.audit.desc_org_rejected", "admin", "تم رفض ملف المنشأة بسبب عدم استيفاء التراخيص", "Organization application rejected due to incomplete licensing", "Audit desc")

	addKey(e, "admin.audit.action_org_suspended", "admin", "إيقاف المنشأة مؤقتاً", "Suspend Organization Temporarily", "Audit action")
	addKey(e, "admin.audit.title_org_suspended", "admin", "إيقاف حساب منشأة", "Suspend Organization Account", "Audit title")
	addKey(e, "admin.audit.desc_org_suspended", "admin", "تم تعليق حساب المنشأة مؤقتاً لمخالفة اللوائح", "Organization account temporarily suspended for regulatory violation", "Audit desc")

	addKey(e, "admin.audit.action_user_registered", "admin", "تسجيل حساب مستخدم جديد", "Register New User Account", "Audit action")
	addKey(e, "admin.audit.title_user_registered", "admin", "إنشاء حساب مستخدم", "Create User Account", "Audit title")
	addKey(e, "admin.audit.desc_user_registered", "admin", "تم تسجيل عضو أو صيدلي جديد في النظام", "A new member or pharmacist was registered in the system", "Audit desc")

	addKey(e, "admin.audit.action_user_status", "admin", "تغيير حالة حساب المستخدم", "Change User Account Status", "Audit action")
	addKey(e, "admin.audit.title_user_status", "admin", "تعديل حالة الحساب", "Modify Account Status", "Audit title")
	addKey(e, "admin.audit.desc_user_status", "admin", "تحديث حالة التفعيل أو الإيقاف لحساب المستخدم", "User account activation or suspension status updated", "Audit desc")

	addKey(e, "admin.audit.action_user_role", "admin", "تعيين دور وصلاحية للمستخدم", "Assign Role and Permission to User", "Audit action")
	addKey(e, "admin.audit.title_user_role", "admin", "إسناد صلاحية أمنية", "Assign Security Permission", "Audit title")
	addKey(e, "admin.audit.desc_user_role", "admin", "تعديل رتبة وصلاحيات المستخدم داخل المنصة", "User platform rank and permissions modified", "Audit desc")

	addKey(e, "admin.audit.action_user_mfa_reset", "admin", "إعادة ضبط التحقق الثنائي (MFA)", "Reset Two-Factor Authentication (MFA)", "Audit action")
	addKey(e, "admin.audit.title_user_mfa_reset", "admin", "إعادة ضبط أمني (MFA)", "Security Reset (MFA)", "Audit title")
	addKey(e, "admin.audit.desc_user_mfa_reset", "admin", "إعادة ضبط مفاتيح المصادقة الثنائية لحساب المستخدم", "Two-factor authentication keys reset for user account", "Audit desc")

	addKey(e, "admin.audit.action_product_created", "admin", "إضافة صنف دوائي جديد", "Add New Medicine", "Audit action")
	addKey(e, "admin.audit.title_product_created", "admin", "إضافة دواء للكتالوج", "Add Product to Catalogue", "Audit title")
	addKey(e, "admin.audit.desc_product_created", "admin", "إدراج صنف دوائي ومستحضر معتمد في الكتالوج الموحد", "Certified pharmaceutical item added to the unified catalogue", "Audit desc")

	addKey(e, "admin.audit.action_product_updated", "admin", "تعديل بيانات الصنف الدوائي", "Edit Product Details", "Audit action")
	addKey(e, "admin.audit.title_product_updated", "admin", "تحديث بيانات دواء", "Update Medicine Details", "Audit title")
	addKey(e, "admin.audit.desc_product_updated", "admin", "تعديل الأسعار أو المادة الفعالة أو بيانات الصنف", "Prices, active ingredients, or product details updated", "Audit desc")

	addKey(e, "admin.audit.action_product_deleted", "admin", "حذف صنف من الكتالوج", "Delete Product from Catalogue", "Audit action")
	addKey(e, "admin.audit.title_product_deleted", "admin", "حذف صنف دوائي", "Delete Medicine", "Audit title")
	addKey(e, "admin.audit.desc_product_deleted", "admin", "إلغاء أو حذف صنف دوائي من الكتالوج المعتمد", "Pharmaceutical item removed from certified catalogue", "Audit desc")

	addKey(e, "admin.audit.action_variant_created", "admin", "إضافة عرض توريد جديد", "Add New Supplier Offer", "Audit action")
	addKey(e, "admin.audit.title_variant_created", "admin", "إضافة عرض سعر دوائي", "Add Medicine Price Offer", "Audit title")
	addKey(e, "admin.audit.desc_variant_created", "admin", "طرح عرض أسعار وتوريد جديد لصنف معتمد", "New supply and pricing offer listed for approved product", "Audit desc")

	addKey(e, "admin.audit.action_order_created", "admin", "إنشاء طلب توريد جديد", "Create New Supply Order", "Audit action")
	addKey(e, "admin.audit.title_order_created", "admin", "إنشاء أمر توريد", "Create Supply Order", "Audit title")
	addKey(e, "admin.audit.desc_order_created", "admin", "تم تقديم أمر توريد دوائي جديد من صيدلية", "A new pharmaceutical supply order was submitted by a pharmacy", "Audit desc")

	addKey(e, "admin.audit.action_order_status", "admin", "تحديث حالة أمر التوريد", "Update Supply Order Status", "Audit action")
	addKey(e, "admin.audit.title_order_status", "admin", "تحديث حالة الشحن/التوريد", "Update Shipping/Supply Status", "Audit title")
	addKey(e, "admin.audit.desc_order_status", "admin", "تغيير حالة الطلب الدوائي بين التجهيز والتوصيل والاستلام", "Order status changed between processing, shipping, and delivery", "Audit desc")

	addKey(e, "admin.audit.action_inst_created", "admin", "إضافة تصنيف هيكل مؤسسي", "Add Institutional Structure", "Audit action")
	addKey(e, "admin.audit.title_inst_created", "admin", "إضافة هيكل مؤسسي جديد", "Add New Institutional Structure", "Audit title")
	addKey(e, "admin.audit.desc_inst_created", "admin", "إنشاء تصنيف هيكلي جديد للمنشآت والمستودعات", "New institutional structure category created for organizations and warehouses", "Audit desc")

	addKey(e, "admin.audit.action_inst_updated", "admin", "تعديل تصنيف هيكل مؤسسي", "Edit Institutional Structure", "Audit action")
	addKey(e, "admin.audit.title_inst_updated", "admin", "تعديل هيكل مؤسسي", "Edit Institutional Structure", "Audit title")
	addKey(e, "admin.audit.desc_inst_updated", "admin", "تحديث بيانات تصنيف هيكلي أو باقة التسعير", "Institutional structure details or pricing tier updated", "Audit desc")

	addKey(e, "admin.audit.desc_default", "admin", "عملية إدارية مسجلة بالنظام", "Administrative action logged in system", "Default audit desc")

	addKey(e, "admin.audit.entity_org", "admin", "منشأة / شركة", "Organization / Company", "Audit entity organization")
	addKey(e, "admin.audit.entity_user", "admin", "مستخدم", "User", "Audit entity user")
	addKey(e, "admin.audit.entity_product", "admin", "صنف دوائي", "Medicine Product", "Audit entity product")
	addKey(e, "admin.audit.entity_variant", "admin", "عرض توريد", "Supply Offer", "Audit entity variant")
	addKey(e, "admin.audit.entity_order", "admin", "أمر توريد", "Supply Order", "Audit entity order")
	addKey(e, "admin.audit.entity_branch", "admin", "فرع مستودع / صيدلية", "Warehouse / Pharmacy Branch", "Audit entity branch")
	addKey(e, "admin.audit.entity_inst", "admin", "هيكل مؤسسي", "Institutional Structure", "Audit entity institutional")

	addKey(e, "admin.audit.default_actor", "admin", "النظام / System", "System", "Default audit actor")
	addKey(e, "admin.audit.default_org", "admin", "المنصة الرئيسية", "Main Platform", "Default audit organization")

	// --- Temporary Warehouses & Price Lists ---
	addKey(e, "admin.temp_wh.invalid_id", "admin", "معرف المستودع غير صحيح.", "Invalid temporary warehouse ID.", "Validation error")
	addKey(e, "admin.temp_wh.not_found", "admin", "المستودع غير موجود.", "Temporary warehouse not found.", "Not found error")
	addKey(e, "admin.temp_wh.mapping_updated_msg", "admin", "تم تحديث أعمدة المستودع [%s] بنجاح (إجمالي %d صنف).", "Warehouse columns [%s] updated successfully (total %d items).", "Success message")
	addKey(e, "admin.temp_wh.mapping_applied_msg", "admin", "تم تحديث وتأكيد أعمدة المستودع [%s] بنجاح.", "Warehouse columns [%s] updated and confirmed successfully.", "Success notice")
	addKey(e, "admin.temp_wh.deleted_success", "admin", "تم حذف المستودع وكافة أصنافه بنجاح.", "Temporary warehouse and all items deleted successfully.", "Success notice")
	addKey(e, "admin.temp_wh.unarchived_success", "admin", "تم تفعيل المستودع وإتاحته بالخصومات بنجاح.", "Warehouse unarchived and enabled for discounts.", "Success notice")
	addKey(e, "admin.temp_wh.manual_archive_reason", "admin", "أرشفة يدوية من لوحة المشرف", "Manual archive from admin portal", "Archive reason")
	addKey(e, "admin.temp_wh.archived_success", "admin", "تم أرشفة المستودع بنجاح.", "Warehouse archived successfully.", "Success notice")
	addKey(e, "admin.temp_wh.not_owner", "admin", "لا تملك صلاحية إدارة هذا المستودع لأنه ليس من رفعك.", "You cannot manage this warehouse because you did not upload it.", "Forbidden error")

	// --- Categories & Reference ---
	addKey(e, "admin.categories.service_unavailable", "catalog", "خدمة الكتالوج غير متاحة.", "Catalogue service is currently unavailable.", "Service error")
	addKey(e, "admin.categories.name_required", "catalog", "يرجى كتابة اسم فئة المنتج بالعربية أو الإنجليزية.", "Please enter category name in Arabic or English.", "Validation error")
	addKey(e, "admin.categories.created_success", "catalog", "تم إنشاء فئة المنتجات بنجاح.", "Product category created successfully.", "Success notice")
	addKey(e, "admin.categories.invalid_id", "catalog", "معرف الفئة غير صالح.", "Invalid category ID.", "Validation error")
	addKey(e, "admin.categories.not_found", "catalog", "فئة المنتجات غير موجودة.", "Product category not found.", "Not found error")
	addKey(e, "admin.categories.updated_success", "catalog", "تم تحديث بيانات فئة المنتجات بنجاح.", "Product category updated successfully.", "Success notice")
	addKey(e, "admin.categories.status_updated_success", "catalog", "تم تحديث حالة الفئة بنجاح.", "Category status updated successfully.", "Success notice")
	addKey(e, "admin.categories.cannot_delete_has_products_format", "catalog", "لا يمكن حذف هذه الفئة لوجود %d صنف معتمد مرتبط بها. يرجى نقل الأصناف لفئة أخرى أولاً.", "Cannot delete this category because %d certified products are linked to it. Please move items to another category first.", "Delete restriction error")
	addKey(e, "admin.categories.deleted_success", "catalog", "تم حذف فئة المنتجات بنجاح.", "Product category deleted successfully.", "Success notice")

	addKey(e, "admin.reference.social_media_title", "settings", "قنوات التواصل الاجتماعي للمنصة", "Platform Social Media Channels", "Page title")
	addKey(e, "admin.reference.social_channel", "settings", "قناة تواصل", "Social Channel", "Entity singular")
	addKey(e, "admin.reference.api_gateway_title", "developers", "بوابة الواجهات البرمجية (API Gateway)", "API Gateway", "Gateway title")
	addKey(e, "admin.reference.api_gateway_desc_format", "developers", "البيئة: %s | الرابط: %s", "Environment: %s | URL: %s", "Gateway desc")
	addKey(e, "admin.reference.api_gateway_timeout_format", "developers", "المهلة: %d ثوانٍ", "Timeout: %d seconds", "Gateway timeout")
	addKey(e, "admin.reference.ai_gateway_title", "developers", "بوابة الذكاء الاصطناعي (AI Gateway)", "AI Gateway", "Gateway title")
	addKey(e, "admin.reference.ai_gateway_desc_format", "developers", "الرابط: %s", "URL: %s", "Gateway desc")
	addKey(e, "admin.reference.ai_gateway_max_tokens_format", "developers", "الرموز القصوى: %d", "Max tokens: %d", "Gateway tokens")
	addKey(e, "admin.reference.api_integrations_title", "developers", "بوابات الربط والواجهات البرمجية (APIs)", "API Integrations & Gateways", "Page title")
	addKey(e, "admin.reference.api_integration", "developers", "واجهة ربط", "API Integration", "Entity singular")

	// --- Platform Roles & Permissions ---
	addKey(e, "admin.roles.title", "admin", "الأدوار والصلاحيات", "Roles & Permissions", "Page title")
	addKey(e, "admin.roles.subtitle", "admin", "أدوار مشرفي المنصة: ما الذي يراه كل مشرف وما الذي يستطيع تغييره.", "Platform moderator roles: what each staff member sees and can modify.", "Page subtitle")
	addKey(e, "admin.roles.badge_full_access", "admin", "كامل الصلاحيات", "Full Access", "Badge label")
	addKey(e, "admin.roles.badge_staff", "admin", "لوحة الإدارة", "Admin Portal", "Badge label")
	addKey(e, "admin.roles.badge_regular", "admin", "حساب عادي", "Standard Account", "Badge label")
	addKey(e, "admin.roles.id_service_unavailable", "identity", "خدمة الهوية غير متوفرة.", "Identity service is currently unavailable.", "Service error")
	addKey(e, "admin.roles.not_found", "identity", "الدور غير موجود.", "Role not found.", "Not found error")
	addKey(e, "admin.roles.created_success", "identity", "تم إنشاء الدور. حدّد صلاحياته الآن.", "Role created. Configure its permissions now.", "Success notice")
	addKey(e, "admin.roles.permissions_saved_success", "identity", "تم حفظ صلاحيات الدور.", "Role permissions saved successfully.", "Success notice")
	addKey(e, "admin.roles.deleted_success", "identity", "تم حذف الدور.", "Role deleted successfully.", "Success notice")
	addKey(e, "admin.roles.invalid_request", "identity", "طلب غير صالح.", "Invalid request.", "Validation error")
	addKey(e, "admin.roles.cannot_change_own_role", "identity", "لا يمكنك تغيير دور حسابك بنفسك.", "You cannot change your own account role.", "Permission error")
	addKey(e, "admin.roles.user_role_updated_success", "identity", "تم تحديث دور المستخدم وإنهاء جلساته.", "User role updated and active sessions terminated.", "Success notice")
	addKey(e, "admin.roles.invalid_form", "identity", "بيانات النموذج غير صالحة.", "Invalid form data.", "Validation error")
	addKey(e, "admin.roles.staff_created_success", "identity", "تم إنشاء حساب المشرف وإسناد دوره.", "Staff account created and role assigned successfully.", "Success notice")

	// --- Product Images Import ---
	addKey(e, "admin.image_import.file_too_large", "catalog", "حجم الملف كبير جداً أو تعذر قراءته.", "File size is too large or could not be read.", "Upload error")
	addKey(e, "admin.image_import.select_valid_file", "catalog", "يرجى اختيار ملف Excel أو CSV صالح.", "Please select a valid Excel or CSV file.", "Validation error")
	addKey(e, "admin.image_import.unsupported_format", "catalog", "صيغة الملف غير مدعومة. يرجى رفع ملف Excel (.xlsx أو .xls) أو ملف نصي (.csv).", "Unsupported file format. Please upload an Excel (.xlsx or .xls) or CSV (.csv) file.", "Format error")
	addKey(e, "admin.image_import.file_empty", "catalog", "الملف المرفوع فارغ أو تالف.", "The uploaded file is empty or corrupted.", "Validation error")
	addKey(e, "admin.image_import.header_only", "catalog", "الملف يحتوي على صف العناوين فقط ولا يوجد به أي بيانات.", "File contains headers only and has no data rows.", "Validation error")
	addKey(e, "admin.image_import.session_not_found", "catalog", "جلسة المعالجة منتهية الصلاحية أو غير موجودة.", "Processing session expired or not found.", "Not found error")
	addKey(e, "admin.image_import.catalog_service_unavailable", "catalog", "خدمة الكتالوج معطلة.", "Catalogue service is disabled.", "Service error")
	addKey(e, "admin.image_import.select_columns_required", "catalog", "يرجى تحديد عمود كود الصنف (SKU) وعمود رابط الصورة (Image URL) للبدء بربط الصور.", "Please select the SKU column and the Image URL column to begin linking images.", "Validation error")
	addKey(e, "admin.image_import.session_deleted_success", "catalog", "تم حذف جلسة استيراد الصور بنجاح.", "Image import session deleted successfully.", "Success notice")

	// --- Geography & Locations ---
	addKey(e, "admin.geo.city_name_ar_required", "admin", "اسم المدينة بالعربية مطلوب.", "City Arabic name is required.", "Validation error")
	addKey(e, "admin.geo.city_created_success", "admin", "تم حفظ وإضافة المدينة الفرعية بنجاح في قاعدة البيانات.", "City/district added and saved successfully.", "Success notice")
	addKey(e, "admin.geo.city_invalid_id", "admin", "معرف المدينة غير صالح.", "Invalid city ID.", "Validation error")
	addKey(e, "admin.geo.city_status_updated_success", "admin", "تم تحديث حالة تفعيل المدينة بنجاح.", "City status updated successfully.", "Success notice")
	addKey(e, "admin.geo.city_updated_success", "admin", "تم تحديث وحفظ بيانات المدينة بنجاح.", "City data updated and saved successfully.", "Success notice")
	addKey(e, "admin.geo.gov_name_ar_required", "admin", "اسم المحافظة بالعربية مطلوب.", "Governorate Arabic name is required.", "Validation error")
	addKey(e, "admin.geo.gov_created_success", "admin", "تم حفظ وإضافة المحافظة الرئيسية بنجاح.", "Main governorate added and saved successfully.", "Success notice")
	addKey(e, "admin.geo.gov_invalid_id", "admin", "معرف المحافظة غير صالح.", "Invalid governorate ID.", "Validation error")
	addKey(e, "admin.geo.gov_updated_success", "admin", "تم تحديث بيانات المحافظة بنجاح.", "Governorate data updated successfully.", "Success notice")
	addKey(e, "admin.geo.gov_status_updated_success", "admin", "تم تحديث حالة تفعيل المحافظة بنجاح.", "Governorate status updated successfully.", "Success notice")

	// --- Platform Settings ---
}
