# Error codes: ERP Integration API

Codes are stable. Never rename or reuse one; add a new code instead. Every code has an i18n key `erp.err.<code in lower case without the ERP_ prefix>` declared in `internal/shared/i18n/catalog_erp.go` (function `loadErpKeys`, registered in `internal/shared/i18n/catalog.go` `loadCatalogDefaults`). The HTTP layer builds `message` as `{"ar": i18n.T(i18n.AR, key), "en": i18n.T(i18n.EN, key)}`. Do not put Arabic literals in Go (`make check-hardcoded-arabic`).

Messages that need a number use `%d`/`%s` placeholders filled from `details`.

Envelope:
```json
{"status":"error","requestId":"9f2c...","error":{"code":"ERP_SCOPE_MISSING","message":{"ar":"...","en":"..."},"details":{"requiredScope":"erp.orders.read"}}}
```

## Authentication and access

| Code | HTTP | Arabic | English | details |
|---|---|---|---|---|
| ERP_AUTH_MISSING | 401 | مفتاح الواجهة غير موجود في الطلب. | No API key was sent. | |
| ERP_AUTH_INVALID | 401 | مفتاح الواجهة غير صحيح. | The API key is not valid. | |
| ERP_KEY_REVOKED | 401 | تم إلغاء هذا المفتاح. | This API key was revoked. | revokedAt |
| ERP_KEY_EXPIRED | 401 | انتهت صلاحية هذا المفتاح. | This API key has expired. | expiredAt |
| ERP_ORG_NOT_APPROVED | 403 | المنشأة غير معتمدة حالياً ولا يمكن استخدام المفتاح. | The organization is not approved, so the key cannot be used. | organizationStatus |
| ERP_INTEGRATION_PAUSED | 403 | تم إيقاف هذا الربط مؤقتاً. | This integration is paused. | integrationStatus |
| ERP_SCOPE_MISSING | 403 | المفتاح لا يملك الصلاحية المطلوبة لهذه العملية. | The key does not have the permission this call needs. | requiredScope |
| ERP_CAPABILITY_REQUIRED | 403 | هذه العملية متاحة للمورّدين فقط. | This call is available to suppliers only. | organizationType |
| ERP_RATE_LIMITED | 429 | تجاوزت الحد المسموح من الطلبات. أعد المحاولة بعد %d ثانية. | Too many requests. Retry in %d seconds. | retryAfterSeconds, limitPerMinute |
| ERP_DISABLED | 503 | واجهة الربط غير مفعلة حالياً. | The integration API is switched off. | |

## Request shape

| Code | HTTP | Arabic | English | details |
|---|---|---|---|---|
| ERP_BAD_JSON | 400 | تعذر قراءة محتوى الطلب. | The request body could not be read as JSON. | position |
| ERP_UNSUPPORTED_MEDIA | 415 | نوع المحتوى يجب أن يكون application/json. | Content-Type must be application/json. | |
| ERP_BODY_TOO_LARGE | 413 | حجم الطلب أكبر من الحد المسموح (%d ميجابايت). | The request is larger than the limit (%d MB). | limitBytes |
| ERP_VALIDATION | 422 | بيانات الطلب غير صالحة. | The request data is not valid. | fields |
| ERP_CURSOR_INVALID | 400 | مؤشر الصفحات غير صالح. ابدأ من جديد بدون مؤشر. | The page cursor is not valid. Start again without a cursor. | |
| ERP_NOT_FOUND | 404 | العنصر المطلوب غير موجود. | The requested item was not found. | |
| ERP_IDEMPOTENCY_CONFLICT | 409 | استُخدم مفتاح منع التكرار نفسه مع محتوى مختلف. | The same idempotency key was used with different content. | |
| ERP_INTERNAL | 500 | حدث خطأ غير متوقع. اذكر مرجع الطلب عند التواصل معنا. | Something went wrong on our side. Quote the request id when you contact us. | |
| ERP_UNAVAILABLE | 503 | الخدمة غير متاحة مؤقتاً. أعد المحاولة بعد قليل. | The service is temporarily unavailable. Try again shortly. | |

## Product sync (request level)

| Code | HTTP | Arabic | English | details |
|---|---|---|---|---|
| ERP_BATCH_EMPTY | 422 | الدفعة لا تحتوي على أصناف. | The batch has no items. | |
| ERP_BATCH_TOO_LARGE | 413 | عدد الأصناف أكبر من الحد المسموح (%d صنف في الدفعة). | The batch has more items than allowed (%d per batch). | limit, received |
| ERP_SYNC_NOT_FOUND | 404 | لم يتم العثور على عملية المزامنة. | The sync was not found. | |

## Product sync (item level)

These appear in `items[].code`, with `state` `failed` unless noted. The batch itself is accepted.

| Code | State | Arabic | English |
|---|---|---|---|
| ERP_ITEM_CODE_REQUIRED | failed | كود الصنف مطلوب. | The item code is required. |
| ERP_ITEM_NAME_REQUIRED | failed | اسم الصنف مطلوب. | The item name is required. |
| ERP_ITEM_PRICE_INVALID | failed | السعر مطلوب ويجب أن يكون رقماً غير سالب. | The price is required and must be a non-negative number. |
| ERP_ITEM_PRICE_PRECISION | failed | السعر يحتوي على أكثر من خانتين عشريتين ولن يتم تقريبه. | The price has more than two decimals and is not rounded. |
| ERP_ITEM_STOCK_INVALID | failed | الرصيد مطلوب ويجب أن يكون عدداً صحيحاً. | The stock is required and must be a whole number. |
| ERP_ITEM_STOCK_NEGATIVE | failed | الرصيد لا يمكن أن يكون سالباً. | The stock cannot be negative. |
| ERP_ITEM_DISCOUNT_INVALID | failed | الخصم يجب أن يكون نسبة بين 0 و 100 بحد أقصى خانتين عشريتين. | The discount must be a percentage from 0 to 100 with at most two decimals. |
| ERP_ITEM_TAX_INVALID | failed | الضريبة يجب أن تكون نسبة بين 0 و 100 بحد أقصى خانتين عشريتين. | The tax must be a percentage from 0 to 100 with at most two decimals. |
| ERP_ITEM_QUOTA_INVALID | failed | كمية الحصة يجب أن تكون عدداً صحيحاً أكبر من صفر. | The quota quantity must be a whole number above zero. |
| ERP_ITEM_EXPIRY_INVALID | failed | تاريخ الصلاحية غير صالح. استخدم الصيغة YYYY-MM-DD. | The expiry date is not valid. Use YYYY-MM-DD. |
| ERP_ITEM_BARCODE_INVALID | failed | الباركود يجب أن يكون أرقاماً فقط وبطول لا يزيد عن 32. | The barcode must be digits only, up to 32 long. |
| ERP_ITEM_SUPERSEDED | unchanged | تم تجاهل هذا السطر لأن نفس الكود ظهر لاحقاً في الدفعة. | Ignored because the same code appears later in the batch. |
| ERP_ITEM_MATCH_REVIEW | needs_review | الصنف يحتاج مراجعة المورد لتحديد المنتج الصحيح. | The item needs the supplier to pick the right product. |
| ERP_ITEM_BARCODE_CONFLICT | needs_review | الباركود مرتبط بأكثر من صنف لدى المورد. | The barcode belongs to more than one of the supplier's products. |
| ERP_ITEM_DELETED_LINK | needs_review | الصنف المرتبط بهذا الكود تم حذفه في دواء 24. | The product linked to this code was deleted in Dawa24. |
| ERP_ITEM_MATCHER_UNAVAILABLE | needs_review | تعذر تشغيل المطابقة مؤقتاً وسيظل الصنف في المراجعة. | Matching was unavailable; the item stays in review. |
| ERP_ITEM_WRITE_FAILED | failed | تعذر حفظ الصنف. أعد إرساله في المزامنة التالية. | The item could not be saved. Send it again in the next sync. |

## Orders

| Code | HTTP | Arabic | English | details |
|---|---|---|---|---|
| ERP_ORDER_NOT_FOUND | 404 | الطلب غير موجود. | The order was not found. | |
| ERP_ORDER_STATE_CONFLICT | 409 | حالة الطلب الحالية لا تسمح بهذا الإجراء (%s). | The order's current status does not allow this (%s). | currentStatus |
| ERP_ORDER_NEGOTIATION_PENDING | 409 | الطلب بانتظار قرار التفاوض ويُعالج من الموقع. | The order awaits a price-negotiation decision, handled on the website. | |
| ERP_STATUS_NOT_ALLOWED | 422 | الحالة المطلوبة غير مسموحة. المسموح: confirmed أو rejected. | That status is not allowed. Allowed: confirmed or rejected. | allowed |
| ERP_DELIVERY_NOT_ALLOWED | 403 | لا يمكن تسجيل التسليم من النظام لأنه يحتاج الرقم السري للمشتري. | Delivery cannot be set from the ERP because it needs the buyer's PIN. | |
| ERP_WALLET_INSUFFICIENT | 409 | رصيد محفظة المشتري لا يكفي لتأكيد الطلب. | The buyer's wallet cannot cover this order. | requiredAmount, availableAmount |
| ERP_ACK_REFERENCE_CONFLICT | 409 | هذا الطلب مؤكد الاستلام بمرجع مختلف. | This order was already acknowledged with a different reference. | existingReference |

`ERP_ORDER_STATE_CONFLICT` has two sub-cases: the status moved in Dawa24 (details.currentStatus), and the order is not `pending`.

A repeated confirm or reject returns HTTP 200 with `data.result` equal to `already_confirmed` or `already_rejected`; it is not an error.

## Purchase orders

| Code | HTTP | Arabic | English | details |
|---|---|---|---|---|
| ERP_PO_REFERENCE_REQUIRED | 422 | المرجع الفريد للطلب (externalReference) مطلوب. | The unique request reference (externalReference) is required. | |
| ERP_PO_BUYING_USER_MISSING | 409 | لم يتم تحديد مستخدم الشراء للمفتاح أو لم يعد عضواً نشطاً. | The key has no buying user, or that user is no longer an active member. | |
| ERP_PO_ITEM_NOT_FOUND | 422 | الصنف رقم %d غير موجود. | Line %d refers to a product that does not exist. | line, variantId |
| ERP_PO_NO_ACTIVE_OFFER | 422 | لا يوجد عرض نشط للصنف رقم %d. | Line %d has no active offer. | line, variantId |
| ERP_PO_QUANTITY_INVALID | 422 | الكمية في السطر %d يجب أن تكون عدداً صحيحاً موجباً. | The quantity on line %d must be a positive whole number. | line |
| ERP_PO_PRICE_MISMATCH | 409 | السعر المرسل يختلف عن سعر دواء 24. السعر الصحيح موضح في التفاصيل. | The price sent differs from Dawa24's. The correct price is in the details. | lines[{line, sentPrice, correctPrice}] |
| ERP_PO_SUPPLIER_NOT_LINKED | 422 | المورد في السطر %d غير مرتبط بحساب منشأتك. | The supplier on line %d is not linked to your organization. | line, supplierName |
| ERP_PO_SELF_SUPPLY | 422 | لا يمكن الشراء من منشأتك نفسها. | You cannot buy from your own organization. | |
| ERP_PO_REFUSED | 409 | رفضت قواعد الشراء الطلب: %s | The ordering rules refused the order: %s | reason (a Dawa24 error code such as `quota.exhausted`) |
| ERP_PO_IN_PROGRESS | 409 | طلب الشراء بنفس المرجع قيد المعالجة. أعد المحاولة بعد دقيقة. | A purchase order with this reference is still being processed. Retry in a minute. | |
| ERP_PO_PAYMENT_NOT_SUPPORTED | 409 | الدفع بالمحفظة غير متاح عبر واجهة الربط. | Wallet payment is not available through the integration API. | |

## Internal codes (Reveal and the web screens, not part of the ERP API)

| Code | HTTP | Arabic | English |
|---|---|---|---|
| REVEAL_NOT_REVEALABLE | 403 | هذه القيمة لا يمكن إظهارها. | This value can never be revealed. |
| REVEAL_FORBIDDEN | 403 | لا تملك صلاحية إظهار هذه القيمة. | You are not allowed to reveal this value. |
| REVEAL_NOT_FOUND | 404 | القيمة غير موجودة. | The value was not found. |
| ERP_CODE_TAKEN | 409 | هذا الكود مستخدم لعميل آخر لدى هذا المورد. | This code is already used by another customer of this supplier. |
