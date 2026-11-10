"""Build the partner-facing, plain-language Postman collection for Orgasoft from the Dawa24 ERP contract.

Requests, bodies and saved replies come from openapi.yaml (the contract the server is tested against);
the explanations are hand-written here in simple Arabic and English.
"""
import json, re, sys, uuid, yaml

src, outdir, catalog = sys.argv[1], sys.argv[2], sys.argv[3]
# The server's own error messages (internal/shared/i18n/catalog_erp.go), so examples show exactly what is sent.
MSG = {k: (ar, en) for k, ar, en in re.findall(r'addKey\(e, "erp\.err\.([a-z_]+)", "erp", "([^"]*)", "([^"]*)"', open(catalog, encoding="utf-8").read())}

AUTH = [(401, "Wrong or missing key", "ERP_AUTH_INVALID", None),
        (403, "Key lacks permission", "ERP_SCOPE_MISSING", {"requiredScope": "SCOPE"}),
        (429, "Too many requests", "ERP_RATE_LIMITED", {"retryAfterSeconds": 12, "limitPerMinute": 600})]
ERRORS = {
    "/health": AUTH[:1] + AUTH[2:],
    "/products/sync": AUTH + [(422, "Invalid data", "ERP_BATCH_EMPTY", None),
                              (413, "Too many products", "ERP_BATCH_TOO_LARGE", {"limit": 50000, "received": 50210}),
                              (409, "Same Idempotency-Key, different body", "ERP_IDEMPOTENCY_CONFLICT", None)],
    "/products/sync/{syncId}": [(404, "Upload not found", "ERP_SYNC_NOT_FOUND", None)],
    "/products/sync/{syncId}/items": [(404, "Upload not found", "ERP_SYNC_NOT_FOUND", None),
                                      (400, "Bad page cursor", "ERP_CURSOR_INVALID", None)],
    "/orders/sales": AUTH[1:2] + [(400, "Bad page cursor", "ERP_CURSOR_INVALID", None)],
    "/orders/sales/{orderNumber}": [(404, "Order not found", "ERP_ORDER_NOT_FOUND", None)],
    "/orders/sales/ack": [(422, "Invalid data", "ERP_VALIDATION", {"fields": ["orders"]})],
    "/orders/sales/{orderNumber}/status": [(404, "Order not found", "ERP_ORDER_NOT_FOUND", None),
                                           (409, "Order is no longer pending", "ERP_ORDER_STATE_CONFLICT", {"currentStatus": "confirmed"}),
                                           (422, "Status must be confirmed or rejected", "ERP_STATUS_NOT_ALLOWED", {"allowed": ["confirmed", "rejected"]})],
    "/orders/purchase": AUTH[1:2] + [(400, "Bad page cursor", "ERP_CURSOR_INVALID", None)],
    "/orders/purchase/{orderNumber}": [(404, "Order not found", "ERP_ORDER_NOT_FOUND", None)],
    "/orders/purchase/ack": [(422, "Invalid data", "ERP_VALIDATION", {"fields": ["orders"]})],
    "/orgasoft/orders/sales": AUTH[1:2] + [(400, "Bad page cursor", "ERP_CURSOR_INVALID", None)],
    "/orgasoft/orders/purchase": AUTH[1:2] + [(400, "Bad page cursor", "ERP_CURSOR_INVALID", None)],
    "/offers": [(403, "Key lacks erp.po.create", "ERP_SCOPE_MISSING", {"requiredScope": "erp.po.create"}),
               (422, "Barcode missing", "ERP_VALIDATION", {"fields": ["barcode"]})],
}
PURCHASE_ERRORS = [
    (422, "Wrong quantity on a line", "ERP_PO_QUANTITY_INVALID", {"line": 1}),
    (409, "Price differs from Dawa24's", "ERP_PO_PRICE_MISMATCH", {"lines": [{"line": 1, "sentPrice": "48.00", "correctPrice": "50.40"}]}),
    (422, "Supplier not linked to your organization", "ERP_PO_SUPPLIER_NOT_LINKED", {"line": 1, "supplierName": "Al Nour Pharma"}),
    (422, "No active offer for the product", "ERP_PO_NO_ACTIVE_OFFER", {"line": 1, "variantId": "91530"}),
    (409, "Refused by the ordering rules", "ERP_PO_REFUSED", {"reason": "quota.exhausted"}),
    (409, "Same reference, different body", "ERP_IDEMPOTENCY_CONFLICT", None),
    (409, "Same reference still being processed", "ERP_PO_IN_PROGRESS", None),
]
SCOPE = {"/offers": "erp.po.create", "/health": "", "/products/sync": "erp.products.sync", "/orders/sales": "erp.orders.read", "/orders/purchase": "erp.orders.read", "/orgasoft/orders/sales": "erp.orders.read", "/orgasoft/orders/purchase": "erp.orders.read"}


def error_body(code, details):
    ar, en = MSG[code[4:].lower()]
    ints = [v for v in (details or {}).values() if isinstance(v, int)]
    strs = [v for v in (details or {}).values() if isinstance(v, str)]
    if "%d" in ar and ints:
        ar, en = ar.replace("%d", str(ints[0])), en.replace("%d", str(ints[0]))
    if "%s" in ar and strs:
        ar, en = ar.replace("%s", strs[0]), en.replace("%s", strs[0])
    return {"status": "error", "requestId": "a41f7c2e9b0d4e63",
            "error": {"code": code, "message": {"ar": ar, "en": en}, "details": details}}
d = yaml.safe_load(open(src, encoding="utf-8"))


# (folder, request summary in contract) -> (new name, explanation). Order here is the order shown.
PLAN = [
    ("01 Start here - ابدأ هنا", "Use these first to check that everything is connected. / استخدمها أولاً للتأكد من أن الربط يعمل.", [
        ("GET", "/health", "1. Check my API key - تأكد من مفتاح الربط",
         "**What it does:** checks that your API key works and shows your company name, warehouse and what the key is allowed to do.\n\n"
         "**When:** once at setup, and any time something does not work.\n\n"
         "**ماذا يفعل:** يتأكد أن مفتاح الربط صحيح ويعرض اسم شركتك والمخزن والصلاحيات.\n\n"
         "**متى:** مرة عند الإعداد، وكلما حدثت مشكلة."),
    ]),
    ("02 Products - الأصناف", "Send your products to Dawa24: code, name, price, stock. / أرسل أصنافك إلى دواء24: الكود والاسم والسعر والرصيد.", [
        ("POST", "/products/sync", "1. Send products - إرسال الأصناف",
         "**What it does:** sends your products to Dawa24.\n\n"
         "**Full list or only changes?** Both work. Dawa24 compares every product with the last data it received and "
         "applies only real changes. A product you do not send is left as it is (never deleted). To show a product as "
         "out of stock, send `stock: 0`.\n\n"
         "**Fields:**\n"
         "| Field | Required | Meaning |\n|---|---|---|\n"
         "| externalProductCode | yes | Your product code |\n"
         "| arName | yes | Arabic name |\n"
         "| publicPrice | yes | Public price, max 2 decimals (62.50) |\n"
         "| stock | yes | Quantity available |\n"
         "| barcode | no | Barcode |\n"
         "| discount | no | Discount percent (12.5 = 12.5%) |\n"
         "| tax | no | Tax percent |\n"
         "| expiryDate | no | YYYY-MM-DD |\n"
         "| isQuota / quotaQuantity | no | Limit how much one pharmacy branch can buy |\n\n"
         "Field names can start with a small or capital letter (`arName` or `ArName`). Up to 50,000 products per request.\n\n"
         "**The reply** gives a `syncId`. Use it in the next two requests (it is filled in automatically).\n\n"
         "**ماذا يفعل:** يرسل أصنافك إلى دواء24. يمكنك إرسال القائمة كاملة أو المتغير فقط، ودواء24 يطبق التغييرات الحقيقية فقط. "
         "الصنف الذي لا ترسله يبقى كما هو ولا يُحذف. لإظهار الصنف غير متوفر أرسل الرصيد 0."),
        ("GET", "/products/sync/{syncId}", "2. Check the upload - متابعة الإرسال",
         "**What it does:** shows if the upload finished and how many products were created, updated, unchanged, "
         "need review, or failed.\n\n**When:** a few seconds after step 1. Repeat until `state` is `completed`.\n\n"
         "**ماذا يفعل:** يوضح هل انتهت المعالجة وعدد الأصناف الجديدة والمحدثة وغير المتغيرة والتي تحتاج مراجعة أو فشلت."),
        ("GET", "/products/sync/{syncId}/items", "3. See each product's result - نتيجة كل صنف",
         "**What it does:** lists the result of every product in the upload. Add `state=failed` to see only the "
         "problems; each one has a clear message in Arabic and English.\n\n"
         "**ماذا يفعل:** يعرض نتيجة كل صنف. أضف `state=failed` لعرض الأخطاء فقط مع رسالة واضحة بالعربي والإنجليزي."),
    ]),
    ("03 Sales orders - طلبات البيع", "Orders that pharmacies placed with you. / الطلبات التي أرسلتها الصيدليات إليك.", [
        ("GET", "/orders/sales", "1. Get new orders - جلب الطلبات الجديدة",
         "**What it does:** lists orders pharmacies placed with you. Keep `state=new` to get only orders you have not "
         "imported yet.\n\n**When:** every few minutes.\n\n"
         "**More than one page?** If `page.nextCursor` is not null, call again with `cursor=<that value>`.\n\n"
         "**ماذا يفعل:** يعرض طلبات الصيدليات. اترك `state=new` لجلب الطلبات التي لم تستوردها بعد. استدعه كل بضع دقائق."),
        ("GET", "/orgasoft/orders/sales", "2. Orders in Orgasoft format - الطلبات بصيغة Orgasoft",
         "**What it does:** the same new orders as step 1, written as a plain list in the layout Orgasoft asked for: one entry per shipment, with the pharmacy name, code, phone, city and address, and each item's code, quantity, price and discount percent.\n\n"
         "**More than one page?** The reply header `X-Next-Cursor` holds the cursor for the next page. It is absent on the last page.\n\n"
         "**ماذا يفعل:** نفس الطلبات الجديدة بالشكل الذي طلبته أوركاسوفت: قائمة مباشرة، طلب لكل شحنة، مع اسم الصيدلية وكودها وهاتفها ومدينتها وعنوانها، ولكل صنف كوده وكميته وسعره ونسبة خصمه.\n\n"
         "**الصفحة التالية:** الترويسة `X-Next-Cursor` تحمل مؤشر الصفحة التالية، وتغيب في الصفحة الأخيرة."),
        ("GET", "/orders/sales/{orderNumber}", "3. Get one order - جلب طلب واحد",
         "**What it does:** shows one order with the pharmacy, address, phone and all lines.\n\n"
         "**ماذا يفعل:** يعرض طلباً واحداً بكل تفاصيله: الصيدلية والعنوان والهاتف والأصناف."),
        ("POST", "/orders/sales/ack", "3. Mark orders as imported - تأكيد استيراد الطلبات",
         "**What it does:** tells Dawa24 you saved these orders in your system, with your own reference number. They "
         "will not come back in `state=new`.\n\n**When:** right after you save the orders. Safe to repeat.\n\n"
         "**ماذا يفعل:** يخبر دواء24 أنك حفظت هذه الطلبات عندك مع رقمك المرجعي، فلا تظهر مرة أخرى في الطلبات الجديدة. تكراره آمن."),
        ("POST", "/orders/sales/{orderNumber}/status", "4. Confirm or reject an order - قبول أو رفض الطلب",
         "**What it does:** confirms or rejects a pending order. The pharmacy is notified.\n\n"
         "- Confirm: `{\"status\": \"confirmed\"}`\n- Reject: `{\"status\": \"rejected\", \"reason\": \"...\"}`\n\n"
         "**ماذا يفعل:** يقبل أو يرفض طلباً معلقاً، ويتم إبلاغ الصيدلية. عند الرفض اكتب السبب."),
    ]),
    ("04 Purchase orders - طلبات الشراء", "For pharmacies that use your ERP: their orders from suppliers on Dawa24. / للصيدليات التي تستخدم نظامكم: طلباتها من الموردين على دواء24.", [
        ("GET", "/orders/purchase", "1. Get purchase orders - جلب طلبات الشراء",
         "**What it does:** lists the orders the pharmacy placed with suppliers on Dawa24.\n\n"
         "**ماذا يفعل:** يعرض طلبات الصيدلية من الموردين على دواء24."),
        ("GET", "/orgasoft/orders/purchase", "2. Purchase orders in Orgasoft format - طلبات الشراء بصيغة Orgasoft",
         "**What it does:** the same purchase orders as step 1, as a plain list in Orgasoft's layout: one entry per order with all its lines. The entry describes the ordering pharmacy, and `pharmacyCode` is null.\n\n"
         "**More than one page?** Use the `X-Next-Cursor` header, absent on the last page.\n\n"
         "**ماذا يفعل:** نفس طلبات الشراء بالشكل الذي طلبته أوركاسوفت: طلب واحد لكل طلب شراء مع كل أصنافه. الطلب يصف الصيدلية التي طلبته، و`pharmacyCode` يكون null."),
        ("GET", "/orders/purchase/{orderNumber}", "3. Get one purchase order - جلب طلب شراء واحد",
         "**What it does:** shows one purchase order with each supplier's shipment and lines.\n\n"
         "**ماذا يفعل:** يعرض طلب شراء واحداً مع شحنة كل مورد وأصنافها."),
        ("POST", "/orders/purchase/ack", "3. Mark purchase orders as imported - تأكيد استيراد طلبات الشراء",
         "**What it does:** same as for sales orders: tells Dawa24 you saved these purchase orders.\n\n"
         "**ماذا يفعل:** مثل طلبات البيع: يخبر دواء24 أنك حفظت طلبات الشراء عندك."),
        ("GET", "/offers", "4. Find supplier offers by barcode - البحث عن عروض الموردين",
         "**What it does:** finds which suppliers sell a product, by barcode, with price and stock.\n\n"
         "**ماذا يفعل:** يبحث بالباركود عن الموردين الذين يبيعون الصنف مع السعر والرصيد."),
        ("POST", "/orders/purchase", "5. Create a purchase order - إنشاء طلب شراء",
         "**What it does:** places a purchase order on Dawa24 from your system.\n\n"
         "**ماذا يفعل:** ينشئ طلب شراء على دواء24 من نظامكم."),
    ]),
]

INTRO = """**Dawa24 ERP API — for Orgasoft**

**How to read this collection (no server needed):**
1. Click a request on the left.
2. Read the explanation on the right.
3. Open the **Body** tab to see exactly what to send.
4. Click **Examples** (top right) to see exactly what Dawa24 replies — for success and for each error.

**How to run it (when Dawa24 gives you a test server and a key):**
1. Choose the environment **Dawa24 ERP - test server** (top right).
2. Put the server address in `baseUrl` and your key in `apiKey`. Nothing else to change.
3. Run **01 Start here → Check my API key**. If it says `"status": "ok"`, you are connected.

**Rules that apply to every request:**
- Your key goes in the header `Authorization: Bearer <key>` (already set up here).
- Every reply has `"status": "ok"` or `"status": "error"`. Errors have a `code` and a message in Arabic and English.
- Prices are text with at most 2 decimals, like `"125.50"`. Discount and tax are percents.
- Up to 600 requests per minute. If you get error 429, wait the seconds in the `Retry-After` header.
- Keep the `requestId` from the reply when you report a problem — it lets us find your request.

---

**طريقة قراءة المجموعة (بدون سيرفر):** اضغط على أي طلب، اقرأ الشرح، افتح تبويب **Body** لترى ما يجب إرساله، واضغط **Examples** لترى رد دواء24 بالضبط في حالة النجاح وكل خطأ.

**طريقة التشغيل (عندما نرسل لكم سيرفر تجريبي ومفتاح):** اختر البيئة **Dawa24 ERP - test server**، ضع عنوان السيرفر في `baseUrl` والمفتاح في `apiKey`، ثم شغّل **Check my API key**. إذا ظهر `"status": "ok"` فالربط يعمل.

**قواعد عامة:** المفتاح يُرسل في `Authorization: Bearer`. كل رد فيه `status` ورسائل الخطأ بالعربي والإنجليزي. الأسعار بحد أقصى رقمين عشريين. احتفظوا بـ `requestId` عند الإبلاغ عن مشكلة."""


def deref(o):
    while isinstance(o, dict) and "$ref" in o:
        x = d
        for p in o["$ref"].lstrip("#/").split("/"):
            x = x[p]
        o = x
    return o


def examples_of(content):
    c = content.get("application/json") or {}
    if "example" in c:
        return [("", c["example"])]
    return [(k, deref(v).get("value")) for k, v in c.get("examples", {}).items()]


ERROR_LABEL = {"400": "Bad request", "401": "Wrong or missing key", "403": "Not allowed", "404": "Not found",
               "409": "Conflict", "413": "Too large", "415": "Wrong content type", "422": "Invalid data",
               "429": "Too many requests"}


def build(method, path, name, text):
    op = d["paths"][path][method.lower()]
    errors = ERRORS.get(path, [])
    if (method, path) == ("POST", "/orders/purchase"):
        errors = PURCHASE_ERRORS
    params = [deref(p) for p in d["paths"][path].get("parameters", []) + op.get("parameters", [])]
    pm_path, variables, query = path, [], []
    headers = [{"key": "Accept", "value": "application/json"}]
    for p in params:
        ex = p.get("example", deref(p.get("schema", {})).get("example"))
        if p["in"] == "path":
            pm_path = pm_path.replace("{%s}" % p["name"], ":" + p["name"])
            default = {"orderNumber": "10542"}.get(p["name"], "{{%s}}" % p["name"])
            variables.append({"key": p["name"], "value": default})
        elif p["in"] == "query":
            on = p["name"] == "state" or p.get("required", False)
            val = "new" if p["name"] == "state" and "orders" in path else ("" if ex is None else str(ex))
            if p["name"] == "state" and "products" in path:
                on, val = False, "failed"
            query.append({"key": p["name"], "value": val, "disabled": not on})
        elif p["in"] == "header" and p["name"].lower() == "idempotency-key":
            headers.append({"key": "Idempotency-Key", "value": "{{$guid}}",
                            "description": "Optional. Same key = same upload if you retry."})
    active = [q for q in query if not q["disabled"]]
    url = {"raw": "{{baseUrl}}/erp/v1" + pm_path + ("?" + "&".join(f"{q['key']}={q['value']}" for q in active) if active else ""),
           "host": ["{{baseUrl}}"], "path": ["erp", "v1"] + [s for s in pm_path.split("/") if s]}
    if query:
        url["query"] = query
    if variables:
        url["variable"] = variables
    req = {"method": method, "header": headers, "url": url, "description": text}
    if "requestBody" in op:
        body = examples_of(deref(op["requestBody"]).get("content", {}))
        headers.append({"key": "Content-Type", "value": "application/json"})
        req["body"] = {"mode": "raw", "raw": json.dumps(body[0][1] if body else {}, ensure_ascii=False, indent=2),
                       "options": {"raw": {"language": "json"}}}
    responses = []
    for code, resp in op.get("responses", {}).items():
        if not str(code).startswith("2") and errors and not (method, path, str(code)) == ("POST", "/orders/purchase", "409"):
            continue  # error examples are written below, one per real case
        resp = deref(resp)
        for exname, val in examples_of(resp.get("content", {})):
            label = "Success" if str(code).startswith("2") else ERROR_LABEL.get(str(code), "Error")
            if (method, path, str(code)) == ("POST", "/orders/purchase", "201"):
                label = "Created"
            elif (method, path, str(code)) == ("POST", "/orders/purchase", "200"):
                label = "Already created (same externalReference sent again)"
            elif str(code) == "409" and isinstance(val, dict) and "error" in val:
                label = "Price changed - correct price in details"
            responses.append({"name": f"{code} {label}", "originalRequest": {k: v for k, v in req.items() if k != "description"},
                              "status": label, "code": int(code), "_postman_previewlanguage": "json",
                              "header": [{"key": "Content-Type", "value": "application/json"}],
                              "body": json.dumps(val, ensure_ascii=False, indent=2)})
    for status, label, code, details in errors:
        if details and details.get("requiredScope") == "SCOPE":
            details = {"requiredScope": SCOPE.get(path, "erp.orders.read")}
        responses.append({"name": f"{status} {label}", "originalRequest": {k: v for k, v in req.items() if k != "description"},
                          "status": label, "code": status, "_postman_previewlanguage": "json",
                          "header": [{"key": "Content-Type", "value": "application/json"}],
                          "body": json.dumps(error_body(code, details), ensure_ascii=False, indent=2)})
    responses.sort(key=lambda r: (not str(r["code"]).startswith("2"), r["code"]))
    tests = ["pm.test('Reply status is ok', () => pm.expect(pm.response.code).to.be.within(200, 299));"]
    if path == "/products/sync":
        tests.append("pm.collectionVariables.set('syncId', pm.response.json().data.syncId);")
    return {"name": name, "request": req, "response": responses,
            "event": [{"listen": "test", "script": {"type": "text/javascript", "exec": tests}}]}


items, count, ex_count = [], 0, 0
for folder, note, reqs in PLAN:
    built = [build(m, p, n, t) for m, p, n, t in reqs]
    count += len(built)
    ex_count += sum(len(b["response"]) for b in built)
    items.append({"name": folder, "description": note, "item": built})

coll = {"info": {"_postman_id": str(uuid.uuid4()), "name": "Dawa24 ERP API for Orgasoft", "description": INTRO,
                 "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},
        "auth": {"type": "bearer", "bearer": [{"key": "token", "value": "{{apiKey}}", "type": "string"}]},
        "item": items, "variable": [{"key": "syncId", "value": ""}]}
env = {"id": str(uuid.uuid4()), "name": "Dawa24 ERP - test server", "_postman_variable_scope": "environment",
       "values": [{"key": "baseUrl", "value": "PUT-TEST-SERVER-ADDRESS-HERE", "enabled": True},
                  {"key": "apiKey", "value": "PUT-YOUR-API-KEY-HERE", "type": "secret", "enabled": True}]}
json.dump(coll, open(f"{outdir}/Dawa24-ERP-API.postman_collection.json", "w", encoding="utf-8"), ensure_ascii=False, indent=2)
json.dump(env, open(f"{outdir}/Dawa24-ERP-test.postman_environment.json", "w", encoding="utf-8"), ensure_ascii=False, indent=2)
print(f"{count} requests, {ex_count} saved replies")
