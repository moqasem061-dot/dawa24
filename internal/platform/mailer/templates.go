package mailer

import (
	"fmt"
	"strings"
)

// RenderOTPEmail produces the subject, HTML body, and plaintext body for a 6-digit verification code.
func RenderOTPEmail(code, purpose, lang string) (subject, html, text string) {
	isAr := lang != "en"

	var titleAr, titleEn, purposeAr, purposeEn string
	switch purpose {
	case "password_reset":
		subject = "رمز استعادة كلمة المرور | Password Reset Code - Dawa24"
		titleAr = "استعادة كلمة المرور"
		titleEn = "Password Reset Request"
		purposeAr = "لقد تلقينا طلباً لإعادة تعيين كلمة المرور الخاصة بحسابك في منصة دوا 24."
		purposeEn = "We received a request to reset your password on Dawa24."
	case "registration":
		subject = "رمز تأكيد البريد الإلكتروني | Email Verification Code - Dawa24"
		titleAr = "تأكيد البريد الإلكتروني"
		titleEn = "Verify Your Email Address"
		purposeAr = "شكراً لانضمامك إلى منصة دوا 24 للتوريد الدوائي. يرجى تأكيد بريدك الإلكتروني لإتمام التسجيل."
		purposeEn = "Thank you for joining Dawa24 Pharmaceutical B2B Platform. Please verify your email to complete registration."
	default:
		subject = "رمز التحقق من الحساب | Account Verification Code - Dawa24"
		titleAr = "رمز التحقق والأمان"
		titleEn = "Verification Code"
		purposeAr = "رمز التحقق الخاص بحسابك في منصة دوا 24:"
		purposeEn = "Your verification code for Dawa24:"
	}

	html = fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s" dir="%s">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f1f5f9; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
  <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background-color: #f1f5f9; padding: 32px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="100%%" style="max-width: 560px; background-color: #ffffff; border-radius: 14px; overflow: hidden; box-shadow: 0 4px 6px -1px rgba(0,0,0,0.05), 0 2px 4px -2px rgba(0,0,0,0.05); border: 1px solid #e2e8f0;">
          <!-- Brand Header -->
          <tr>
            <td style="background: linear-gradient(135deg, #0284c7 0%%, #0369a1 100%%); padding: 26px 32px; text-align: center;">
              <table role="presentation" cellspacing="0" cellpadding="0" style="margin: 0 auto;">
                <tr>
                  <td style="vertical-align: middle; padding-left: 10px;">
                    <div style="width: 36px; height: 36px; background: rgba(255,255,255,0.2); border-radius: 10px; text-align: center; line-height: 36px; font-size: 20px;">💊</div>
                  </td>
                  <td style="vertical-align: middle; text-align: right;">
                    <h1 style="margin: 0; color: #ffffff; font-size: 24px; font-weight: 800; letter-spacing: -0.5px;">Dawa24 | دوا 24</h1>
                    <p style="margin: 2px 0 0 0; color: #e0f2fe; font-size: 13px; font-weight: 500;">سوق الأدوية والمستلزمات الطبية المعتمد</p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          <!-- Main Content -->
          <tr>
            <td style="padding: 36px 32px; direction: rtl; text-align: right;">
              <h2 style="margin: 0 0 12px 0; color: #0f172a; font-size: 20px; font-weight: 700;">%s</h2>
              <p style="margin: 0 0 24px 0; color: #475569; font-size: 15px; line-height: 1.6;">%s</p>

              <!-- OTP Code Card -->
              <div style="background-color: #f0f9ff; border: 2px dashed #bae6fd; border-radius: 12px; padding: 24px; text-align: center; margin: 0 0 24px 0;">
                <span style="display: block; font-size: 12px; font-weight: 700; color: #0369a1; text-transform: uppercase; margin-bottom: 8px; letter-spacing: 1px;">رمز التحقق (OTP)</span>
                <span style="font-family: 'SF Mono', Consolas, 'Liberation Mono', Menlo, Courier, monospace; font-size: 36px; font-weight: 800; color: #0284c7; letter-spacing: 8px; display: inline-block;">%s</span>
                <span style="display: block; font-size: 13px; color: #64748b; margin-top: 10px;">الرمز صالح لمدة 15 دقيقة فقط</span>
              </div>

              <!-- Security Notice -->
              <div style="background-color: #fffbeb; border-right: 4px solid #f59e0b; padding: 14px 16px; border-radius: 6px; margin: 0 0 28px 0;">
                <p style="margin: 0; color: #92400e; font-size: 13px; line-height: 1.5; font-weight: 500;">
                  ⚠️ <strong>تنبيه أمان:</strong> لا تشارك هذا الرمز مع أي شخص إطلاقاً. فريق عمل منصة دوا 24 لن يطلب منك هذا الرمز تحت أي ظرف.
                </p>
              </div>

              <hr style="border: 0; border-top: 1px solid #e2e8f0; margin: 28px 0 20px 0;">

              <!-- English Bilingual Section -->
              <div style="direction: ltr; text-align: left;">
                <h3 style="margin: 0 0 8px 0; color: #334155; font-size: 16px; font-weight: 600;">%s</h3>
                <p style="margin: 0 0 12px 0; color: #64748b; font-size: 13px; line-height: 1.5;">%s</p>
                <p style="margin: 0; color: #94a3b8; font-size: 12px;">This code expires in 15 minutes. If you did not request this, you can safely ignore this email.</p>
              </div>
            </td>
          </tr>

          <!-- Footer -->
          <tr>
            <td style="background-color: #f8fafc; border-top: 1px solid #e2e8f0; padding: 20px 32px; text-align: center;">
              <p style="margin: 0 0 6px 0; color: #64748b; font-size: 12px;">
                منصة دوا 24 — سوق الأدوية والمستلزمات الصيدلانية لمصر والشرق الأوسط
              </p>
              <p style="margin: 0; color: #94a3b8; font-size: 11px;">
                © 2026 Dawa24 Store. All rights reserved. &bull; <a href="https://dawa24.com" style="color: #0284c7; text-decoration: none;">dawa24.com</a>
              </p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`,
		map[bool]string{true: "ar", false: "en"}[isAr],
		map[bool]string{true: "rtl", false: "ltr"}[isAr],
		subject,
		titleAr,
		purposeAr,
		code,
		titleEn,
		purposeEn,
	)

	text = fmt.Sprintf(`Dawa24 | دوا 24
%s

رمز التحقق الخاص بك هو: %s
صالح لمدة 15 دقيقة.

Your verification code is: %s
Valid for 15 minutes.

تنبيه أمان: لا تشارك هذا الرمز مع أي شخص إطلاقاً.
Never share this code with anyone.

منصة دوا 24 - سوق الأدوية والمستلزمات الصيدلانية
https://dawa24.com
`, titleAr, code, code)

	return subject, html, text
}

// RenderNotificationEmail renders a user-targeted platform notification email.
func RenderNotificationEmail(title, body, actionURL, lang string) (subject, html, text string) {
	subject = fmt.Sprintf("[دوا 24] %s", title)
	if strings.TrimSpace(actionURL) == "" {
		actionURL = "https://dawa24.com/notifications"
	}

	html = fmt.Sprintf(`<!DOCTYPE html>
<html lang="ar" dir="rtl">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f8fafc; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
  <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background-color: #f8fafc; padding: 32px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="100%%" style="max-width: 560px; background-color: #ffffff; border-radius: 14px; overflow: hidden; box-shadow: 0 4px 6px -1px rgba(0,0,0,0.05); border: 1px solid #e2e8f0;">
          <!-- Header -->
          <tr>
            <td style="background: linear-gradient(135deg, #0284c7 0%%, #0369a1 100%%); padding: 24px 32px; text-align: center;">
              <table role="presentation" cellspacing="0" cellpadding="0" style="margin: 0 auto;">
                <tr>
                  <td style="vertical-align: middle; padding-left: 10px;">
                    <div style="width: 34px; height: 34px; background: rgba(255,255,255,0.2); border-radius: 8px; text-align: center; line-height: 34px; font-size: 18px;">💊</div>
                  </td>
                  <td style="vertical-align: middle; text-align: right;">
                    <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 800;">Dawa24 | دوا 24</h1>
                    <p style="margin: 2px 0 0 0; color: #e0f2fe; font-size: 13px;">سوق الأدوية والمستلزمات الطبية المعتمد</p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          <!-- Content -->
          <tr>
            <td style="padding: 32px; direction: rtl; text-align: right;">
              <div style="display: inline-block; background-color: #f0f9ff; color: #0369a1; font-size: 12px; font-weight: 700; padding: 4px 10px; border-radius: 6px; margin-bottom: 12px;">إشعار من المنصة</div>
              <h2 style="margin: 0 0 16px 0; color: #0f172a; font-size: 19px; font-weight: 700; line-height: 1.4;">%s</h2>
              <p style="margin: 0 0 28px 0; color: #334155; font-size: 15px; line-height: 1.7; white-space: pre-line;">%s</p>

              <!-- CTA Button -->
              <div style="text-align: center; margin: 32px 0 16px 0;">
                <a href="%s" style="display: inline-block; background-color: #0284c7; color: #ffffff; font-size: 15px; font-weight: 700; text-decoration: none; padding: 12px 28px; border-radius: 8px; box-shadow: 0 2px 4px rgba(2,132,199,0.25);">عرض التفاصيل في المنصة &larr;</a>
              </div>
            </td>
          </tr>

          <!-- Footer with Unsubscribe / Preferences Link -->
          <tr>
            <td style="background-color: #f8fafc; border-top: 1px solid #e2e8f0; padding: 20px 32px; text-align: center; direction: rtl;">
              <p style="margin: 0 0 6px 0; color: #64748b; font-size: 12px;">
                تصلك هذه الرسالة بناءً على إعدادات التنبيهات الخاصة بحسابك في منصة دوا 24.
              </p>
              <p style="margin: 0; color: #94a3b8; font-size: 11px;">
                لإدارة تفضيلات الإشعارات، يمكنك زيارة <a href="https://dawa24.com/settings#preferences" style="color: #0284c7; text-decoration: underline;">إعدادات الحساب والتفضيلات</a>. &bull; <a href="https://dawa24.com" style="color: #0284c7; text-decoration: none;">dawa24.com</a>
              </p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`,
		subject,
		title,
		body,
		actionURL,
	)

	text = fmt.Sprintf(`Dawa24 | دوا 24
%s

%s

عرض التفاصيل في المنصة:
%s

https://dawa24.com
`, title, body, actionURL)

	return subject, html, text
}

// RenderApprovalEmail renders an official celebration and account activation email.
func RenderApprovalEmail(userName, title, message, actionURL, lang string) (subject, html, text string) {
	if strings.TrimSpace(title) == "" {
		title = "تم اعتماد حسابك بنجاح"
	}
	subject = fmt.Sprintf("تهانينا! %s | Dawa24", title)
	if strings.TrimSpace(actionURL) == "" {
		actionURL = "https://dawa24.com/auth/login"
	}
	if strings.TrimSpace(userName) == "" {
		userName = "شريكنا العزيز"
	}

	html = fmt.Sprintf(`<!DOCTYPE html>
<html lang="ar" dir="rtl">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f8fafc; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
  <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background-color: #f8fafc; padding: 32px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="100%%" style="max-width: 560px; background-color: #ffffff; border-radius: 14px; overflow: hidden; box-shadow: 0 4px 6px -1px rgba(0,0,0,0.05); border: 1px solid #e2e8f0;">
          <!-- Celebration Header -->
          <tr>
            <td style="background: linear-gradient(135deg, #0284c7 0%%, #0369a1 100%%); padding: 28px 32px; text-align: center;">
              <table role="presentation" cellspacing="0" cellpadding="0" style="margin: 0 auto;">
                <tr>
                  <td style="vertical-align: middle; padding-left: 10px;">
                    <div style="width: 40px; height: 40px; background: rgba(255,255,255,0.2); border-radius: 10px; text-align: center; line-height: 40px; font-size: 22px;">🎉</div>
                  </td>
                  <td style="vertical-align: middle; text-align: right;">
                    <h1 style="margin: 0; color: #ffffff; font-size: 24px; font-weight: 800;">Dawa24 | دوا 24</h1>
                    <p style="margin: 2px 0 0 0; color: #e0f2fe; font-size: 13px;">سوق الأدوية والمستلزمات الطبية المعتمد</p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          <!-- Content -->
          <tr>
            <td style="padding: 36px 32px; direction: rtl; text-align: right;">
              <div style="display: inline-block; background-color: #ecfdf5; color: #047857; font-size: 13px; font-weight: 700; padding: 5px 12px; border-radius: 6px; margin-bottom: 16px;">✓ اعتماد رسمي مكتمل</div>
              <h2 style="margin: 0 0 12px 0; color: #0f172a; font-size: 21px; font-weight: 800;">مرحباً %s،</h2>
              <h3 style="margin: 0 0 16px 0; color: #0284c7; font-size: 17px; font-weight: 700;">%s</h3>
              <p style="margin: 0 0 24px 0; color: #334155; font-size: 15px; line-height: 1.7;">%s</p>

              <!-- Success Card -->
              <div style="background-color: #f8fafc; border: 1px solid #e2e8f0; border-radius: 10px; padding: 18px 20px; margin: 0 0 28px 0;">
                <p style="margin: 0; color: #475569; font-size: 14px; line-height: 1.6;">
                  تم تفعيل وتوثيق حسابكم بنجاح من قِبل إدارة المنصة، ويمكنكم الآن تسجيل الدخول والاستفادة من كافة الصلاحيات والخدمات المتاحة فوراً.
                </p>
              </div>

              <!-- CTA Button -->
              <div style="text-align: center; margin: 28px 0 12px 0;">
                <a href="%s" style="display: inline-block; background-color: #0284c7; color: #ffffff; font-size: 15px; font-weight: 700; text-decoration: none; padding: 13px 32px; border-radius: 8px; box-shadow: 0 2px 5px rgba(2,132,199,0.3);">تسجيل الدخول إلى حسابك &larr;</a>
              </div>
            </td>
          </tr>

          <!-- Footer -->
          <tr>
            <td style="background-color: #f8fafc; border-top: 1px solid #e2e8f0; padding: 20px 32px; text-align: center;">
              <p style="margin: 0 0 6px 0; color: #64748b; font-size: 12px;">
                منصة دوا 24 — المنظومة الرقمية الشاملة لتجارة وتوريد الأدوية والمستلزمات الطبية
              </p>
              <p style="margin: 0; color: #94a3b8; font-size: 11px;">
                © 2026 Dawa24 Store. All rights reserved. &bull; <a href="https://dawa24.com" style="color: #0284c7; text-decoration: none;">dawa24.com</a>
              </p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`,
		subject,
		userName,
		title,
		message,
		actionURL,
	)

	text = fmt.Sprintf(`Dawa24 | دوا 24
مرحباً %s،

%s
%s

تسجيل الدخول إلى حسابك:
%s

https://dawa24.com
`, userName, title, message, actionURL)

	return subject, html, text
}
