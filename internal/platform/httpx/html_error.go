package httpx

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
)

// renderHTMLError renders an isolated, mobile-responsive HTML error page for human browser visitors.
// It never leaks internal endpoints, query details, or driver error messages.
func renderHTMLError(w http.ResponseWriter, r *http.Request, status int, _, _, reqID string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	var (
		statusPillBg    = "#e0f2fe"
		statusPillClr   = "#0369a1"
		iconBg          = "rgba(2, 132, 199, 0.12)"
		iconSVG         = `<svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="#0284c7" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"></circle><line x1="21" y1="21" x2="16.65" y2="16.65"></line><line x1="8" y1="11" x2="14" y2="11"></line></svg>`
		titleAR         = "الصفحة غير موجودة"
		titleEN         = "Page Not Found"
		descAR          = "عذراً، لم نتمكن من العثور على الصفحة المطلوبة. قد يكون الرابط غير صحيح أو تم نقل الصفحة."
		btnSecondaryURL = "/"
		btnSecondaryLbl = "الصفحة الرئيسية"
	)

	switch {
	case status == http.StatusUnauthorized:
		statusPillBg = "#ede9fe"
		statusPillClr = "#6d28d9"
		iconBg = "rgba(109, 40, 217, 0.12)"
		iconSVG = `<svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="#6d28d9" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect><path d="M7 11V7a5 5 0 0 1 10 0v4"></path></svg>`
		titleAR = "تسجيل الدخول مطلوب"
		titleEN = "Authentication Required"
		descAR = "يرجى تسجيل الدخول بحسابك للمتابعة والوصول إلى هذا المحتوى."
		target := "/auth/login"
		if r != nil && r.URL.RequestURI() != "" && r.URL.Path != "/auth/login" {
			target += "?redirect=" + url.QueryEscape(r.URL.RequestURI())
		}
		btnSecondaryURL = target
		btnSecondaryLbl = "تسجيل الدخول"

	case status == http.StatusForbidden:
		statusPillBg = "#fef3c7"
		statusPillClr = "#b45309"
		iconBg = "rgba(217, 119, 6, 0.12)"
		iconSVG = `<svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="#b45309" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"></path><line x1="12" y1="8" x2="12" y2="12"></line><line x1="12" y1="16" x2="12.01" y2="16"></line></svg>`
		titleAR = "غير مصرح بالوصول"
		titleEN = "Access Forbidden"
		descAR = "عذراً، ليس لديك الصلاحيات الكافية للوصول إلى هذه الصفحة أو هذا الإجراء."

	case status == http.StatusNotFound:
		// Defaults set above for 404

	case status == http.StatusMethodNotAllowed:
		statusPillBg = "#fef3c7"
		statusPillClr = "#b45309"
		iconBg = "rgba(217, 119, 6, 0.12)"
		iconSVG = `<svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="#b45309" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"></line></svg>`
		titleAR = "إجراء غير متاح"
		titleEN = "Method Not Allowed"
		descAR = "طريقة إرسال الطلب غير مدعومة لهذا المسار."

	case status == http.StatusTooManyRequests:
		statusPillBg = "#ffedd5"
		statusPillClr = "#c2410c"
		iconBg = "rgba(194, 65, 12, 0.12)"
		iconSVG = `<svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="#c2410c" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>`
		titleAR = "طلبات كثيرة جداً"
		titleEN = "Too Many Requests"
		descAR = "تم إرسال عدد كبير من الطلبات في وقت وجيز. يرجى الانتظار بضع لحظات ثم إعادة المحاولة."

	case status >= 500:
		statusPillBg = "#ffe4e6"
		statusPillClr = "#be123c"
		iconBg = "rgba(225, 29, 72, 0.12)"
		iconSVG = `<svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="#e11d48" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect><rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect><line x1="6" y1="6" x2="6.01" y2="6"></line><line x1="6" y1="18" x2="6.01" y2="18"></line></svg>`
		titleAR = "حدث خطأ غير متوقع"
		titleEN = "Server Error"
		descAR = "نعتذر عن هذا الخطأ المؤقت. تم تسجيل المشكلة وجاري العمل على معالجتها."

	default:
		statusPillBg = "#f1f5f9"
		statusPillClr = "#475569"
		iconBg = "rgba(71, 85, 105, 0.12)"
		iconSVG = `<svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="#475569" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="8" x2="12" y2="12"></line><line x1="12" y1="16" x2="12.01" y2="16"></line></svg>`
		titleAR = "تنبيه"
		titleEN = "Notice"
		descAR = "تعذر إكمال طلبك في الوقت الحالي. يرجى إعادة المحاولة."
	}

	safeReqID := html.EscapeString(reqID)
	reqIDBlock := ""
	if safeReqID != "" {
		reqIDBlock = fmt.Sprintf(`<div class="ref-badge" onclick="copyRef('%s', this)" title="انقر لنسخ رمز المرجع"><span class="ref-lbl">رمز المرجع:</span><code class="ref-id">%s</code><span class="ref-copy">نسخ</span></div>`, safeReqID, safeReqID)
	}

	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="ar" dir="rtl">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=5">
  <title>%s - دواء 24</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: 'Readex Pro', -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
      background: #f8fafc;
      color: #0f172a;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
      padding: 20px 16px;
      line-height: 1.5;
    }
    .card {
      background: #ffffff;
      border: 1px solid #e2e8f0;
      border-radius: 24px;
      box-shadow: 0 20px 40px -15px rgba(0, 0, 0, 0.07);
      max-width: 440px;
      width: 100%%;
      padding: 32px 24px;
      text-align: center;
    }
    .brand {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      margin-bottom: 20px;
      font-weight: 800;
      font-size: 16px;
      color: #0f172a;
      text-decoration: none;
    }
    .brand-num { color: #0284c7; }
    .icon-wrap {
      width: 64px;
      height: 64px;
      border-radius: 20px;
      background: %s;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      margin-bottom: 16px;
    }
    .status-pill {
      display: inline-block;
      font-size: 12px;
      font-weight: 800;
      padding: 3px 12px;
      border-radius: 9999px;
      margin-bottom: 12px;
      letter-spacing: 0.5px;
      font-family: monospace;
      background: %s;
      color: %s;
    }
    h1 {
      font-size: 20px;
      font-weight: 800;
      color: #0f172a;
      margin-bottom: 4px;
    }
    .subtitle {
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 1px;
      color: #64748b;
      margin-bottom: 16px;
    }
    p {
      font-size: 14px;
      line-height: 1.6;
      color: #475569;
      margin-bottom: 24px;
    }
    .actions {
      display: flex;
      flex-direction: column;
      gap: 10px;
    }
    .btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 8px;
      font-size: 14px;
      font-weight: 700;
      border-radius: 12px;
      min-height: 46px;
      padding: 10px 20px;
      cursor: pointer;
      text-decoration: none;
      border: 1px solid transparent;
      transition: background 0.15s ease, transform 0.1s ease;
    }
    .btn:active { transform: scale(0.98); }
    .btn-primary {
      background: #0284c7;
      color: #ffffff;
      border-color: #0284c7;
    }
    .btn-primary:hover { background: #0369a1; }
    .btn-secondary {
      background: #f1f5f9;
      color: #334155;
      border-color: #cbd5e1;
    }
    .btn-secondary:hover { background: #e2e8f0; }
    .ref-badge {
      margin-top: 24px;
      padding: 8px 12px;
      background: #f8fafc;
      border: 1px dashed #cbd5e1;
      border-radius: 10px;
      font-size: 11px;
      color: #64748b;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 8px;
      cursor: pointer;
      user-select: none;
      width: 100%%;
    }
    .ref-id { font-family: monospace; font-weight: 700; color: #0f172a; direction: ltr; }
    .ref-copy { color: #0284c7; font-weight: 600; font-size: 10px; }
    .footer {
      margin-top: 20px;
      font-size: 11px;
      color: #94a3b8;
    }
    @media (prefers-color-scheme: dark) {
      body { background: #09131a; color: #f1f5f9; }
      .card { background: #11222e; border-color: #1e3a4f; box-shadow: 0 20px 40px -15px rgba(0,0,0,0.5); }
      .brand { color: #f1f5f9; }
      h1 { color: #f8fafc; }
      p { color: #94a3b8; }
      .btn-secondary { background: #1e293b; color: #cbd5e1; border-color: #334155; }
      .btn-secondary:hover { background: #334155; }
      .ref-badge { background: #09131a; border-color: #334155; color: #94a3b8; }
      .ref-id { color: #e2e8f0; }
    }
  </style>
</head>
<body>
  <div class="card">
    <a href="/" class="brand">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="#0284c7" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z"/></svg>
      <span>دواء <span class="brand-num">24</span></span>
    </a>
    <div class="icon-wrap">%s</div>
    <div><span class="status-pill">%d</span></div>
    <h1>%s</h1>
    <div class="subtitle">%s</div>
    <p>%s</p>
    <div class="actions">
      <button type="button" onclick="if(window.history.length>1){window.history.back();}else{window.location.href='/';}" class="btn btn-primary">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
        <span>العودة للصفحة السابقة</span>
      </button>
      <a href="%s" class="btn btn-secondary">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"></path><polyline points="9 22 9 12 15 12 15 22"></polyline></svg>
        <span>%s</span>
      </a>
    </div>
    %s
  </div>
  <div class="footer">دواء 24 — سوق الدواء والمستلزمات الطبية</div>
  <script>
    function copyRef(t, el) {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(t).then(function() {
          var c = el.querySelector('.ref-copy');
          if (c) {
            c.textContent = 'تم النسخ ✓';
            setTimeout(function() { c.textContent = 'نسخ'; }, 2000);
          }
        });
      }
    }
  </script>
</body>
</html>`,
		html.EscapeString(titleAR),
		iconBg,
		statusPillBg,
		statusPillClr,
		iconSVG,
		status,
		html.EscapeString(titleAR),
		html.EscapeString(titleEN),
		html.EscapeString(descAR),
		html.EscapeString(btnSecondaryURL),
		html.EscapeString(btnSecondaryLbl),
		reqIDBlock,
	)
}
