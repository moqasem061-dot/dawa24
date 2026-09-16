package assistant

import (
	"context"
	"errors"

	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// One vocabulary for everything that can go wrong.
//
// The rule this file exists to enforce: the user gets a sentence in Arabic
// telling them what happened and whether to retry; the log gets the detail.
// Nothing from a Gateway error body, a database error, or a stack ever reaches
// the browser — those name models, hosts and columns, and the browser is the
// last place any of that belongs.

// Code is a stable machine-readable failure name. The UI branches on it; the
// text beside it is for people.
type Code string

const (
	CodeGatewayUnavailable Code = "gateway_unavailable"
	CodeGatewayQuota       Code = "gateway_quota"
	CodeGatewayDisabled    Code = "gateway_disabled"
	CodeToolDenied         Code = "tool_denied"
	CodeToolFailed         Code = "tool_failed"
	CodeAttachmentRejected Code = "attachment_rejected"
	CodeAttachmentTooLarge Code = "attachment_too_large"
	// Two attachment failures that used to arrive as "unsupported file type",
	// which is advice nobody can act on. A HEIC photograph looks like an
	// ordinary picture in the camera roll, and a storage outage is not the
	// user's file being wrong.
	CodeAttachmentHEIC      Code = "attachment_heic"
	CodeAttachmentStore     Code = "attachment_store_unavailable"
	CodeTranscribeUnavail   Code = "transcribe_unavailable"
	CodeTranscribeFailed    Code = "transcribe_failed"
	CodeStreamInterrupted   Code = "stream_interrupted"
	CodeTurnTimeout         Code = "turn_timeout"
	CodeRateLimited         Code = "rate_limited"
	CodeForbidden           Code = "forbidden"
	CodeNotFound            Code = "not_found"
	CodeInvalidRequest      Code = "invalid_request"
	CodeInternal            Code = "internal"
	CodeConversationExpired Code = "conversation_expired"

	// Two failures that used to collapse into "unavailable" and told the user
	// to try again — advice that is wrong for both. A revoked key and a
	// malformed request do not fix themselves with time.
	CodeGatewayUnauthorized Code = "gateway_unauthorized"
	CodeGatewayRejected     Code = "gateway_rejected"
)

// Failure is a user-facing error.
type Failure struct {
	Code      Code   `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

var messages = map[Code]Failure{
	CodeGatewayUnavailable: {CodeGatewayUnavailable,
		"خدمة المساعد الذكي غير متاحة حالياً. حاول مرة أخرى بعد قليل.", true},
	CodeGatewayQuota: {CodeGatewayQuota,
		"انتهت حصة الذكاء الاصطناعي المتاحة لمنشأتك لهذه الفترة. راجع صفحة الاشتراك.", false},
	CodeGatewayUnauthorized: {CodeGatewayUnauthorized,
		"مفتاح الذكاء الاصطناعي لهذه المنشأة غير صالح. تواصل مع إدارة المنصة.", false},
	CodeGatewayRejected: {CodeGatewayRejected,
		"رفضت خدمة الذكاء الاصطناعي هذا الطلب. جرّب صياغة أبسط أو مرفقاً أصغر.", false},
	CodeGatewayDisabled: {CodeGatewayDisabled,
		"المساعد الذكي غير مفعّل على هذا الحساب. تواصل مع إدارة المنصة.", false},
	CodeToolDenied: {CodeToolDenied,
		"هذه البيانات خارج نطاق صلاحياتك.", false},
	CodeToolFailed: {CodeToolFailed,
		"تعذّر قراءة البيانات المطلوبة. حاول مرة أخرى.", true},
	CodeAttachmentRejected: {CodeAttachmentRejected,
		"نوع الملف غير مدعوم. المدعوم: الصور و PDF و Word و Excel والنصوص والملفات الصوتية.", false},
	CodeAttachmentTooLarge: {CodeAttachmentTooLarge,
		"حجم الملف أكبر من الحد المسموح (١٠ ميجابايت).", false},
	CodeAttachmentHEIC: {CodeAttachmentHEIC,
		"صيغة HEIC غير مدعومة. من إعدادات الكاميرا في الآيفون اختر «الأكثر توافقاً»، " +
			"أو أرسل الصورة كلقطة شاشة بصيغة JPEG.", false},
	CodeAttachmentStore: {CodeAttachmentStore,
		"تعذّر حفظ الملف على الخادم. حاول مرة أخرى، وإن تكرر الأمر أبلغ إدارة المنصة.", true},
	CodeTranscribeUnavail: {CodeTranscribeUnavail,
		"خدمة تفريغ الصوت غير مفعّلة حالياً. اكتب سؤالك نصاً.", false},
	CodeTranscribeFailed: {CodeTranscribeFailed,
		"تعذّر تحويل التسجيل إلى نص. أعد المحاولة أو اكتب سؤالك.", true},
	CodeStreamInterrupted: {CodeStreamInterrupted,
		"انقطع الاتصال أثناء كتابة الإجابة. الإجابة محفوظة، أعد فتح المحادثة.", true},
	CodeTurnTimeout: {CodeTurnTimeout,
		"استغرقت الإجابة وقتاً أطول من المسموح. جرّب سؤالاً أكثر تحديداً.", true},
	CodeRateLimited: {CodeRateLimited,
		"تم تجاوز الحد المسموح من الطلبات (429). انتظر دقيقة ثم أعد المحاولة.", true},
	CodeForbidden: {CodeForbidden,
		"لا تملك صلاحية استخدام المساعد الذكي. اطلب من مالك الحساب تفعيلها لدورك.", false},
	CodeNotFound: {CodeNotFound,
		"المحادثة غير موجودة.", false},
	CodeInvalidRequest: {CodeInvalidRequest,
		"الطلب غير صالح.", false},
	CodeInternal: {CodeInternal,
		"حدث خطأ غير متوقع. تم تسجيله وسنراجعه.", true},
	CodeConversationExpired: {CodeConversationExpired,
		"انتهت صلاحية هذه المحادثة وحُذفت تلقائياً بعد ستة أشهر.", false},
}

// Fail returns the user-facing failure for a code.
func Fail(code Code) Failure {
	if f, ok := messages[code]; ok {
		return f
	}
	return messages[CodeInternal]
}

// ClassifyGateway maps a Gateway transport error onto a user-facing code.
//
// The error itself is never shown. gateway.ErrBadRequest, for instance, often
// carries the upstream's own complaint about a model name — accurate,
// unactionable, and a provider leak.
func ClassifyGateway(err error) Code {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, gateway.ErrDisabled):
		return CodeGatewayDisabled
	case errors.Is(err, gateway.ErrQuotaExceeded):
		return CodeGatewayQuota
	case errors.Is(err, gateway.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return CodeTurnTimeout
	case errors.Is(err, gateway.ErrRateLimited):
		return CodeRateLimited
	case errors.Is(err, gateway.ErrUnauthorized):
		return CodeGatewayUnauthorized
	case errors.Is(err, gateway.ErrBadRequest):
		return CodeGatewayRejected
	case errors.Is(err, gateway.ErrCircuitOpen):
		return CodeGatewayUnavailable
	default:
		return CodeGatewayUnavailable
	}
}
