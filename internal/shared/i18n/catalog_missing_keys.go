package i18n

func loadMissingCatalogKeys(e *engine) {
	// Common & General
	addKey(e, "common.egp", "common", "ج.م", "EGP", "Currency unit")
	addKey(e, "common.items", "common", "أصناف", "Items", "Count label")
	addKey(e, "common.products", "common", "المنتجات", "Products", "Count label")
	addKey(e, "common.all_types", "common", "كافة الأنواع", "All Types", "Filter option")
	addKey(e, "common.all_statuses", "common", "كافة الحالات", "All Statuses", "Filter option")
	addKey(e, "common.default", "common", "الافتراضي", "Default", "Badge or label")
	addKey(e, "common.supplier", "common", "المورد", "Supplier", "Role / entity")
	addKey(e, "common.review", "common", "مراجعة", "Review", "Action or state")
	addKey(e, "common.commercial_register", "common", "السجل التجاري", "Commercial Register", "Document label")
	addKey(e, "common.address", "common", "العنوان", "Address", "Contact field")
	addKey(e, "common.phone", "common", "الهاتف", "Phone", "Contact field")
	addKey(e, "common.saved", "common", "تم الحفظ بنجاح", "Saved successfully", "Notification")
	addKey(e, "common.deleted", "common", "تم الحذف بنجاح", "Deleted successfully", "Notification")
	addKey(e, "common.invalid_id", "common", "المعرف غير صالح", "Invalid ID", "Error message")
	addKey(e, "common.action", "common", "الإجراء", "Action", "Table header")
	addKey(e, "common.document", "common", "المستند", "Document", "Table header")
	addKey(e, "common.approval", "common", "الموافقة", "Approval", "State label")
	addKey(e, "common.accepted", "common", "مقبول", "Accepted", "Status badge")
	addKey(e, "common.cancelled", "common", "ملغي", "Cancelled", "Status badge")

	// Navigation & Breadcrumbs
	addKey(e, "nav.home", "nav", "الرئيسية", "Home", "Navigation link")
	addKey(e, "nav.smart_order", "nav", "الطلب الذكي", "Smart Order", "Navigation link")
	addKey(e, "nav.suppliers", "nav", "الموردين", "Suppliers", "Navigation link")
	addKey(e, "nav.team", "nav", "فريق العمل", "Team", "Navigation link")
	addKey(e, "nav.breadcrumb", "nav", "مسار التنقل", "Breadcrumb", "Navigation label")

	// Search & Catalog Filter
	addKey(e, "search.price_from", "search", "السعر من", "Price from", "Price filter")
	addKey(e, "search.price_to", "search", "السعر إلى", "Price to", "Price filter")
	addKey(e, "product.not_available", "catalog", "غير متوفر", "Not Available", "Availability badge")

	// Branches
	addKey(e, "branches.address", "branches", "العنوان", "Address", "Branch address")
	addKey(e, "branches.phone", "branches", "رقم الهاتف", "Phone Number", "Branch phone")

	// Statuses & Wallet
	addKey(e, "status.completed", "status", "مكتمل", "Completed", "Transaction status")
	addKey(e, "status.rejected", "status", "مرفوض", "Rejected", "Transaction status")
	addKey(e, "wallet.amount", "wallet", "المبلغ", "Amount", "Input label")
	addKey(e, "wallet.notes", "wallet", "ملاحظات", "Notes", "Input label")
	addKey(e, "billing.recharge_wallet", "billing", "شحن المحفظة", "Recharge Wallet", "Action button")

	// Orders & Validation
	addKey(e, "orders.order_not_found", "orders", "الطلب غير موجود", "Order not found", "Error message")
	addKey(e, "smartorder.form_parse_error", "smartorder", "تعذر قراءة بيانات النموذج", "Unable to parse form data", "Error message")
	addKey(e, "validation.invalid_id", "validation", "المعرف غير صالح", "Invalid ID", "Validation error")
	addKey(e, "validation.invalid_data", "validation", "بيانات غير صالحة", "Invalid data", "Validation error")
	addKey(e, "customer.saving.import.missing_name_col", "customer", "عمود اسم الصنف مفقود في الملف", "Product name column is missing in the file", "Validation error")

	// Admin
	addKey(e, "admin.users.not_found", "admin", "المستخدم غير موجود", "User not found", "Error message")
	addKey(e, "admin.warehouses.not_found", "admin", "المستودع غير موجود", "Warehouse not found", "Error message")
	addKey(e, "admin.temp_wh.invalid_data", "admin", "بيانات المستودع المؤقت غير صالحة", "Invalid temporary warehouse data", "Validation error")
	addKey(e, "admin.temp_wh.all_files_mapped_msg", "admin", "تم تعيين كافة الملفات بنجاح", "All files have been mapped successfully", "Notice")

	// Vendor Ingest & Imports
	addKey(e, "vendor.import.select_file", "vendor_ingest", "يرجى اختيار ملف للاستيراد", "Please select a file to import", "Validation error")
	addKey(e, "vendor.import.empty_file", "vendor_ingest", "الملف فارغ ولا يحتوي على بيانات", "File is empty and contains no data", "Validation error")
	addKey(e, "vendor.import.file_too_large", "vendor_ingest", "حجم الملف كبير جداً", "File size is too large", "Validation error")
	addKey(e, "vendor.import.cancelled_notice", "vendor_ingest", "تم إلغاء عملية الاستيراد", "Import operation was cancelled", "Notice")
	addKey(e, "vendor.offer.location_delete_failed", "vendor_offer", "تعذر حذف نطاق التغطية", "Failed to delete coverage location", "Error message")

	// Vendor Warehouses
	addKey(e, "vendor_warehouses.status_active", "vendor_warehouses", "نشط", "Active", "Status badge")
	addKey(e, "vendor_warehouses.status_inactive", "vendor_warehouses", "غير نشط", "Inactive", "Status badge")
	addKey(e, "vendor_warehouses.btn_save_changes", "vendor_warehouses", "حفظ التغييرات", "Save Changes", "Action button")

	// Vendor Dashboard & Orders
	addKey(e, "vendor_dashboard.modal_th_item", "vendor_dashboard", "الصنف", "Item", "Modal table header")
	addKey(e, "vendor_dashboard.modal_th_price", "vendor_dashboard", "السعر", "Price", "Modal table header")
	addKey(e, "vendor_dashboard.modal_th_qty", "vendor_dashboard", "الكمية", "Quantity", "Modal table header")
	addKey(e, "vendor_dashboard.modal_th_total", "vendor_dashboard", "الإجمالي", "Total", "Modal table header")
	addKey(e, "vendor_orders.th_public_price", "vendor_orders", "سعر الجمهور", "Public Price", "Table header")
	addKey(e, "vendor_orders.th_discount_pct", "vendor_orders", "نسبة الخصم", "Discount %", "Table header")

	// AI
	addKey(e, "ai.feat.auto_assistant", "ai", "المساعد الذكي التلقائي", "Automated Assistant", "Feature name")

	// Vendor Earnings & Financial Polishing
	addKey(e, "vendor_finance.earnings.formula_title", "vendor_finance", "معادلة الأرباح المعتمدة", "Approved Profit Formula", "Formula title")
	addKey(e, "vendor_finance.earnings.formula_simple", "vendor_finance", "صافي الربح = صافي مبيعاتك - تكلفة البضاعة المباعة (COGS)", "Net Profit = Net Sales - Cost of Goods Sold (COGS)", "Formula subtitle")
	addKey(e, "vendor_finance.earnings.formula_detail", "vendor_finance", "إجمالي التكلفة = تكلفة شراء الأصناف الفعلية (سعر التكلفة بعد خصم التكلفة)", "Total Cost = Actual Item Purchase Cost (Cost Price after Cost Discount)", "Formula detail")
	addKey(e, "vendor_finance.earnings.btn_all_time", "vendor_finance", "عرض كافة الفترات السابقة (All Time)", "View All Time Records", "Button")
	addKey(e, "vendor_finance.earnings.status_partially_paid", "vendor_finance", "سداد جزئي", "Partially Paid", "Badge")
	addKey(e, "vendor_finance.earnings.tab_orders", "vendor_finance", "أرباح الشحنات والطلبات المسلّمة", "Delivered Shipments & Orders Profit", "Tab")
	addKey(e, "vendor_finance.earnings.tab_products", "vendor_finance", "تحليل ربحية وهوامش الأصناف", "Products Profitability & Margins", "Tab")
	addKey(e, "vendor_finance.earnings.kpi_gross_sales_title", "vendor_finance", "إجمالي سعر الجمهور", "Gross Retail Value", "KPI title")
	addKey(e, "vendor_finance.earnings.kpi_gross_sales_desc", "vendor_finance", "القيمة الكلية المحتسبة بالأسعار الرسمية للجمهور قبل الخصومات", "Total value calculated at official retail prices before discounts", "KPI subtitle")
	addKey(e, "vendor_finance.earnings.kpi_net_sales_desc", "vendor_finance", "قيمة المبيعات الفعلية المحققة بعد الخصم", "Actual sales revenue realized after discounts", "KPI subtitle")
	addKey(e, "vendor_finance.earnings.kpi_cogs_desc", "vendor_finance", "تكلفة شراء البضاعة الفعلية للمورد", "Actual merchandise purchase cost for the vendor", "KPI subtitle")
	addKey(e, "vendor_finance.earnings.kpi_net_profit_desc", "vendor_finance", "صافي الأرباح بعد حسم تكاليف البضاعة والخصومات", "Net profit after deducting product costs and discounts", "KPI subtitle")
	addKey(e, "vendor_finance.earnings.kpi_profit_on_cost_desc", "vendor_finance", "العائد على التكلفة (نسبة الربح من رأس المال)", "Profit ratio relative to merchandise capital", "KPI subtitle")
	addKey(e, "vendor_finance.earnings.discounts_granted_sub", "vendor_finance", "خصومات بيع ممنوحة: %s ج.م", "Discounts granted: %s EGP", "Subtitle")
	addKey(e, "vendor_finance.earnings.products_empty_title", "vendor_finance", "لا توجد مبيعات أصناف خلال هذه الفترة المحددة", "No product sales during this period", "Empty title")
	addKey(e, "vendor_finance.earnings.products_empty_desc", "vendor_finance", "تظهر تحليلات ربحية الأصناف وهوامش الربح هنا فور تسليم الطلبيات.", "Product profitability and margin analytics will appear here once orders are delivered.", "Empty desc")

	// Smart Order Default Quantity
	addKey(e, "smart_order.default_qty_title", "smart_order", "الكمية الافتراضية للبنود", "Default Item Quantity", "Feature title")
	addKey(e, "smart_order.default_qty_hint", "smart_order", "تطبيق كمية محددة دفعة واحدة على بنود الطلبية لتسهيل وسرعة المراجعة", "Apply a default quantity across items in bulk for fast review", "Feature subtitle")
	addKey(e, "smart_order.default_qty_label", "smart_order", "الكمية", "Quantity", "Input label")
	addKey(e, "smart_order.apply_default_qty_all", "smart_order", "تطبيق على كافة الأصناف", "Apply to All Items", "Button")
	addKey(e, "smart_order.apply_default_qty_selected", "smart_order", "تطبيق على المحدد", "Apply to Selected", "Button")

	// Vendor Stock Alerts
	addKey(e, "vendor_stock_alerts.page_title", "vendor_stock_alerts", "تنبيهات انخفاض المخزون", "Stock Alerts", "Page Title")
	addKey(e, "vendor_stock_alerts.title", "vendor_stock_alerts", "تنبيهات انخفاض ونفاد المخزون", "Low & Out of Stock Alerts", "Page Header")
	addKey(e, "vendor_stock_alerts.subtitle", "vendor_stock_alerts", "قائمة تفصيلية بجميع الأصناف التي بلغت أو انخفضت عن حد إعادة الطلب بالمخازن والفروع", "Detailed list of items at or below reorder threshold across warehouses and branches", "Page Subtitle")
	addKey(e, "vendor_stock_alerts.btn_all_inventory", "vendor_stock_alerts", "إدارة المخزون العام", "All Inventory", "Button")
	addKey(e, "vendor_stock_alerts.btn_warehouses", "vendor_stock_alerts", "إدارة المخازن", "Warehouses", "Button")
	addKey(e, "vendor_stock_alerts.stat_out_of_stock", "vendor_stock_alerts", "أصناف نفدت بالكامل", "Out of Stock", "Stat Card")
	addKey(e, "vendor_stock_alerts.stat_out_of_stock_desc", "vendor_stock_alerts", "الرصيد المتاح 0 عبوة", "Available stock is 0 packs", "Stat Card Subtitle")
	addKey(e, "vendor_stock_alerts.stat_approaching_empty", "vendor_stock_alerts", "أصناف قاربت على النفاد", "Approaching Empty", "Stat Card")
	addKey(e, "vendor_stock_alerts.stat_approaching_empty_desc", "vendor_stock_alerts", "الرصيد أقل من أو يساوي حد الطلب", "Stock <= reorder threshold", "Stat Card Subtitle")
	addKey(e, "vendor_stock_alerts.stat_total_alerts", "vendor_stock_alerts", "إجمالي الأصناف الحرجة", "Total Critical Items", "Stat Card")
	addKey(e, "vendor_stock_alerts.stat_total_alerts_desc", "vendor_stock_alerts", "تحتاج لإعادة تعبئة أو توريد", "Needs replenishment or restock", "Stat Card Subtitle")
	addKey(e, "vendor_stock_alerts.stat_affected_warehouses", "vendor_stock_alerts", "المخازن المتأثرة", "Affected Warehouses", "Stat Card")
	addKey(e, "vendor_stock_alerts.stat_affected_warehouses_desc", "vendor_stock_alerts", "مواقع بها نقص في المخزون", "Locations with stock deficit", "Stat Card Subtitle")
	addKey(e, "vendor_stock_alerts.search_placeholder", "vendor_stock_alerts", "ابحث باسم الدواء، الكود، SKU، أو رقم التشغيلة...", "Search by medicine name, code, SKU, or batch...", "Search Placeholder")
	addKey(e, "vendor_stock_alerts.filter_all_statuses", "vendor_stock_alerts", "كافة التنبيهات (الكل)", "All Alerts", "Dropdown Option")
	addKey(e, "vendor_stock_alerts.filter_out_of_stock", "vendor_stock_alerts", "أصناف نفدت فقط (0)", "Out of Stock Only", "Dropdown Option")
	addKey(e, "vendor_stock_alerts.filter_low_stock", "vendor_stock_alerts", "أصناف منخفضة فقط (>0)", "Low Stock Only", "Dropdown Option")
	addKey(e, "vendor_stock_alerts.empty_filter_title", "vendor_stock_alerts", "لا توجد نتائج مطابقة لخيارات البحث", "No matching stock alerts", "Empty Title")
	addKey(e, "vendor_stock_alerts.empty_filter_desc", "vendor_stock_alerts", "جرب تعديل كلمات البحث أو اختيار مخزن آخر.", "Try adjusting search terms or warehouse filter.", "Empty Subtitle")
	addKey(e, "vendor_stock_alerts.empty_healthy_title", "vendor_stock_alerts", "المخزون سليم وآمن تماماً!", "Inventory is completely healthy!", "Empty Title")
	addKey(e, "vendor_stock_alerts.empty_healthy_desc", "vendor_stock_alerts", "رائع! لا توجد حالياً أي أصناف تحت حد إعادة الطلب في كافة المخازن والفروع.", "Great! No items are currently below reorder thresholds across any warehouses.", "Empty Subtitle")
	addKey(e, "vendor_stock_alerts.th_product_variant", "vendor_stock_alerts", "المنتج / الصنف", "Product & Variant", "Table Header")
	addKey(e, "vendor_stock_alerts.th_branch", "vendor_stock_alerts", "الفرع التابع", "Branch", "Table Header")
	addKey(e, "vendor_stock_alerts.th_warehouse", "vendor_stock_alerts", "المخزن", "Warehouse", "Table Header")
	addKey(e, "vendor_stock_alerts.th_available_qty", "vendor_stock_alerts", "الرصيد المتاح", "Available Qty", "Table Header")
	addKey(e, "vendor_stock_alerts.th_reorder_level", "vendor_stock_alerts", "حد الطلب", "Reorder Level", "Table Header")
	addKey(e, "vendor_stock_alerts.th_deficit", "vendor_stock_alerts", "مقدار العجز", "Deficit", "Table Header")
	addKey(e, "vendor_stock_alerts.th_status", "vendor_stock_alerts", "الحالة", "Status", "Table Header")
	addKey(e, "vendor_stock_alerts.th_actions", "vendor_stock_alerts", "الإجراءات", "Actions", "Table Header")
	addKey(e, "vendor_stock_alerts.btn_restock", "vendor_stock_alerts", "تعديل / تعبئة", "Restock / Adjust", "Button")
	addKey(e, "vendor_stock_alerts.btn_adjust_title", "vendor_stock_alerts", "تعديل رصيد الصنف في هذا المخزن", "Adjust stock in warehouse", "Tooltip")
	addKey(e, "vendor_stock_alerts.btn_view_wh", "vendor_stock_alerts", "عرض تفاصيل المخزن", "View warehouse details", "Tooltip")
	addKey(e, "vendor_stock_alerts.main_branch", "vendor_stock_alerts", "الفرع الرئيسي", "Main Branch", "Label")
}

