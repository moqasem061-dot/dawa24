package mailer

import (
	"fmt"
	"time"
)

// RenderPasswordChangedEmail is the security alert sent after a user changes
// their password. Inline styles are deliberate: this is an email body, which
// mail clients render without stylesheets, and the site's CSP does not apply.
func RenderPasswordChangedEmail(changedAt time.Time, lang string) (subject, html, text string) {
	subject = "تنبيه أمان: تم تغيير كلمة المرور لحسابك في دوا 24 | Security Alert"
	if lang == "en" {
		subject = "Security Alert: Password Changed for your Dawa24 account"
	}

	nowStr := changedAt.Format("2006-01-02 15:04:05 MST")
	text = fmt.Sprintf(
		"مرحباً،\n\nنود إعلامك بأنه تم تغيير كلمة المرور الخاصة بحسابك في منصة دوا 24 بنجاح في: %s.\n\nإذا قمت أنت بهذا الإجراء، فلا داعي لاتخاذ أي خطوة.\n\nإذا لم تكن أنت من قام بتغيير كلمة المرور، يرجى التوجه فوراً لرابط استعادة كلمة المرور أو التواصل مع إدارة المنصة لحماية حسابك.\n\nفريق أمان دوا 24\nsupport@dawa24.com",
		nowStr,
	)

	html = fmt.Sprintf(`<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f1f5f9; padding: 24px 0; margin: 0;">
  <table width="100%%" border="0" cellspacing="0" cellpadding="0">
    <tr><td align="center">
      <table width="600" style="background: #ffffff; border-radius: 12px; overflow: hidden; box-shadow: 0 4px 6px rgba(0,0,0,0.05);">
        <tr>
          <td style="background: #1e3a8a; padding: 24px; text-align: center;">
            <h1 style="color: #ffffff; margin: 0; font-size: 22px;">دوا 24 | Dawa24</h1>
          </td>
        </tr>
        <tr>
          <td style="padding: 32px 24px; color: #1e293b; line-height: 1.6;">
            <div style="background: #fef2f2; border-right: 4px solid #ef4444; padding: 12px 16px; border-radius: 6px; margin-bottom: 20px;">
              <strong style="color: #991b1b; font-size: 15px;">🔒 تنبيه أمني بشأن حسابك</strong>
            </div>
            <p style="font-size: 15px; margin: 0 0 16px;">مرحباً،</p>
            <p style="font-size: 14px; margin: 0 0 16px;">
              نود إحاطتك بأنه تم تحديث وتغيير كلمة المرور الخاصة بحسابك في منصة دوا 24 بنجاح في:
              <strong style="font-family: monospace;">%s</strong>
            </p>
            <p style="font-size: 14px; margin: 0 0 20px; color: #475569;">
              إذا قمت أنت بهذا الإجراء، يمكنك تجاهل هذه الرسالة بأمان.
            </p>
            <div style="background: #f8fafc; border: 1px solid #e2e8f0; padding: 16px; border-radius: 8px; font-size: 13px; color: #64748b;">
              <strong>هل لم تكن أنت؟</strong><br>
              إذا لم تكن أنت من قام بهذا التعديل، يرجى إعادة تعيين كلمة المرور فوراً عبر المنصة أو التواصل مع فريق الدعم الفني:
              <a href="mailto:support@dawa24.com" style="color: #2563eb;">support@dawa24.com</a>
            </div>
          </td>
        </tr>
        <tr>
          <td style="background: #f8fafc; padding: 16px 24px; text-align: center; font-size: 12px; color: #94a3b8; border-top: 1px solid #e2e8f0;">
            منظومة دوا 24 الرقمية للتوريدات الدوائية &copy; 2026
          </td>
        </tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, nowStr)

	return subject, html, text
}
