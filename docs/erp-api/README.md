# Dawa24 ERP Integration API

This page explains how to connect an ERP system to Dawa24. The partner contract is
`internal/modules/erp/http/spec/openapi.yaml` (served at `GET /erp/v1/openapi.yaml`).
The Postman collection `dawa24-erp.postman_collection.json` runs the whole journey.

## العربية

### ما هذه الواجهة؟
واجهة برمجية تسمح لنظام المورّد (مثل Orgasoft) بإرسال الأصناف والمخزون، وقراءة طلبات
الصرف، وتأكيد استلامها، وقبول الطلب أو رفضه. كل المفاتيح تعمل لمنشأة واحدة فقط.

### 1. احصل على مفتاح
يُنشئ مالك المورّد المفتاح من الموقع: `/vendor/integrations` ← "مفتاح جديد". يظهر المفتاح
مرة واحدة فقط؛ انسخه وخزّنه. يبدأ المفتاح بـ `d24_live_` أو `d24_test_`.

### 2. أول طلب
أرسل `GET {baseUrl}/erp/v1/health` مع الترويسة `Authorization: Bearer <المفتاح>`. الرد
`200` يعني أن العنوان والمفتاح صحيحان.

### 3. ترتيب مجلدات Postman
1. ابدأ هنا: التحقق من المفتاح.
2. الأصناف: إرسال دفعة الأصناف ومتابعتها ونتيجة كل صنف.
3. طلبات البيع: جلب الطلبات الجديدة وطلب واحد وتأكيد الاستيراد وقبول الطلب أو رفضه.
4. طلبات الشراء: طلبات الشراء وتأكيد استيرادها. البحث عن العروض وإنشاء طلب الشراء
   والبحث عن العروض وإنشاء طلب الشراء (القسم 9).

### 4. الرحلة الكاملة (13 خطوة)
1. تحقق من المفتاح (`/health`).
2. أرسل دفعة أصناف فيها صنف جديد، وصنف معروف، وصنف بسعر غير صالح.
3. تابع حالة الإرسال حتى تصبح `completed`.
4. اقرأ نتيجة كل صنف؛ الصنف ذو السعر غير الصالح يظهر `failed` مع رمز الخطأ.
5. أرسل الدفعة نفسها مرة ثانية؛ الأصناف المعروفة تظهر `unchanged` ولا تتكرر.
6. اطلب من الصيدلية إدخال رقم عميلها لدى المورّد عند الدفع (تظهر نافذة الرقم).
7. اقرأ طلبات البيع الجديدة؛ يظهر رقم العميل لدى المورّد في `customer.erpCode`.
8. أكّد استيراد الطلب.
9. اقرأ الطلبات الجديدة مرة أخرى؛ لن يظهر الطلب المستورد.
10. أكّد الطلب (`confirmed`).
11. أكّد الطلب نفسه مرة ثانية؛ يعود `already_confirmed` ولا يحدث تغيير.
12. جرّب تسليم الطلب؛ الرد `403 ERP_DELIVERY_NOT_ALLOWED`، لأن التسليم لا يُضبط من الواجهة.
13. الشراء: البحث عن العروض ثم إنشاء طلب شراء (القسم 9).

### 5. قراءة الطلبات (الاستطلاع)
ابدأ كل دورة بدون `cursor` واطلب `state=new`. الـ `cursor` يتنقّل داخل الدورة الواحدة فقط،
فلا يُفوَّت طلب ولا يتكرر. الطلب الذي تستورده يختفي من `state=new` في الدورة التالية.

### 6. إعادة المحاولة والتكرار
- أرسل `Idempotency-Key` مع دفعة الأصناف؛ إعادة الإرسال بالمفتاح نفسه تعيد `syncId` نفسه.
- إعادة تأكيد الاستيراد بالمرجع نفسه آمنة، والمرجع الجديد لطلب سبق تأكيده يُرفض لذلك الطلب فقط.
- عند `429` اقرأ الترويسة `Retry-After` وانتظر.

### 7. حدود الاستخدام
- 600 طلب في الدقيقة لكل مفتاح (قابلة للتعديل لكل مفتاح بين 10 و6000).
- 6 دفعات أصناف في الدقيقة.
- حتى 50,000 صنف و32 ميغابايت في كل دفعة (يُقبل `Content-Encoding: gzip`).
- حتى 500 طلب في كل تأكيد استيراد.

### 8. رموز الخطأ
كل رد خطأ يحمل `error.code` ورسالتين `ar` و`en`. الجدول الكامل في آخر هذه الصفحة.

---

### 9. إنشاء طلب شراء والبحث عن العروض
- `GET /erp/v1/offers?barcode=...` يعيد حتى 50 عرضاً مرتبة من الأرخص، فيه `variantId` و`unitPrice` و`supplier.linked`، ويحتاج صلاحية `erp.po.create`.
- `POST /erp/v1/orders/purchase` ينشئ طلب شراء على دواء24 بالأسعار الخاصة بدواء24. إذا أرسلت `unitPrice` ولم يطابق السعر، يُرفض الطلب بـ `ERP_PO_PRICE_MISMATCH` ويظهر السعر الصحيح في `details.lines`.
- أعد إرسال نفس `externalReference` عند انقطاع الرد؛ ستحصل على الطلب الأول بالرد `200` ولن يُنشأ طلب ثانٍ.
- المورّد يجب أن يكون مرتبطاً بمنشأتك (`ERP_PO_SUPPLIER_NOT_LINKED`)، ولا يُنشأ أي حساب أو ربط عند الرفض.
- الدفع بالمحفظة غير متاح عبر الواجهة (`ERP_PO_PAYMENT_NOT_SUPPORTED`).

## English

### What this API is
A machine API that lets a supplier's ERP (first consumer: Orgasoft) push products and stock,
read the orders it received, acknowledge them, and confirm or reject them. A key belongs to
one organization, and every call sees only that organization's data.

### 1. Get a key
The supplier owner creates a key on the website at `/vendor/integrations` ("New key"). The
key is shown once. It starts with `d24_live_` or `d24_test_`.

### 2. First call
`GET {baseUrl}/erp/v1/health` with `Authorization: Bearer <key>`. A `200` means the address
and key are correct.

### 3. Postman folders
1. Start here: check the key.
2. Products: send a batch, check the upload, read each product's result.
3. Sales orders: read new orders, read one order, mark orders as imported, confirm or reject.
4. Purchase orders: read purchase orders and mark them as imported. Find offers and create a
   purchase order (section 9 of the Arabic part).

### 4. The full journey (13 steps)
The journey is the same as in the Arabic section above (steps 1 to 13). Each step is one
request in the collection, in folder order.

### 5. Polling orders
Start each cycle without a `cursor` and filter `state=new`. The cursor pages only inside one
cycle, so no order is skipped or repeated. An order you acknowledge leaves `state=new` in the
next cycle.

### 6. Retries and idempotency
- Send `Idempotency-Key` with a product batch. The same key returns the same `syncId`.
- Acknowledging the same reference again is safe. A new reference for an order already
  acknowledged is refused for that order only.
- Confirming or rejecting an order again returns the current result and changes nothing.

### 7. Rate limits and sizes
- 600 requests per minute per key by default; the limit can be set per key between 10 and 6000.
- 6 product batches per minute.
- Up to 50,000 products and 32 MiB per batch (`Content-Encoding: gzip` is accepted).
- Up to 500 orders per acknowledgement call.
- On `429`, wait for the `Retry-After` header.

### 8. Money, names and fields
- Money is a decimal string with at most two decimals, for example `"125.50"`. A JSON number is
  accepted, but more than two decimals is refused, never rounded.
- `discount` and `tax` are percentages.
- Input field names may be camelCase or PascalCase. Output is always camelCase.
- Every message has an Arabic (`ar`) and an English (`en`) text.

### Field aliases
| Canonical name | Accepted spellings (first letter case ignored) |
|---|---|
| externalProductCode | externalProductCode, ExternalProductCode |
| arName | arName, ArName |
| publicPrice | publicPrice, PublicPrice |
| stock | stock, Stock |
| discount | discount, Discount |
| tax | tax, Tax |
| isQuota | isQuota, IsQuota |
| quotaQuantity | quotaQuantity, QuotaQuantity |
| barcode | barcode, Barcode |
| expiryDate | expiryDate, ExpiryDate |
| unit | unit, Unit (optional; ignored when absent) |
| manufacturer | manufacturer, Manufacturer (optional; ignored when absent) |
| batchNumber | batchNumber, BatchNumber (optional; ignored when absent) |

### Endpoints

#### GET /erp/v1/health
Checks the key and shows the organization, the scopes it has, and its warehouse. Works with any valid key.

#### POST /erp/v1/products/sync
Sends a batch of changed products (scope `erp.products.sync`, supplier organizations only).
Returns `202` with a `syncId`. A product missing from the batch is left unchanged. Stock `0`
shows the product out of stock. A repeated code in one batch: the last row wins.
Errors: `ERP_BATCH_EMPTY`, `ERP_BATCH_TOO_LARGE`, `ERP_IDEMPOTENCY_CONFLICT`. A bad product inside a batch does not fail the call; it shows in the item results with its own code (for example `ERP_ITEM_PRICE_PRECISION`).

#### GET /erp/v1/products/sync/{syncId}
The overall state of a batch: `queued`, `processing`, `completed` or `failed`. A completed batch can still hold failed items.

#### GET /erp/v1/products/sync/{syncId}/items
The result of each item in a batch. Filter with `state=pending|created|updated|unchanged|needs_review|failed`; page with `cursor` and `limit` (default 1000, maximum 5000).

#### GET /erp/v1/orders/sales
The orders the supplier received (scope `erp.orders.read`). Each order carries only the
supplier's own part (its shipment). Filter with `state=new|imported|changed|all` (default `new`); page with `cursor` and `limit` (default 100, maximum 500).

#### GET /erp/v1/orders/sales/{orderNumber}
One sales order, with the same fields as the list.

#### POST /erp/v1/orders/sales/ack
Marks up to 500 orders as imported (scope `erp.orders.ack`). Each order returns its own result:
`acknowledged`, `already_acknowledged` or `conflict` (`ERP_ACK_REFERENCE_CONFLICT`).

#### POST /erp/v1/orders/sales/{orderNumber}/status
Confirms or rejects a pending order (scope `erp.orders.decide`). Confirming a wallet-paid order
charges the buyer's wallet; rejecting refunds it. Delivery is not set through the API
(`ERP_DELIVERY_NOT_ALLOWED`).

#### GET /erp/v1/orders/purchase
The orders the key's organization placed (scope `erp.orders.read`; pharmacies and suppliers).

#### GET /erp/v1/orders/purchase/{orderNumber}
One purchase order.

#### POST /erp/v1/orders/purchase/ack
Marks purchase orders as imported (scope `erp.orders.ack`). Same behaviour as the sales acknowledgement.

#### POST /erp/v1/orders/purchase
Creates a purchase order (scope `erp.po.create`). Prices are Dawa24's; a sent `unitPrice` that
differs is refused with `ERP_PO_PRICE_MISMATCH`. Retrying with the same `externalReference`
returns the first order. Body: `externalReference` (required, up to 100 characters), `notes` (optional, up to 500),
`lines[{variantId, quantity, unitPrice?}]` (1 to 200 lines; `quantity` is a whole number from 1 to 100000;
`unitPrice` is a string or number, optional). The reply is `201` for a new order and `200` for a replay.
The buyer key needs a buying user, and each supplier must already be linked to your organization. No account is ever created.

#### GET /erp/v1/offers
Active supplier products with stock for one barcode, to get the `variantId` for a purchase order (scope `erp.po.create`). Query: `barcode` (required). At most 50 offers, cheapest first, with `unitPrice` (the price after discount) and `supplier.linked`.

## Error codes

Every code has a stable name. Each row gives the HTTP status, the Arabic and English text, and
the fields in `details`.

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
| ERP_BAD_JSON | 400 | تعذر قراءة محتوى الطلب. | The request body could not be read as JSON. | position |
| ERP_UNSUPPORTED_MEDIA | 415 | نوع المحتوى يجب أن يكون application/json. | Content-Type must be application/json. | |
| ERP_BODY_TOO_LARGE | 413 | حجم الطلب أكبر من الحد المسموح (%d ميجابايت). | The request is larger than the limit (%d MB). | limitBytes |
| ERP_VALIDATION | 422 | بيانات الطلب غير صالحة. | The request data is not valid. | fields |
| ERP_CURSOR_INVALID | 400 | مؤشر الصفحات غير صالح. ابدأ من جديد بدون مؤشر. | The page cursor is not valid. Start again without a cursor. | |
| ERP_NOT_FOUND | 404 | العنصر المطلوب غير موجود. | The requested item was not found. | |
| ERP_IDEMPOTENCY_CONFLICT | 409 | استُخدم مفتاح منع التكرار نفسه مع محتوى مختلف. | The same idempotency key was used with different content. | |
| ERP_INTERNAL | 500 | حدث خطأ غير متوقع. اذكر مرجع الطلب عند التواصل معنا. | Something went wrong on our side. Quote the request id when you contact us. | |
| ERP_UNAVAILABLE | 503 | الخدمة غير متاحة مؤقتاً. أعد المحاولة بعد قليل. | The service is temporarily unavailable. Try again shortly. | |
| ERP_BATCH_EMPTY | 422 | الدفعة لا تحتوي على أصناف. | The batch has no items. | |
| ERP_BATCH_TOO_LARGE | 413 | عدد الأصناف أكبر من الحد المسموح (%d صنف في الدفعة). | The batch has more items than allowed (%d per batch). | limit, received |
| ERP_SYNC_NOT_FOUND | 404 | لم يتم العثور على عملية المزامنة. | The sync was not found. | |
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
| ERP_ORDER_NOT_FOUND | 404 | الطلب غير موجود. | The order was not found. | |
| ERP_ORDER_STATE_CONFLICT | 409 | حالة الطلب الحالية لا تسمح بهذا الإجراء (%s). | The order's current status does not allow this (%s). | currentStatus |
| ERP_ORDER_NEGOTIATION_PENDING | 409 | الطلب بانتظار قرار التفاوض ويُعالج من الموقع. | The order awaits a price-negotiation decision, handled on the website. | |
| ERP_STATUS_NOT_ALLOWED | 422 | الحالة المطلوبة غير مسموحة. المسموح: confirmed أو rejected. | That status is not allowed. Allowed: confirmed or rejected. | allowed |
| ERP_DELIVERY_NOT_ALLOWED | 403 | لا يمكن تسجيل التسليم من النظام لأنه يحتاج الرقم السري للمشتري. | Delivery cannot be set from the ERP because it needs the buyer's PIN. | |
| ERP_WALLET_INSUFFICIENT | 409 | رصيد محفظة المشتري لا يكفي لتأكيد الطلب. | The buyer's wallet cannot cover this order. | requiredAmount, availableAmount |
| ERP_ACK_REFERENCE_CONFLICT | 409 | هذا الطلب مؤكد الاستلام بمرجع مختلف. | This order was already acknowledged with a different reference. | existingReference |
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
| ERP_CODE_TAKEN | 409 | هذا الكود مستخدم لعميل آخر لدى هذا المورد. | This code is already used by another customer of this supplier. |
