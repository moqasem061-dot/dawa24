package datasets

import (
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// The buying side, shared by pharmacies and by suppliers that buy from other
// suppliers — the same screens, the same data, each under its own dashboard's
// keys (rbac.Capability). commerce.orders.organization_id is the BUYER, so
// every dataset over orders is scoped through the order, never through a
// line's or shipment's organization_id — those name the supplier.

// keys resolves capabilities to the permission keys one dashboard grants them
// under.
func keys(scope rbac.Scope, caps ...rbac.Capability) []string {
	out := rbac.RequiredKeys(scope, caps...)
	if scope == rbac.ScopeVendor {
		for _, c := range caps {
			switch c {
			case rbac.BuyOrderView:
				out = append(out, "vendor.order.view")
			case rbac.BuyPurchaseRequestView:
				out = append(out, "vendor.purchase_request.view")
			case rbac.InvoiceView:
				out = append(out, "vendor.invoice.view", "vendor.order.view")
			case rbac.BuyCartUse:
				out = append(out, "vendor.order.view")
			case rbac.BuySmartOrderView:
				out = append(out, "vendor.order.view")
			}
		}
	}
	return out
}

func buyingDatasets(scope rbac.Scope) []Dataset {
	return []Dataset{
		{
			Name: "purchase_orders", Label: "طلبات الشراء", Scope: scope,
			Description: "Orders this organisation placed as a buyer, one row per order, with totals and status.",
			Permissions: keys(scope, rbac.BuyOrderView),
			From:        `commerce.orders o LEFT JOIN org.branches b ON b.id = o.branch_id`,
			Tenant:      `(o.organization_id = @org OR o.customer_id = @user OR o.customer_id IN (SELECT m.user_id FROM org.members m WHERE m.organization_id = @org AND m.status = 'active')) AND o.deleted_at IS NULL`,
			Fields: []Field{
				search("number", "رقم الطلب", `o.order_number`),
				search("order_number", "رقم الطلب", `o.order_number`),
				enum("status", "الحالة", `o.status`, orderStatuses...),
				enum("order_status", "حالة الطلب", `o.status`, orderStatuses...),
				enum("payment_status", "حالة الدفع", `o.payment_status`, paymentStatuses...),
				enum("payment_method", "طريقة الدفع", `o.payment_method`, paymentMethods...),
				search("branch", "الفرع", localized("b.name")),
				search("supplier", "المورد", `(SELECT string_agg(DISTINCT `+orgName("so")+`, '، ')
					FROM commerce.order_shipments sh JOIN org.organizations so ON so.id = sh.organization_id
					WHERE sh.order_id = o.id)`),
				search("suppliers", "الموردون", `(SELECT string_agg(DISTINCT `+orgName("so")+`, '، ')
					FROM commerce.order_shipments sh JOIN org.organizations so ON so.id = sh.organization_id
					WHERE sh.order_id = o.id)`),
				integer("items", "عدد البنود", `(SELECT count(*) FROM commerce.order_lines l WHERE l.order_id = o.id)`),
				money("subtotal", "الإجمالي قبل الخصم", `o.subtotal`),
				money("discount", "الخصم", `o.discount_amount`),
				money("shipping_fee", "رسوم الشحن", `o.shipping_fee`),
				hidden(money("tax", "الضريبة", `o.tax_amount`)),
				money("total", "الإجمالي", `o.total_amount`),
				money("total_amount", "إجمالي الطلب", `o.total_amount`),
				money("total_price", "إجمالي الطلب", `o.total_amount`),
				hidden(flag("negotiated", "طلب تفاوض", `o.is_negotiation`)),
				hidden(text("negotiation_status", "حالة التفاوض", `o.negotiation_status`)),
				instant("created_at", "تاريخ الطلب", `o.created_at`),
				instant("ordered_at", "تاريخ الطلب", `o.created_at`),
				instant("delivered_at", "تاريخ التسليم", `o.delivered_at`),
				hidden(ref("branch_ref", "مرجع الفرع", `o.branch_id`, handles.KindBranch)),
			},
			Key:         key(`o.id`, handles.KindOrder, entityOrder, "number"),
			DefaultSort: "created_at", TimeField: "created_at",
		},
		{
			Name: "purchase_order_lines", Label: "بنود طلبات الشراء", Scope: scope,
			Description: "Every product line of this organisation's purchase orders: product, supplier, quantity, price. Use for spend by product or supplier.",
			Permissions: keys(scope, rbac.BuyOrderView),
			From: `commerce.order_lines l JOIN commerce.orders o ON o.id = l.order_id
				LEFT JOIN org.organizations s ON s.id = l.organization_id
				LEFT JOIN catalog.products p ON p.id = l.product_id`,
			Tenant: `(o.organization_id = @org OR o.customer_id = @user OR o.customer_id IN (SELECT m.user_id FROM org.members m WHERE m.organization_id = @org AND m.status = 'active')) AND o.deleted_at IS NULL`,
			Fields: []Field{
				search("order_number", "رقم الطلب", `o.order_number`),
				search("number", "رقم الطلب", `o.order_number`),
				enum("order_status", "حالة الطلب", `o.status`, orderStatuses...),
				enum("status", "الحالة", `o.status`, orderStatuses...),
				search("product", "الصنف", `COALESCE(NULLIF(`+localized("p.name")+`, ''), `+localized("l.product_name")+`)`),
				search("sku", "كود الصنف", `l.sku`),
				search("supplier", "المورد", orgName("s")),
				search("suppliers", "الموردون", orgName("s")),
				integer("quantity", "الكمية", `l.quantity`),
				money("unit_price", "سعر الوحدة", `l.unit_price`),
				money("discount", "الخصم", `l.discount_amount`),
				money("total", "الإجمالي", `l.total_price`),
				money("total_price", "إجمالي البند", `l.total_price`),
				money("total_amount", "إجمالي البند", `l.total_price`),
				hidden(flag("negotiated", "سعر تفاوض", `l.is_negotiated`)),
				instant("ordered_at", "تاريخ الطلب", `o.created_at`),
				instant("created_at", "تاريخ الطلب", `o.created_at`),
				ref("order_ref", "مرجع الطلب", `o.id`, handles.KindOrder),
				ref("product_ref", "مرجع الصنف", `l.product_id`, handles.KindProduct),
				ref("variant_ref", "مرجع العرض", `l.product_variant_id`, handles.KindVariant),
				ref("supplier_ref", "مرجع المورد", `l.organization_id`, handles.KindOrgUnit),
			},
			Parents:     map[handles.Kind]string{handles.KindOrder: `l.order_id`},
			DefaultSort: "ordered_at", TimeField: "ordered_at",
		},
		{
			Name: "purchase_shipments", Label: "شحنات طلبات الشراء", Scope: scope,
			Description: "Per-supplier shipments of this organisation's purchase orders with delivery status and dates.",
			Permissions: keys(scope, rbac.BuyOrderView),
			From: `commerce.order_shipments sh JOIN commerce.orders o ON o.id = sh.order_id
				LEFT JOIN org.organizations s ON s.id = sh.organization_id
				LEFT JOIN org.branches b ON b.id = o.branch_id`,
			Tenant: `(o.organization_id = @org OR o.customer_id = @user OR o.customer_id IN (SELECT m.user_id FROM org.members m WHERE m.organization_id = @org AND m.status = 'active')) AND o.deleted_at IS NULL`,
			Fields: []Field{
				search("shipment_number", "رقم الشحنة", `sh.shipment_number`),
				search("order_number", "رقم الطلب", `o.order_number`),
				search("number", "رقم الطلب", `o.order_number`),
				search("supplier", "المورد", orgName("s")),
				search("suppliers", "الموردون", orgName("s")),
				text("branch", "الفرع", localized("b.name")),
				enum("status", "الحالة", `sh.status`, shipmentStatuses...),
				enum("order_status", "حالة الطلب", `o.status`, orderStatuses...),
				money("subtotal", "قيمة البضاعة", `sh.subtotal`),
				money("shipping_fee", "رسوم الشحن", `sh.shipping_fee`),
				money("total", "الإجمالي", `sh.total_amount`),
				money("total_amount", "الإجمالي", `sh.total_amount`),
				hidden(text("carrier", "شركة الشحن", `sh.carrier_name`)),
				hidden(text("tracking_number", "رقم التتبع", `sh.tracking_number`)),
				instant("created_at", "تاريخ الإنشاء", `sh.created_at`),
				instant("ordered_at", "تاريخ الطلب", `sh.created_at`),
				instant("shipped_at", "تاريخ الشحن", `sh.shipped_at`),
				instant("delivered_at", "تاريخ التسليم", `sh.delivered_at`),
				ref("order_ref", "مرجع الطلب", `o.id`, handles.KindOrder),
				ref("supplier_ref", "مرجع المورد", `sh.organization_id`, handles.KindOrgUnit),
			},
			Parents:     map[handles.Kind]string{handles.KindOrder: `sh.order_id`},
			DefaultSort: "created_at", TimeField: "created_at",
		},
		{
			Name: "purchase_order_history", Label: "سجل حالات الطلبات", Scope: scope,
			Description: "Status changes of this organisation's purchase orders over time.",
			Permissions: keys(scope, rbac.BuyOrderView),
			From:        `commerce.order_status_history h JOIN commerce.orders o ON o.id = h.order_id`,
			Tenant:      `(o.organization_id = @org OR o.customer_id = @user OR o.customer_id IN (SELECT m.user_id FROM org.members m WHERE m.organization_id = @org AND m.status = 'active')) AND o.deleted_at IS NULL`,
			Fields: []Field{
				search("order_number", "رقم الطلب", `o.order_number`),
				text("from_status", "من حالة", `h.from_status`),
				text("to_status", "إلى حالة", `h.to_status`),
				search("notes", "ملاحظات", `h.notes`),
				instant("changed_at", "وقت التغيير", `h.created_at`),
			},
			Parents:     map[handles.Kind]string{handles.KindOrder: `h.order_id`},
			DefaultSort: "changed_at", TimeField: "changed_at",
		},
		{
			Name: "purchase_invoices", Label: "فواتير المشتريات", Scope: scope,
			Description: "Invoices suppliers issued to this organisation as a buyer, with due dates and payment status.",
			Permissions: keys(scope, rbac.InvoiceView),
			From: `billing.invoices i LEFT JOIN org.organizations s ON s.id = i.organization_id
				LEFT JOIN commerce.orders o ON o.id = i.order_id`,
			Tenant: `(i.customer_org_id = @org OR o.organization_id = @org OR o.customer_id = @user OR o.customer_id IN (SELECT m.user_id FROM org.members m WHERE m.organization_id = @org AND m.status = 'active'))`,
			Fields: []Field{
				search("number", "رقم الفاتورة", `i.invoice_number`),
				search("supplier", "المورد", orgName("s")),
				search("order_number", "رقم الطلب", `o.order_number`),
				enum("status", "الحالة", `i.status`, invoiceStatuses...),
				day("issue_date", "تاريخ الإصدار", `i.issue_date`),
				day("due_date", "تاريخ الاستحقاق", `i.due_date`),
				money("subtotal", "قبل الضريبة", `i.subtotal`),
				money("tax", "الضريبة", `i.tax_amount`),
				money("discount", "الخصم", `i.discount_amount`),
				money("total", "الإجمالي", `i.total_amount`),
				hidden(text("payment_method", "طريقة الدفع", `i.payment_method`)),
				ref("order_ref", "مرجع الطلب", `i.order_id`, handles.KindOrder),
			},
			Key:         key(`i.id`, handles.KindInvoice, "", "number"),
			Parents:     map[handles.Kind]string{handles.KindOrder: `i.order_id`},
			DefaultSort: "issue_date", TimeField: "issue_date",
		},
		{
			Name: "payments", Label: "المدفوعات", Scope: scope,
			Description: "Payments this organisation made, by method and status.",
			Permissions: keys(scope, rbac.WalletView),
			From: `billing.payments p LEFT JOIN commerce.orders o ON o.id = p.order_id
				LEFT JOIN billing.invoices i ON i.id = p.invoice_id`,
			Tenant: `(p.organization_id = @org OR p.user_id = @user OR o.organization_id = @org OR o.customer_id = @user)`,
			Fields: []Field{
				money("amount", "المبلغ", `p.amount`),
				text("method", "الطريقة", `p.method`),
				text("status", "الحالة", `p.status`),
				search("order_number", "رقم الطلب", `o.order_number`),
				search("invoice_number", "رقم الفاتورة", `i.invoice_number`),
				hidden(search("reference", "المرجع", `p.reference_number`)),
				instant("paid_at", "تاريخ الدفع", `p.paid_at`),
				instant("created_at", "تاريخ التسجيل", `p.created_at`),
			},
			DefaultSort: "created_at", TimeField: "created_at",
		},
		{
			Name: "purchase_requests", Label: "طلبات عروض الأسعار", Scope: scope,
			Description: "Requests for quotation this organisation sent to suppliers.",
			Permissions: keys(scope, rbac.BuyPurchaseRequestView),
			From: `commerce.purchase_requests pr LEFT JOIN org.organizations v ON v.id = pr.vendor_org_id
				LEFT JOIN org.branches b ON b.id = pr.branch_id`,
			Tenant: `(pr.organization_id = @org OR pr.customer_id = @user OR pr.customer_id IN (SELECT m.user_id FROM org.members m WHERE m.organization_id = @org AND m.status = 'active'))`,
			Fields: []Field{
				search("number", "رقم الطلب", `pr.request_number`),
				search("supplier", "المورد", orgName("v")),
				text("branch", "الفرع", localized("b.name")),
				enum("status", "الحالة", `pr.status`, requestStatuses...),
				integer("items", "عدد الأصناف", `pr.total_items`),
				money("estimated_total", "الإجمالي التقديري", `pr.estimated_total`),
				hidden(search("buyer_notes", "ملاحظات الصيدلية", `pr.buyer_notes`)),
				hidden(text("supplier_notes", "رد المورد", `pr.vendor_notes`)),
				instant("created_at", "تاريخ الإرسال", `pr.created_at`),
				instant("responded_at", "تاريخ الرد", `pr.responded_at`),
			},
			Key:         key(`pr.id`, handles.KindRequest, "", "number"),
			DefaultSort: "created_at", TimeField: "created_at",
		},
		{
			Name: "purchase_request_lines", Label: "أصناف طلبات عروض الأسعار", Scope: scope,
			Description: "Products inside this organisation's quotation requests, with target and offered prices.",
			Permissions: keys(scope, rbac.BuyPurchaseRequestView),
			From:        `commerce.purchase_request_lines l JOIN commerce.purchase_requests pr ON pr.id = l.request_id`,
			Tenant:      `(pr.organization_id = @org OR pr.customer_id = @user OR pr.customer_id IN (SELECT m.user_id FROM org.members m WHERE m.organization_id = @org AND m.status = 'active'))`,
			Fields:      requestLineFields(),
			Parents:     map[handles.Kind]string{handles.KindRequest: `l.request_id`},
			DefaultSort: "created_at",
		},
		{
			Name: "cart_items", Label: "سلة المشتريات", Scope: scope,
			Description: "Lines in the signed-in user's current cart for this organisation.",
			Permissions: keys(scope, rbac.BuyCartUse),
			From: `commerce.cart_items ci JOIN commerce.carts c ON c.id = ci.cart_id
				LEFT JOIN catalog.product_variants v ON v.id = ci.product_variant_id
				LEFT JOIN catalog.products p ON p.id = ci.product_id
				LEFT JOIN org.organizations s ON s.id = v.organization_id`,
			Tenant: `c.organization_id = @org AND c.user_id = @user`,
			Fields: []Field{
				search("product", "الصنف", localized("p.name")),
				search("sku", "كود الصنف", `v.sku`),
				search("supplier", "المورد", orgName("s")),
				integer("quantity", "الكمية", `ci.quantity`),
				money("unit_price", "سعر الوحدة", `ci.unit_price`),
				money("line_total", "الإجمالي", `ci.quantity * ci.unit_price`),
				instant("added_at", "أضيف في", `ci.created_at`),
				ref("variant_ref", "مرجع العرض", `ci.product_variant_id`, handles.KindVariant),
				ref("supplier_ref", "مرجع المورد", `v.organization_id`, handles.KindOrgUnit),
			},
			Key:         key(`ci.id`, KindCartLine, "", "product"),
			DefaultSort: "added_at",
		},
		{
			Name: "smart_order_runs", Label: "عمليات الطلب الذكي", Scope: scope,
			Description: "Smart-order uploads: how many lines matched, had no supplier, and the estimated total.",
			Permissions: keys(scope, rbac.BuySmartOrderView),
			From:        `smartorder.runs r LEFT JOIN org.branches b ON b.id = r.branch_id`,
			Tenant:      `r.organization_id = @org`,
			Fields: []Field{
				search("number", "رقم العملية", `r.run_number`),
				search("file", "الملف", `r.original_filename`),
				text("branch", "الفرع", localized("b.name")),
				text("status", "الحالة", `r.status`),
				integer("total_rows", "إجمالي الأسطر", `r.total_rows`),
				integer("matched_rows", "المطابقة", `r.matched_rows`),
				integer("unmatched_rows", "غير المطابقة", `r.unmatched_rows`),
				integer("no_supplier_rows", "بلا مورد", `r.no_supplier_rows`),
				hidden(integer("coverage_blocked_rows", "خارج التغطية", `r.coverage_blocked_rows`)),
				hidden(integer("quota_blocked_rows", "محجوبة بالكوتة", `r.quota_blocked_rows`)),
				money("estimated_total", "الإجمالي التقديري", `r.estimated_total`),
				flag("budget_exceeded", "تجاوز الميزانية", `r.budget_exceeded`),
				instant("created_at", "تاريخ الرفع", `r.created_at`),
				instant("finalized_at", "تاريخ الاعتماد", `r.finalized_at`),
			},
			Key:         key(`r.id`, handles.KindSmartRun, "", "number"),
			DefaultSort: "created_at", TimeField: "created_at",
		},
	}
}

// companyDatasets are the organisation's own records both trading dashboards
// have, each under its own dashboard's keys.
func companyDatasets(scope rbac.Scope) []Dataset {
	prefix := "pharmacy."
	if scope == rbac.ScopeVendor {
		prefix = "vendor."
	}
	return []Dataset{
		walletTransactions(scope, keys(scope, rbac.WalletView)),
		branches(scope, []string{prefix + "branch.view"}),
		team(scope, []string{prefix + "team.view"}),
		subscriptions(scope, []string{prefix + "subscription.view"}),
	}
}

// Declarations shared by the pharmacy and vendor dashboards. Each takes the
// dashboard's own permission, so the two can never be granted by one key.

func walletTransactions(scope rbac.Scope, perms []string) Dataset {
	return Dataset{
		Name: "wallet_transactions", Label: "حركات المحفظة", Scope: scope,
		Description: "Every wallet movement of this organisation: deposits, purchases, refunds, withdrawals, with running balance.",
		Permissions: perms,
		From:        `billing.wallet_transactions t JOIN billing.wallets w ON w.id = t.wallet_id`,
		Tenant:      `w.organization_id = @org`,
		Fields: []Field{
			enum("type", "النوع", `t.type`, walletTxTypes...),
			money("amount", "المبلغ", `t.amount`),
			money("balance_after", "الرصيد بعد الحركة", `t.balance_after`),
			text("reference_type", "مرتبط بـ", `t.reference_type`),
			search("description", "الوصف", `t.description`),
			instant("created_at", "التاريخ", `t.created_at`),
		},
		DefaultSort: "created_at", TimeField: "created_at",
	}
}

func requestLineFields() []Field {
	return []Field{
		search("request_number", "رقم الطلب", `pr.request_number`),
		search("product", "الصنف", `l.product_name`),
		search("sku", "كود الصنف", `l.product_sku`),
		integer("quantity", "الكمية", `l.quantity`),
		money("target_price", "السعر المستهدف", `l.target_price`),
		number("target_discount", "الخصم المستهدف %", `l.target_discount`),
		money("offered_price", "السعر المعروض", `l.offered_price`),
		number("offered_discount", "الخصم المعروض %", `l.offered_discount`),
		text("status", "الحالة", `l.status`),
		hidden(text("notes", "ملاحظات", `l.notes`)),
		instant("created_at", "التاريخ", `l.created_at`),
		ref("request_ref", "مرجع الطلب", `l.request_id`, handles.KindRequest),
	}
}

func branches(scope rbac.Scope, perms []string) Dataset {
	return Dataset{
		Name: "branches", Label: "الفروع", Scope: scope,
		Description: "This organisation's branches.",
		Permissions: perms,
		From:        `org.branches b`,
		Tenant:      `b.organization_id = @org AND b.deleted_at IS NULL`,
		Fields: []Field{
			search("name", "الفرع", localized("b.name")),
			search("code", "الكود", `b.code`),
			flag("is_main", "رئيسي", `b.is_main`),
			text("status", "الحالة", `b.status`),
			text("phone", "الهاتف", `b.phone`),
			search("address", "العنوان", `b.address`),
			hidden(text("manager", "المدير", `b.manager_name`)),
			hidden(text("operating_hours", "ساعات العمل", `b.operating_hours`)),
			instant("created_at", "تاريخ الإضافة", `b.created_at`),
		},
		Key:         key(`b.id`, handles.KindBranch, entityBranch, "name"),
		DefaultSort: "created_at",
	}
}

// team never selects salary columns: the team screen's view permission does
// not show pay, and nothing here should either.
func team(scope rbac.Scope, perms []string) Dataset {
	return Dataset{
		Name: "team", Label: "فريق العمل", Scope: scope,
		Description: "Members of this organisation: role, branch, status.",
		Permissions: perms,
		From: `org.members m JOIN identity.users u ON u.id = m.user_id
			LEFT JOIN org.branches b ON b.id = m.branch_id
			LEFT JOIN org.roles r ON r.id = m.org_role_id`,
		Tenant: `m.organization_id = @org`,
		Fields: []Field{
			search("name", "الاسم", personName("u")),
			search("email", "البريد", `u.email`),
			hidden(text("phone", "الهاتف", `u.phone`)),
			search("role", "الدور", `COALESCE(NULLIF(`+localized("r.name")+`,''), m.role_key)`),
			search("job_title", "المسمى الوظيفي", `m.job_title`),
			text("branch", "الفرع", localized("b.name")),
			text("status", "الحالة", `m.status`),
			flag("active", "نشط", `m.is_active`),
			instant("joined_at", "تاريخ الانضمام", `COALESCE(m.joined_at, m.created_at)`),
		},
		DefaultSort: "joined_at",
	}
}

func subscriptions(scope rbac.Scope, perms []string) Dataset {
	return Dataset{
		Name: "subscriptions", Label: "الاشتراكات", Scope: scope,
		Description: "Subscription history of this organisation. The current plan is the latest starts_at.",
		Permissions: perms,
		From:        `billing.subscriptions s LEFT JOIN billing.plans p ON p.id = s.plan_id`,
		Tenant:      `s.organization_id = @org`,
		Fields: []Field{
			search("plan", "الباقة", localized("p.name")),
			text("status", "الحالة", `s.status`),
			text("billing_cycle", "دورة الفوترة", `s.billing_cycle`),
			flag("auto_renew", "تجديد تلقائي", `s.auto_renew`),
			instant("starts_at", "تبدأ", `s.starts_at`),
			instant("expires_at", "تنتهي", `s.expires_at`),
		},
		DefaultSort: "starts_at",
	}
}
