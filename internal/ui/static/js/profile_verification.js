/**
 * Profile Settings Verification Handler
 * Provides inline OTP verification for Email and Phone directly within the profile settings tab.
 */
(function () {
  'use strict';

  // The settings page is also reached through boosted navigation, where this
  // script runs after DOMContentLoaded has already fired.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initProfileVerification);
  } else {
    initProfileVerification();
  }

  function initProfileVerification() {
    initEmailVerification();
    initPhoneVerification();
  }

  // bindOnce marks an element so a second run of this script (a revisit
  // through boosted navigation) does not attach every listener twice.
  function bindOnce(el) {
    if (!el || el.dataset.verifyBound) return false;
    el.dataset.verifyBound = '1';
    return true;
  }

  function makeVerifiedBadge(id, label) {
    const b = document.createElement('span');
    b.id = id;
    b.className = 'badge badge-emerald d-inline-flex items-center gap-1 py-0.5 px-2 text-2xs font-semibold';
    const t = document.createElement('span');
    t.textContent = label;
    b.appendChild(t);
    return b;
  }

  // ================= EMAIL VERIFICATION =================
  function initEmailVerification() {
    const openBtn = document.getElementById('profile-verify-email-open-btn');
    const otpBox = document.getElementById('profile-email-otp-box');
    const emailInput = document.getElementById('user-email');
    const codeInput = document.getElementById('profile-email-otp-code');
    const verifyBtn = document.getElementById('profile-email-verify-btn');
    const resendBtn = document.getElementById('profile-email-resend-btn');
    const cancelBtn = document.getElementById('profile-email-cancel-btn');
    const timerSpan = document.getElementById('profile-email-cooldown-timer');
    const feedback = document.getElementById('profile-email-otp-feedback');
    const unverifiedBadge = document.getElementById('profile-email-badge-unverified');
    const verifiedBadge = document.getElementById('profile-email-badge-verified');

    let cooldownInterval = null;

    if (!openBtn && !otpBox) return;
    if (!bindOnce(otpBox || openBtn)) return;

    function showFeedback(msg, isError) {
      if (!feedback) return;
      feedback.textContent = msg;
      feedback.className = 'text-xs font-medium mt-2 ' + (isError ? 'text-danger' : 'text-emerald');
      feedback.classList.remove('s-hidden-init');
      feedback.style.display = 'block';
    }

    function clearFeedback() {
      if (!feedback) return;
      feedback.textContent = '';
      feedback.style.display = 'none';
    }

    function startCooldown(seconds) {
      clearInterval(cooldownInterval);
      let remaining = seconds;
      if (resendBtn) resendBtn.style.display = 'none';
      if (timerSpan) {
        timerSpan.textContent = `${remaining}s`;
        timerSpan.style.display = 'inline';
      }

      cooldownInterval = setInterval(() => {
        remaining -= 1;
        if (remaining <= 0) {
          clearInterval(cooldownInterval);
          if (timerSpan) timerSpan.textContent = '';
          if (resendBtn) {
            resendBtn.style.display = 'inline-flex';
            resendBtn.disabled = false;
          }
        } else if (timerSpan) {
          timerSpan.textContent = `${remaining}s`;
        }
      }, 1000);
    }

    async function sendOTP() {
      const email = emailInput ? emailInput.value.trim() : '';
      if (!email || !email.includes('@') || !email.includes('.')) {
        if (emailInput) emailInput.focus();
        showFeedback('يرجى إدخال بريد إلكتروني صحيح أولاً', true);
        return;
      }

      clearFeedback();
      if (openBtn) openBtn.disabled = true;
      if (resendBtn) resendBtn.disabled = true;
      if (otpBox) otpBox.style.display = 'block';

      try {
        const res = await fetch('/settings/profile/send-email-otp', {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body: new URLSearchParams({ email: email })
        });
        const data = await res.json();

        if (res.ok && data.ok) {
          showFeedback(data.message || 'تم إرسال رمز التحقق إلى بريدك الإلكتروني بنجاح', false);
          startCooldown(data.cooldown || 60);
          if (codeInput) {
            codeInput.value = '';
            codeInput.focus();
          }
        } else {
          showFeedback(data.message || 'فشل إرسال رمز التحقق، يرجى المحاولة لاحقاً', true);
          if (data.cooldown) startCooldown(data.cooldown);
        }
      } catch (err) {
        showFeedback('حدث خطأ في الاتصال بالخادم، حاول ثانية', true);
      } finally {
        if (openBtn) openBtn.disabled = false;
      }
    }

    async function verifyOTP() {
      const email = emailInput ? emailInput.value.trim() : '';
      const code = codeInput ? codeInput.value.trim() : '';
      if (!code || code.length !== 6) {
        showFeedback('يرجى إدخال رمز التحقق المكون من 6 أرقام', true);
        if (codeInput) codeInput.focus();
        return;
      }

      clearFeedback();
      if (verifyBtn) {
        verifyBtn.disabled = true;
        verifyBtn.textContent = 'جارٍ التحقق…';
      }

      try {
        const res = await fetch('/settings/profile/verify-email-otp', {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body: new URLSearchParams({ email: email, code: code })
        });
        const data = await res.json();

        if (res.ok && data.ok && data.verified) {
          showFeedback(data.message || 'تم تأكيد البريد الإلكتروني بنجاح!', false);
          clearInterval(cooldownInterval);
          setTimeout(() => {
            if (otpBox) otpBox.style.display = 'none';
            if (unverifiedBadge) unverifiedBadge.style.display = 'none';
            if (openBtn) openBtn.style.display = 'none';
            if (verifiedBadge) {
              verifiedBadge.style.display = 'inline-flex';
            } else if (unverifiedBadge && unverifiedBadge.parentElement) {
              unverifiedBadge.parentElement.appendChild(
                makeVerifiedBadge('profile-email-badge-verified', unverifiedBadge.dataset.verifiedLabel || 'البريد مؤكد'));
            }
          }, 800);
        } else {
          showFeedback(data.message || 'رمز التأكيد غير صحيح أو منتهي الصلاحية', true);
          if (codeInput) {
            codeInput.focus();
            codeInput.select();
          }
        }
      } catch (err) {
        showFeedback('حدث خطأ أثناء التحقق، يرجى المحاولة ثانية', true);
      } finally {
        if (verifyBtn) {
          verifyBtn.disabled = false;
          verifyBtn.textContent = 'تأكيد الرمز';
        }
      }
    }

    if (openBtn) openBtn.addEventListener('click', sendOTP);
    if (resendBtn) resendBtn.addEventListener('click', sendOTP);
    if (verifyBtn) verifyBtn.addEventListener('click', verifyOTP);
    if (cancelBtn) {
      cancelBtn.addEventListener('click', () => {
        if (otpBox) otpBox.style.display = 'none';
        clearFeedback();
        clearInterval(cooldownInterval);
      });
    }
    if (codeInput) {
      codeInput.addEventListener('input', () => {
        if (codeInput.value.trim().length === 6) verifyOTP();
      });
      codeInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          verifyOTP();
        }
      });
    }
  }

  // ================= PHONE VERIFICATION =================
  function initPhoneVerification() {
    const openBtn = document.getElementById('profile-verify-phone-open-btn');
    const otpBox = document.getElementById('profile-phone-otp-box');
    const phoneInput = document.getElementById('user-phone');
    const codeInput = document.getElementById('profile-phone-otp-code');
    const verifyBtn = document.getElementById('profile-phone-verify-btn');
    const resendBtn = document.getElementById('profile-phone-resend-btn');
    const cancelBtn = document.getElementById('profile-phone-cancel-btn');
    const timerSpan = document.getElementById('profile-phone-cooldown-timer');
    const feedback = document.getElementById('profile-phone-otp-feedback');
    const unverifiedBadge = document.getElementById('profile-phone-badge-unverified');
    const verifiedBadge = document.getElementById('profile-phone-badge-verified');

    let cooldownInterval = null;
    let currentRequestId = '';

    if (!openBtn && !otpBox) return;
    if (!bindOnce(otpBox || openBtn)) return;

    function showFeedback(msg, isError) {
      if (!feedback) return;
      feedback.textContent = msg;
      feedback.className = 'text-xs font-medium mt-2 ' + (isError ? 'text-danger' : 'text-emerald');
      feedback.classList.remove('s-hidden-init');
      feedback.style.display = 'block';
    }

    function clearFeedback() {
      if (!feedback) return;
      feedback.textContent = '';
      feedback.style.display = 'none';
    }

    function startCooldown(seconds) {
      clearInterval(cooldownInterval);
      let remaining = seconds;
      if (resendBtn) resendBtn.style.display = 'none';
      if (timerSpan) {
        timerSpan.textContent = `${remaining}s`;
        timerSpan.style.display = 'inline';
      }

      cooldownInterval = setInterval(() => {
        remaining -= 1;
        if (remaining <= 0) {
          clearInterval(cooldownInterval);
          if (timerSpan) timerSpan.textContent = '';
          if (resendBtn) {
            resendBtn.style.display = 'inline-flex';
            resendBtn.disabled = false;
          }
        } else if (timerSpan) {
          timerSpan.textContent = `${remaining}s`;
        }
      }, 1000);
    }

    async function sendOTP() {
      const phone = phoneInput ? phoneInput.value.trim() : '';
      if (!phone) {
        if (phoneInput) phoneInput.focus();
        showFeedback('يرجى إدخال رقم هاتف صحيح أولاً', true);
        return;
      }

      clearFeedback();
      if (openBtn) openBtn.disabled = true;
      if (resendBtn) resendBtn.disabled = true;
      if (otpBox) otpBox.style.display = 'block';

      try {
        const res = await fetch('/settings/profile/send-phone-otp', {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body: new URLSearchParams({ phone: phone })
        });
        const data = await res.json();

        if (res.ok && data.ok) {
          currentRequestId = data.request_id || '';
          showFeedback(data.message || 'تم إرسال رمز التحقق عبر تيليجرام بنجاح', false);
          startCooldown(data.cooldown || 60);
          if (codeInput) {
            codeInput.value = '';
            codeInput.focus();
          }
        } else {
          showFeedback(data.message || 'فشل إرسال رمز التحقق، يرجى المحاولة لاحقاً', true);
          if (data.cooldown) startCooldown(data.cooldown);
        }
      } catch (err) {
        showFeedback('حدث خطأ في الاتصال بالخادم، حاول ثانية', true);
      } finally {
        if (openBtn) openBtn.disabled = false;
      }
    }

    async function verifyOTP() {
      const phone = phoneInput ? phoneInput.value.trim() : '';
      const code = codeInput ? codeInput.value.trim() : '';
      if (!code || code.length !== 6) {
        showFeedback('يرجى إدخال رمز التحقق المكون من 6 أرقام', true);
        if (codeInput) codeInput.focus();
        return;
      }

      clearFeedback();
      if (verifyBtn) {
        verifyBtn.disabled = true;
        verifyBtn.textContent = 'جارٍ التحقق…';
      }

      try {
        const res = await fetch('/settings/profile/verify-phone-otp', {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body: new URLSearchParams({ phone: phone, request_id: currentRequestId, code: code })
        });
        const data = await res.json();

        if (res.ok && data.ok && data.verified) {
          showFeedback(data.message || 'تم تأكيد رقم الهاتف بنجاح!', false);
          clearInterval(cooldownInterval);
          setTimeout(() => {
            if (otpBox) otpBox.style.display = 'none';
            if (unverifiedBadge) unverifiedBadge.style.display = 'none';
            if (openBtn) openBtn.style.display = 'none';
            if (verifiedBadge) {
              verifiedBadge.style.display = 'inline-flex';
            } else if (unverifiedBadge && unverifiedBadge.parentElement) {
              unverifiedBadge.parentElement.appendChild(
                makeVerifiedBadge('profile-phone-badge-verified', unverifiedBadge.dataset.verifiedLabel || 'الهاتف مؤكد'));
            }
          }, 800);
        } else {
          showFeedback(data.message || 'رمز التأكيد غير صحيح أو منتهي الصلاحية', true);
          if (codeInput) {
            codeInput.focus();
            codeInput.select();
          }
        }
      } catch (err) {
        showFeedback('حدث خطأ أثناء التحقق، يرجى المحاولة ثانية', true);
      } finally {
        if (verifyBtn) {
          verifyBtn.disabled = false;
          verifyBtn.textContent = 'تأكيد الرمز';
        }
      }
    }

    if (openBtn) openBtn.addEventListener('click', sendOTP);
    if (resendBtn) resendBtn.addEventListener('click', sendOTP);
    if (verifyBtn) verifyBtn.addEventListener('click', verifyOTP);
    if (cancelBtn) {
      cancelBtn.addEventListener('click', () => {
        if (otpBox) otpBox.style.display = 'none';
        clearFeedback();
        clearInterval(cooldownInterval);
      });
    }
    if (codeInput) {
      codeInput.addEventListener('input', () => {
        if (codeInput.value.trim().length === 6) verifyOTP();
      });
      codeInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          verifyOTP();
        }
      });
    }
  }
})();
