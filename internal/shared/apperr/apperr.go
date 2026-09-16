// Package apperr defines the closed set of domain error kinds.
//
// Domain and service code returns these; only the HTTP layer translates them to
// status codes and user-facing messages. That keeps modules free of net/http and
// means an error's meaning is decided once, by the code that understands the
// business rule, rather than guessed at by a handler.
package apperr

import (
	"errors"
	"fmt"
)

// Kind classifies an error for transport mapping and metrics.
type Kind string

const (
	KindValidation   Kind = "validation"   // caller sent something invalid
	KindNotFound     Kind = "not_found"    // the entity does not exist (or is not visible to this tenant)
	KindConflict     Kind = "conflict"     // unique violation, concurrent edit, state machine refusal
	KindUnauthorized Kind = "unauthorized" // not authenticated
	KindForbidden    Kind = "forbidden"    // authenticated but not permitted
	KindRateLimited  Kind = "rate_limited"
	KindUnavailable  Kind = "unavailable" // a dependency is down; retry may succeed
	KindInternal     Kind = "internal"    // our bug
)

// Error carries a kind, a stable machine code, a human message, optional field
// errors, and a wrapped cause.
//
// Msg is shown to users and must be safe to display; it never contains SQL,
// stack traces, or internal identifiers. Detail is for logs only.
type Error struct {
	Kind   Kind
	Code   string            // stable identifier, e.g. "order.already_confirmed"
	Msg    string            // user-safe, English; the UI layer localises by Code
	Fields map[string]string // field name -> validation message
	Detail string            // operator-only context, never rendered
	cause  error
}

func (e *Error) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s [%s]: %s (%s)", e.Kind, e.Code, e.Msg, e.Detail)
	}
	return fmt.Sprintf("%s [%s]: %s", e.Kind, e.Code, e.Msg)
}

func (e *Error) Unwrap() error { return e.cause }

// Wrap attaches a cause without changing the classification.
func (e *Error) Wrap(cause error) *Error {
	clone := *e
	clone.cause = cause
	return &clone
}

// WithDetail attaches operator-only context.
func (e *Error) WithDetail(format string, args ...any) *Error {
	clone := *e
	clone.Detail = fmt.Sprintf(format, args...)
	return &clone
}

func New(kind Kind, code, msg string) *Error {
	return &Error{Kind: kind, Code: code, Msg: msg}
}

func Validation(code, msg string, fields map[string]string) *Error {
	return &Error{Kind: KindValidation, Code: code, Msg: msg, Fields: fields}
}

func NotFound(entity string) *Error {
	return &Error{
		Kind: KindNotFound,
		Code: entity + ".not_found",
		Msg:  fmt.Sprintf("The requested %s was not found.", entity),
	}
}

func Conflict(code, msg string) *Error   { return New(KindConflict, code, msg) }
func Forbidden(code, msg string) *Error  { return New(KindForbidden, code, msg) }
func BadRequest(code, msg string) *Error { return Validation(code, msg, nil) }

func Unauthorized() *Error {
	return New(KindUnauthorized, "auth.required", "Authentication is required.")
}

// Internal wraps an unexpected failure. The message is deliberately generic:
// whatever went wrong belongs in the log, not on the user's screen.
func Internal(cause error) *Error {
	return &Error{
		Kind:  KindInternal,
		Code:  "internal",
		Msg:   "Something went wrong on our side. Please try again.",
		cause: cause,
	}
}

// Unavailable marks a dependency failure that a retry might survive.
func Unavailable(dependency string, cause error) *Error {
	return &Error{
		Kind:  KindUnavailable,
		Code:  dependency + ".unavailable",
		Msg:   "A required service is temporarily unavailable. Please try again shortly.",
		cause: cause,
	}
}

// KindOf extracts the Kind from anywhere in the error chain, defaulting to
// KindInternal so that an unclassified error is treated as our fault rather than
// leaking as a 400.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindInternal
}

// As is a typed convenience over errors.As.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

// LocalizedMsg returns the user-facing message localized for the given language (e.g. "ar" or "en").
func (e *Error) LocalizedMsg(lang string) string {
	if lang == "ar" {
		if arMsg, ok := arabicMessages[e.Code]; ok {
			return arMsg
		}
		// If e.Msg already contains Arabic letters, return e.Msg directly
		for _, r := range e.Msg {
			if r >= 0x0600 && r <= 0x06FF {
				return e.Msg
			}
		}
		if arMsg, ok := arabicMessages[string(e.Kind)]; ok {
			return arMsg
		}
	}
	if e.Msg != "" {
		return e.Msg
	}
	return "An error occurred."
}

var arabicMessages = map[string]string{
	"auth.required":                  "تسجيل الدخول مطلوب للمتابعة.",
	"auth.invalid":                   "بيانات الدخول غير صحيحة.",
	"auth.locked":                    "تم قفل الحساب مؤقتاً بسبب المحاولات المتكررة.",
	"user.email_exists":              "البريد الإلكتروني مسجل مسبقاً في النظام. يرجى تسجيل الدخول أو استخدام بريد آخر.",
	"org.commercial_register_exists": "رقم السجل التجاري مسجل مسبقاً لمنشأة أخرى.",
	"org.legal_name_required":        "الاسم القانوني للمنشأة مطلوب.",
	"org.cr_required":                "رقم السجل التجاري أو الرقم الضريبي مطلوب.",
	"org.license_required":           "رقم ترخيص المؤسسة الصيدلية مطلوب.",
	"org.tax_required":               "الرقم الضريبي مطلوب للموردين والشركات.",
	"org.type_invalid":               "يرجى تحديد نوع الحساب بشكل صحيح.",
	"email.invalid":                  "يرجى إدخال عنوان بريد إلكتروني صالح.",
	"password.too_short":             "كلمة المرور يجب أن لا تقل عن 8 أحرف.",
	"password.weak":                  "كلمة المرور يجب أن تحتوي على أحرف كبيرة وصغيرة وأرقام ورموز خاصة.",
	"branch.org_required":            "يجب تحديد المنشأة التابع لها الفرع.",
	"branch.code_required":           "كود الفرع مطلوب.",
	"branch.name_required":           "اسم الفرع مطلوب.",
	"branch.not_found":               "الفرع المطلوب غير موجود.",
	"branch.cannot_delete_main":      "لا يمكن حذف الفرع الرئيسي للمنشأة.",
	"variant.discount_exceeds_price": "نسبة الخصم لا يمكن أن تتجاوز 100%.",
	"variant.name_required":          "اسم الصنف مطلوب باللغة العربية أو الإنجليزية.",
	"variant.price_negative":         "سعر الصنف لا يمكن أن يكون سالباً.",
	"variant.quota_negative":         "الحصة المحددة للطلب لا يمكن أن تكون سالبة.",
	"variant.quota_too_large":        "الحصة المحددة للطلب تتجاوز الحد الأقصى المسموح.",
	"variant.id_required":           "معرف الصنف مطلوب.",
	"tenant.required":                "يرجى تحديد المنشأة لمتابعة العملية.",
	"validation":                     "بيانات الطلب غير صالحة.",
	"not_found":                      "العنصر المطلوب غير موجود.",
	"conflict":                       "تعارض في بيانات العملية.",
	"unauthorized":                   "غير مصرح بهذا الإجراء.",
	"forbidden":                      "ليس لديك صلاحية لتنفيذ هذا الإجراء.",
	"rate_limited":                   "تم تجاوز الحد المسموح للطلبات. يرجى المحاولة لاحقاً.",
	"unavailable":                    "الخدمة غير متوفرة مؤقتاً. يرجى إعادة المحاولة.",
	"internal":                       "حدث خطأ في النظام. يرجى المحاولة مرة أخرى.",
}
