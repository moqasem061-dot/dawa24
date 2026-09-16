/* ==========================================================================
   DAWA 24 — REGISTRATION & ONBOARDING MODULE (registration.js)
   Multi-step stepper, file upload preview, verification & password strength
   ========================================================================== */

(function () {
  'use strict';

  function selectAccountType(type) {
    if (!type) type = 'customer';
    if (type === 'supplier') type = 'vendor';
    if (type === 'seeker' || type === 'jobseeker') type = 'job_seeker';

    const hiddenInput = document.getElementById('reg-account-type-input');
    if (hiddenInput) {
      hiddenInput.value = type;
    }

    const typeCards = document.querySelectorAll('[data-account-type]');
    typeCards.forEach((c) => {
      const cardType = c.getAttribute('data-account-type');
      const isSelected = (cardType === type);
      c.classList.toggle('active', isSelected);
      c.classList.toggle('selected', isSelected);
      c.setAttribute('aria-checked', isSelected ? 'true' : 'false');
      const radio = c.querySelector('input[type="radio"]');
      if (radio) {
        radio.checked = isSelected;
      }
    });

    const badgeLabel = document.getElementById('reg-selected-badge');
    if (badgeLabel) {
      if (type === 'supplier' || type === 'vendor') {
        badgeLabel.textContent = 'نوع الحساب: مورّد / شركة ومخزن أدوية';
      } else if (type === 'job_seeker') {
        badgeLabel.textContent = 'نوع الحساب: باحث عن عمل / كادر طبي وصيدلاني';
      } else {
        badgeLabel.textContent = 'نوع الحساب: صيدلية / منشأة طبية مرخصة';
      }
    }

    document.querySelectorAll('[data-type-visibility]').forEach((el) => {
      const allowed = el.getAttribute('data-type-visibility').split(/\s+/);
      const isVisible = allowed.includes(type);
      el.classList.toggle('d-none', !isVisible);
    });

    if (type !== 'job_seeker') {
      setTimeout(() => {
        document.querySelectorAll('[data-map-picker] .map-canvas, [data-map-picker], .leaflet-container').forEach((c) => {
          if (c._leaflet_map) c._leaflet_map.invalidateSize();
        });
      }, 100);
    }
  }
  window.dawaSelectAccountType = selectAccountType;

  // 3-Step Registration Onboarding Controller & Password Strength Meter
  function initRegistrationStepper() {
  const form = document.getElementById('registration-onboarding-form');
  const typeCards = document.querySelectorAll('[data-account-type]');
  const hiddenInput = document.getElementById('reg-account-type-input');
  const badgeLabel = document.getElementById('reg-selected-badge');

  const step1 = document.getElementById('reg-step-1');
  const step2 = document.getElementById('reg-step-2');
  const step3 = document.getElementById('reg-step-3');

  const stepIndicator1 = document.getElementById('step-indicator-1');
  const stepIndicator2 = document.getElementById('step-indicator-2');
  const stepIndicator3 = document.getElementById('step-indicator-3');

  const gotoStep2Btn = document.getElementById('reg-goto-step-2');
  const backToStep1Btns = [document.getElementById('reg-back-to-step-1'), document.getElementById('reg-step-2-back')];
  const gotoStep3Btn = document.getElementById('reg-goto-step-3');
  const backToStep2Btns = [document.getElementById('reg-step-3-back'), document.getElementById('reg-step-3-back-btn')];

  if (!step1 || !step2) return;

  function showStep(stepNum) {
    const steps = [step1, step2, step3];
    steps.forEach((s, idx) => {
      if (!s) return;
      if (idx + 1 === stepNum) {
        s.classList.remove('d-none');
        s.classList.add('active');
      } else {
        s.classList.remove('active');
        s.classList.add('d-none');
      }
    });

    const indicators = [stepIndicator1, stepIndicator2, stepIndicator3];
    indicators.forEach((ind, idx) => {
      if (!ind) return;
      const num = ind.querySelector('.step-num');
      if (idx + 1 === stepNum) {
        ind.className = 'onboard-step active';
        if (num) num.textContent = (idx + 1).toString();
      } else if (idx + 1 < stepNum) {
        ind.className = 'onboard-step done';
        if (num) num.textContent = '✓';
      } else {
        ind.className = 'onboard-step';
        if (num) num.textContent = (idx + 1).toString();
      }
    });

    window.scrollTo({ top: 0, behavior: 'smooth' });

    // Invalidate Leaflet maps on step change with multiple safety timeouts
    if (typeof L !== 'undefined') {
      [50, 150, 300, 600].forEach((delay) => {
        setTimeout(() => {
          document.querySelectorAll('[data-map-picker] .map-canvas, [data-map-picker], .leaflet-container').forEach((c) => {
            if (c._leaflet_map) c._leaflet_map.invalidateSize();
          });
          window.dispatchEvent(new Event('resize'));
        }, delay);
      });
    }
  }

  // Account Type Selection Cards
  typeCards.forEach((card) => {
    card.addEventListener('click', () => {
      const type = card.getAttribute('data-account-type');
      if (type) {
        selectAccountType(type);
      }
    });

    card.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ' || e.key === 'Spacebar') {
        e.preventDefault();
        const type = card.getAttribute('data-account-type');
        if (type) {
          selectAccountType(type);
        }
      }
    });

    const radio = card.querySelector('input[type="radio"]');
    if (radio) {
      radio.addEventListener('change', () => {
        if (radio.checked) {
          selectAccountType(radio.value);
        }
      });
    }
  });

  // Make stepper indicators clickable for easy navigation
  [stepIndicator1, stepIndicator2, stepIndicator3].forEach((ind, idx) => {
    if (!ind) return;
    ind.style.cursor = 'pointer';
    ind.addEventListener('click', () => {
      showStep(idx + 1);
    });
  });

  // Initial step and visibility setup
  const initialType = hiddenInput ? (hiddenInput.value || 'customer') : 'customer';
  selectAccountType(initialType);

  // If returning with an error alert, auto-advance to step 3 so the user stays on their filled form
  const errorAlert = document.querySelector('.alert-danger');
  if (errorAlert && errorAlert.textContent.trim()) {
    const errText = errorAlert.textContent.trim();
    if (errText.includes('محافظة') || errText.includes('مدينة') || errText.includes('سجل') || errText.includes('منشأة') || errText.includes('ترخيص')) {
      showStep(2);
    } else {
      showStep(3);
    }
  } else {
    showStep(1);
  }

  if (gotoStep2Btn) gotoStep2Btn.addEventListener('click', () => showStep(2));
  backToStep1Btns.forEach((b) => { if (b) b.addEventListener('click', () => showStep(1)); });

  if (gotoStep3Btn) {
    gotoStep3Btn.addEventListener('click', () => {
      const currentType = hiddenInput ? hiddenInput.value : 'customer';
      if (currentType === 'job_seeker') {
        const spec = document.getElementById('reg-specialisation');
        if (spec && !spec.value) {
          spec.focus();
          return;
        }
      } else {
        const legalName = document.getElementById('reg-legal-name');
        const tradeAr = document.getElementById('reg-trade-ar');
        const cr = document.getElementById('reg-cr');
        const address = document.getElementById('reg-address');

        if (legalName && !legalName.value.trim()) {
          legalName.focus();
          return;
        }
        if (tradeAr && !tradeAr.value.trim()) {
          tradeAr.focus();
          return;
        }
        if (cr && !cr.value.trim()) {
          cr.focus();
          return;
        }
        if (address && !address.value.trim()) {
          address.focus();
          return;
        }
      }

      showStep(3);
    });
  }

  backToStep2Btns.forEach((b) => { if (b) b.addEventListener('click', () => showStep(2)); });

  // Real-Time Password Strength Meter & Security Checklist
  const pwdInput = document.getElementById('reg-password');
  const togglePwdBtn = document.getElementById('reg-toggle-pwd');
  const strengthLabel = document.getElementById('pass-strength-label');
  const barSegments = document.querySelectorAll('#pwd-strength-bars .strength-bar-seg');
  const ruleLen = document.getElementById('rule-length');
  const ruleCase = document.getElementById('rule-casing');
  const ruleNum = document.getElementById('rule-number');
  const ruleSpec = document.getElementById('rule-special');

  if (togglePwdBtn && pwdInput) {
    togglePwdBtn.addEventListener('click', () => {
      const isPass = pwdInput.type === 'password';
      pwdInput.type = isPass ? 'text' : 'password';
    });
  }

  if (pwdInput) {
    pwdInput.addEventListener('input', () => {
      const val = pwdInput.value;
      const hasLen = val.length >= 8;
      const hasUpper = /[A-Z]/.test(val);
      const hasLower = /[a-z]/.test(val);
      const hasCasing = hasUpper && hasLower;
      const hasNum = /[0-9]/.test(val);
      const hasSpec = /[^A-Za-z0-9]/.test(val);

      function updateRule(el, pass) {
        if (!el) return;
        const icon = el.querySelector('.rule-icon');
        if (pass) {
          el.className = 'pwd-checklist-item valid';
          if (icon) icon.textContent = '✓';
        } else {
          el.className = 'pwd-checklist-item';
          if (icon) icon.textContent = '●';
        }
      }

      updateRule(ruleLen, hasLen);
      updateRule(ruleCase, hasCasing);
      updateRule(ruleNum, hasNum);
      updateRule(ruleSpec, hasSpec);

      let score = 0;
      if (hasLen) score++;
      if (hasCasing) score++;
      if (hasNum) score++;
      if (hasSpec) score++;

      const colors = ['#ef4444', '#f97316', '#eab308', '#10b981'];
      const labels = ['أدخل كلمة المرور', 'ضعيفة جداً', 'ضعيفة', 'متوسطة', 'قوية ومطابقة للمعايير'];

      if (val.length === 0) {
        if (strengthLabel) {
          strengthLabel.textContent = labels[0];
          strengthLabel.style.color = 'var(--text-muted)';
        }
        barSegments.forEach((b) => { b.style.backgroundColor = 'var(--border)'; });
      } else {
        const color = colors[score - 1] || colors[0];
        if (strengthLabel) {
          strengthLabel.textContent = labels[score] || labels[1];
          strengthLabel.style.color = color;
        }
        barSegments.forEach((b, idx) => {
          b.style.backgroundColor = idx < score ? color : 'var(--border)';
        });
      }
    });
  }

  // --- Telegram Gateway OTP Phone Verification ---
  const sendTgBtn = document.getElementById('tg-send-otp-btn');
  const sendTgBtnText = document.getElementById('tg-send-btn-text');
  const regPhoneInput = document.getElementById('reg-phone');
  const tgOtpBox = document.getElementById('tg-otp-box');
  const tgOtpCode = document.getElementById('tg-otp-code');
  const tgVerifyBtn = document.getElementById('tg-verify-btn');
  const tgResendBtn = document.getElementById('tg-resend-btn');
  const tgCooldownTimer = document.getElementById('tg-cooldown-timer');
  const tgFeedback = document.getElementById('tg-otp-feedback');
  const tgVerifiedBadge = document.getElementById('tg-verified-badge');
  const tgChangePhoneBtn = document.getElementById('tg-change-phone-btn');
  const verifiedTokenInput = document.getElementById('verified-phone-token');

  let tgRequestID = '';
  let tgCooldownInterval = null;

  function showTgFeedback(msg, isError) {
    if (!tgFeedback) return;
    tgFeedback.style.display = 'block';
    tgFeedback.style.color = isError ? '#ef4444' : '#10b981';
    tgFeedback.textContent = msg;
  }

  function clearTgFeedback() {
    if (tgFeedback) {
      tgFeedback.style.display = 'none';
      tgFeedback.textContent = '';
    }
  }

  function startTgCooldown(seconds) {
    clearInterval(tgCooldownInterval);
    let remaining = seconds;
    if (tgCooldownTimer) tgCooldownTimer.textContent = `(إعادة الإرسال خلال ${remaining}ث)`;
    if (tgResendBtn) tgResendBtn.style.display = 'none';
    if (sendTgBtn) sendTgBtn.disabled = true;

    tgCooldownInterval = setInterval(() => {
      remaining--;
      if (remaining <= 0) {
        clearInterval(tgCooldownInterval);
        if (tgCooldownTimer) tgCooldownTimer.textContent = '';
        if (tgResendBtn) tgResendBtn.style.display = 'inline-block';
        if (sendTgBtn) sendTgBtn.disabled = false;
      } else {
        if (tgCooldownTimer) tgCooldownTimer.textContent = `(إعادة الإرسال خلال ${remaining}ث)`;
      }
    }, 1000);
  }

  async function sendTelegramOTP() {
    if (!regPhoneInput) return;
    const phone = regPhoneInput.value.trim();
    if (!phone || phone.length < 8) {
      regPhoneInput.focus();
      showTgFeedback('يرجى إدخال رقم هاتف صحيح أولاً', true);
      return;
    }

    clearTgFeedback();
    if (sendTgBtn) {
      sendTgBtn.disabled = true;
      if (sendTgBtnText) sendTgBtnText.textContent = '⏳ جارٍ الإرسال...';
    }

    try {
      const resp = await fetch('/auth/telegram/send-otp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: 'phone=' + encodeURIComponent(phone)
      });
      const data = await resp.json();

      if (resp.ok && data.ok) {
        tgRequestID = data.request_id || '';
        if (tgOtpBox) tgOtpBox.style.display = 'block';
        if (data.mock_code && tgOtpCode) {
          tgOtpCode.value = data.mock_code;
          showTgFeedback(`تم إرسال الرمز بنجاح! (وضع تجريبي: الرمز ${data.mock_code})`, false);
        } else {
          showTgFeedback('تم إرسال رمز التأكيد إلى تيليجرام بنجاح', false);
        }
        startTgCooldown(data.cooldown || 60);
        if (tgOtpCode) tgOtpCode.focus();
      } else {
        showTgFeedback(data.message || 'تعذر إرسال الرمز، يرجى المحاولة بعد قليل', true);
        if (data.cooldown) {
          startTgCooldown(data.cooldown);
        } else if (sendTgBtn) {
          sendTgBtn.disabled = false;
        }
      }
    } catch (err) {
      showTgFeedback('فشل الاتصال بالخادم، يرجى التحقق من اتصال الإنترنت', true);
      if (sendTgBtn) sendTgBtn.disabled = false;
    } finally {
      if (sendTgBtnText) sendTgBtnText.textContent = 'تأكيد عبر تيليجرام';
    }
  }

  async function verifyTelegramOTP() {
    if (!regPhoneInput || !tgOtpCode) return;
    const phone = regPhoneInput.value.trim();
    const code = tgOtpCode.value.trim();

    if (!code || code.length < 4) {
      tgOtpCode.focus();
      showTgFeedback('يرجى إدخال رمز التأكيد المكون من 6 أرقام', true);
      return;
    }

    clearTgFeedback();
    if (tgVerifyBtn) {
      tgVerifyBtn.disabled = true;
      tgVerifyBtn.textContent = '⏳ تحقق...';
    }

    try {
      const resp = await fetch('/auth/telegram/verify-otp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: 'phone=' + encodeURIComponent(phone) + '&request_id=' + encodeURIComponent(tgRequestID) + '&code=' + encodeURIComponent(code)
      });
      const data = await resp.json();

      if (resp.ok && data.ok && data.token) {
        if (verifiedTokenInput) verifiedTokenInput.value = data.token;
        if (regPhoneInput) {
          regPhoneInput.setAttribute('readonly', 'readonly');
          if (data.phone) regPhoneInput.value = data.phone;
        }
        if (sendTgBtn) sendTgBtn.style.display = 'none';
        if (tgOtpBox) tgOtpBox.style.display = 'none';
        if (tgVerifiedBadge) tgVerifiedBadge.style.display = 'inline-flex';
        clearInterval(tgCooldownInterval);
      } else {
        showTgFeedback(data.message || 'رمز التأكيد غير صحيح، حاول مجدداً', true);
        tgOtpCode.focus();
        tgOtpCode.select();
      }
    } catch (err) {
      showTgFeedback('حدث خطأ أثناء التحقق، يرجى المحاولة ثانية', true);
    } finally {
      if (tgVerifyBtn) {
        tgVerifyBtn.disabled = false;
        tgVerifyBtn.textContent = 'تأكيد الرمز';
      }
    }
  }

  function changeTelegramPhone() {
    if (verifiedTokenInput) verifiedTokenInput.value = '';
    if (regPhoneInput) {
      regPhoneInput.removeAttribute('readonly');
      regPhoneInput.focus();
    }
    if (tgVerifiedBadge) tgVerifiedBadge.style.display = 'none';
    if (tgOtpBox) tgOtpBox.style.display = 'none';
    if (sendTgBtn) {
      sendTgBtn.style.display = 'inline-flex';
      sendTgBtn.disabled = false;
    }
    clearTgFeedback();
    clearInterval(tgCooldownInterval);
    if (tgCooldownTimer) tgCooldownTimer.textContent = '';
  }

  if (sendTgBtn) sendTgBtn.addEventListener('click', sendTelegramOTP);
  if (tgResendBtn) tgResendBtn.addEventListener('click', sendTelegramOTP);
  if (tgVerifyBtn) tgVerifyBtn.addEventListener('click', verifyTelegramOTP);
  if (tgChangePhoneBtn) tgChangePhoneBtn.addEventListener('click', changeTelegramPhone);

  if (tgOtpCode) {
    tgOtpCode.addEventListener('input', () => {
      if (tgOtpCode.value.trim().length === 6) {
        verifyTelegramOTP();
      }
    });
    tgOtpCode.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        verifyTelegramOTP();
      }
    });
  }

  // Form Submit Interceptor
  if (form) {
    form.addEventListener('submit', (e) => {
      const name = document.getElementById('reg-name');
      const email = document.getElementById('reg-email');
      const phone = document.getElementById('reg-phone');
      const password = document.getElementById('reg-password');
      const token = document.getElementById('verified-phone-token');

      if (name && !name.value.trim()) {
        e.preventDefault();
        name.focus();
        return;
      }
      if (email && !email.value.trim()) {
        e.preventDefault();
        email.focus();
        return;
      }
      if (phone && !phone.value.trim()) {
        e.preventDefault();
        phone.focus();
        return;
      }
      if (!token || !token.value.trim()) {
        e.preventDefault();
        showTgFeedback('يرجى تأكيد رقم الهاتف عبر رمز تيليجرام أولاً للمتابعة', true);
        if (sendTgBtn && sendTgBtn.style.display !== 'none') {
          sendTgBtn.focus();
        } else if (tgOtpCode && tgOtpBox && tgOtpBox.style.display !== 'none') {
          tgOtpCode.focus();
        } else if (phone) {
          phone.focus();
        }
        return;
      }
      if (password) {
        const val = password.value;
        if (val.length < 8) {
          e.preventDefault();
          password.focus();
          return;
        }
      }
    });
  }
}

// Live Reverse Geocoding via OpenStreetMap Nominatim
let reverseGeocodeTimer = null;
function fetchDetailedAddressFromCoords(lat, lon) {
  clearTimeout(reverseGeocodeTimer);
  reverseGeocodeTimer = setTimeout(async () => {
    try {
      const addressInput = document.getElementById('reg-address') || document.querySelector('input[name="address"]');
      const hint = document.getElementById('reg-address-hint');
      if (!addressInput) return;

      if (hint) hint.textContent = '⏳ جلب العنوان بالتفصيل...';
      const resp = await fetch(`https://nominatim.openstreetmap.org/reverse?format=json&lat=${lat}&lon=${lon}&zoom=18&addressdetails=1&accept-language=ar`, {
        headers: { 'Accept': 'application/json' }
      });
      if (!resp.ok) {
        if (hint) hint.textContent = '📍 تم تحديد الإحداثيات بنجاح';
        return;
      }
      const data = await resp.json();
      if (data && data.address) {
        const a = data.address;
        const parts = [];
        if (a.road || a.street) parts.push(a.road || a.street);
        if (a.neighbourhood || a.suburb || a.quarter) parts.push(a.neighbourhood || a.suburb || a.quarter);
        if (a.city || a.town || a.village || a.county) parts.push(a.city || a.town || a.village || a.county);
        if (a.state) parts.push(a.state);

        const fullAddr = parts.filter(Boolean).join('، ');
        if (fullAddr) {
          addressInput.value = fullAddr;
          addressInput.dispatchEvent(new Event('input', { bubbles: true }));
          addressInput.dispatchEvent(new Event('change', { bubbles: true }));
          if (hint) hint.textContent = '📍 تم تحديث العنوان تلقائياً من الخريطة';
        }
      }
    } catch (e) {
      console.warn('Reverse geocoding error:', e);
    }
  }, 350);
}
window.fetchDetailedAddressFromCoords = fetchDetailedAddressFromCoords;

// Egyptian Cities Coordinates Reference Table
const EGYPT_CITIES_COORDS = [
  { name: 'القاهرة', lat: 30.0444, lon: 31.2357 },
  { name: 'القاهرة الجديدة', lat: 30.0131, lon: 31.2089 },
  { name: 'الشروق', lat: 30.1219, lon: 31.3665 },
  { name: 'مدينة بدر', lat: 30.1842, lon: 31.2482 },
  { name: 'الجيزة', lat: 30.0131, lon: 31.2089 },
  { name: 'مدينة ستة أكتوبر', lat: 30.0648, lon: 30.9706 },
  { name: 'الشيخ زايد', lat: 30.1111, lon: 30.8544 },
  { name: 'الحوامدية', lat: 29.9667, lon: 31.3000 },
  { name: 'أوسيم', lat: 29.8833, lon: 31.2333 },
  { name: 'البدرشين', lat: 29.8167, lon: 31.2833 },
  { name: 'الإسكندرية', lat: 31.2001, lon: 29.9187 },
  { name: 'برج العرب', lat: 31.0333, lon: 29.7667 },
  { name: 'مدينة برج العرب الجديدة', lat: 30.9164, lon: 29.5553 },
  { name: 'شبرا الخيمة', lat: 30.4500, lon: 31.1833 },
  { name: 'الخصوص', lat: 30.4667, lon: 31.1833 },
  { name: 'بنها', lat: 30.4667, lon: 31.1833 },
  { name: 'قليوب', lat: 30.1833, lon: 31.2167 },
  { name: 'العبور', lat: 30.2000, lon: 31.3167 },
  { name: 'بور سعيد', lat: 31.2654, lon: 32.3020 },
  { name: 'بور فؤاد', lat: 31.2333, lon: 32.3167 },
  { name: 'السويس', lat: 29.9668, lon: 32.5498 },
  { name: 'الإسماعيلية', lat: 30.5965, lon: 32.2715 },
  { name: 'فايد', lat: 30.3333, lon: 32.3000 },
  { name: 'القنطرة شرق', lat: 30.8333, lon: 32.3167 },
  { name: 'طنطا', lat: 30.7865, lon: 31.0004 },
  { name: 'المحلة الكبرى', lat: 30.9667, lon: 31.1667 },
  { name: 'كفر الزيات', lat: 30.8167, lon: 30.8167 },
  { name: 'زفتى', lat: 30.7167, lon: 31.2500 },
  { name: 'المنصورة', lat: 31.0409, lon: 31.3785 },
  { name: 'ميت غمر', lat: 30.7167, lon: 31.2500 },
  { name: 'السنبلاوين', lat: 30.8833, lon: 31.4667 },
  { name: 'دكرنس', lat: 31.0833, lon: 31.6000 },
  { name: 'بلقاس', lat: 31.2333, lon: 31.3667 },
  { name: 'طلخا', lat: 31.0500, lon: 31.3667 },
  { name: 'الزقازيق', lat: 30.5877, lon: 31.5020 },
  { name: 'العاشر من رمضان', lat: 30.3000, lon: 31.7333 },
  { name: 'بلبيس', lat: 30.4167, lon: 31.5667 },
  { name: 'فاقوس', lat: 30.7333, lon: 31.8000 },
  { name: 'منيا القمح', lat: 30.5167, lon: 31.3500 },
  { name: 'أبو حماد', lat: 30.5500, lon: 31.6833 },
  { name: 'أبو كبير', lat: 30.7333, lon: 31.6667 },
  { name: 'دمنهور', lat: 31.0409, lon: 30.4667 },
  { name: 'كفر الدوار', lat: 31.1333, lon: 30.1333 },
  { name: 'إدكو', lat: 31.3000, lon: 30.3000 },
  { name: 'رشيد', lat: 31.4000, lon: 30.4167 },
  { name: 'شبين الكوم', lat: 30.5522, lon: 31.0094 },
  { name: 'منوف', lat: 30.4667, lon: 30.9333 },
  { name: 'أشمون', lat: 30.3000, lon: 30.9833 },
  { name: 'قويسنا', lat: 30.5667, lon: 31.1500 },
  { name: 'مدينة السادات', lat: 30.3833, lon: 30.5167 },
  { name: 'كفر الشيخ', lat: 31.1107, lon: 30.9388 },
  { name: 'دسوق', lat: 31.1333, lon: 30.6500 },
  { name: 'فوه', lat: 31.2000, lon: 30.5500 },
  { name: 'دمياط', lat: 31.4165, lon: 31.8133 },
  { name: 'دمياط الجديدة', lat: 31.4333, lon: 31.6667 },
  { name: 'رأس البر', lat: 31.5167, lon: 31.8167 },
  { name: 'الفيوم', lat: 29.3084, lon: 30.8428 },
  { name: 'بني سويف', lat: 29.0661, lon: 31.0994 },
  { name: 'المنيا', lat: 28.1099, lon: 30.7503 },
  { name: 'ملوي', lat: 27.7333, lon: 30.8333 },
  { name: 'أسيوط', lat: 27.1801, lon: 31.1837 },
  { name: 'ديروط', lat: 27.5667, lon: 30.8167 },
  { name: 'سوهاج', lat: 26.5569, lon: 31.6948 },
  { name: 'طهطا', lat: 26.7667, lon: 31.5000 },
  { name: 'جرجا', lat: 26.3333, lon: 31.8833 },
  { name: 'قنا', lat: 26.1642, lon: 32.7267 },
  { name: 'نجع حمادي', lat: 26.0500, lon: 32.2500 },
  { name: 'الأقصر', lat: 25.6872, lon: 32.6396 },
  { name: 'إسنا', lat: 25.2833, lon: 32.5500 },
  { name: 'أسوان', lat: 24.0889, lon: 32.8998 },
  { name: 'إدفو', lat: 24.9833, lon: 32.8667 },
  { name: 'كوم أمبو', lat: 24.4667, lon: 32.9500 },
  { name: 'مطروح', lat: 31.3525, lon: 27.2453 },
  { name: 'العلمين', lat: 30.8333, lon: 28.9500 },
  { name: 'سيوة', lat: 29.2000, lon: 25.5167 },
  { name: 'الغردقة', lat: 27.2579, lon: 33.8116 },
  { name: 'سفاجا', lat: 26.7292, lon: 33.9365 },
  { name: 'مرسى علم', lat: 25.0676, lon: 34.8966 },
  { name: 'الخارجة', lat: 25.4390, lon: 30.5586 },
  { name: 'الداخلة', lat: 25.5167, lon: 28.9667 },
  { name: 'العريش', lat: 31.1316, lon: 33.7984 },
  { name: 'الطور', lat: 28.2410, lon: 33.6230 },
  { name: 'شرم الشيخ', lat: 27.9158, lon: 34.3299 },
  { name: 'دهب', lat: 28.5094, lon: 34.5137 },
  { name: 'نويبع', lat: 29.0436, lon: 34.6644 },
  { name: 'طابا', lat: 29.4925, lon: 34.8967 },
  { name: 'رأس سدر', lat: 29.5892, lon: 32.7144 }
];

let isMapSyncing = false;

function setMapLocationHelper(container, lat, lon, zoom) {
  if (!container) return;
  if (typeof window.dawaSetMapLocation === 'function') {
    window.dawaSetMapLocation(container, lat, lon, zoom);
    return;
  }
  const canvas = container.querySelector('.map-canvas, .map-container, [data-map-canvas], .leaflet-map-canvas') || container;
  const updateFn = container._updateCoords || (canvas && canvas._updateCoords);
  const targetZoom = (typeof zoom === 'number' && zoom > 0) ? zoom : 14;
  const targetLat = parseFloat(lat);
  const targetLon = parseFloat(lon);

  if (isNaN(targetLat) || isNaN(targetLon) || (targetLat === 0 && targetLon === 0)) return;

  if (typeof updateFn === 'function') {
    updateFn(targetLat, targetLon, targetZoom, false);
  } else if (canvas._leaflet_map || container._leaflet_map) {
    const map = canvas._leaflet_map || container._leaflet_map;
    map.setView([targetLat, targetLon], targetZoom);
  } else {
    container._pendingCoords = { lat: targetLat, lon: targetLon, zoom: targetZoom };
    if (canvas !== container) {
      canvas._pendingCoords = { lat: targetLat, lon: targetLon, zoom: targetZoom };
    }
  }

  const latInput = container.querySelector('[data-map-lat], [data-map-input="lat"], input[name="latitude"], input[name="branch_lat"]');
  const lonInput = container.querySelector('[data-map-lon], [data-map-input="lon"], input[name="longitude"], input[name="branch_lon"]');
  if (latInput) latInput.value = targetLat.toFixed(6);
  if (lonInput) lonInput.value = targetLon.toFixed(6);
  const badge = container.querySelector('[data-map-badge], [data-map-coords-badge]');
  if (badge) badge.textContent = `${targetLat.toFixed(4)}, ${targetLon.toFixed(4)}`;
  const gmapsInput = container.querySelector('[data-map-google-url], [data-map-input="google_url"], input[name="google_maps_url"], input[name="branch_google_maps_url"]');
  if (gmapsInput) gmapsInput.value = `https://www.google.com/maps?q=${targetLat},${targetLon}`;
}

function findCityCoordsByName(name) {
  if (!name) return null;
  const clean = name.trim().toLowerCase();
  for (const c of EGYPT_CITIES_COORDS) {
    if (c.name.toLowerCase() === clean || clean.includes(c.name.toLowerCase()) || c.name.toLowerCase().includes(clean)) {
      return [c.lat, c.lon];
    }
  }
  return null;
}

function initRegistrationMapComboboxSync() {
  window.addEventListener('combobox-change', function (e) {
    if (isMapSyncing) return;
    if (!e.detail || !e.detail.name) return;
    const name = e.detail.name;
    const val = e.detail.value;
    const item = e.detail.item;

    const mapContainer = document.querySelector('[data-map-picker]');
    if (!mapContainer) return;

    if (name === 'branch_governorate_id') {
      if (!val) return;
      let pos = null;
      const govsCoordsEl = document.getElementById('reg-govs-coords');
      if (govsCoordsEl) {
        try {
          const govs = JSON.parse(govsCoordsEl.textContent);
          pos = govs[String(val)];
        } catch (err) {
          console.warn('branch_governorate_id map sync error:', err);
        }
      }
      if (!pos && item && item.label) {
        pos = findCityCoordsByName(item.label);
      }
      if (pos && (pos[0] || pos[1])) {
        setMapLocationHelper(mapContainer, pos[0], pos[1], 11);
        if (typeof window.fetchDetailedAddressFromCoords === 'function') {
          window.fetchDetailedAddressFromCoords(pos[0], pos[1]);
        }
      }
    } else if (name === 'branch_city_id') {
      if (val) {
        let pos = null;
        const citiesCoordsEl = document.getElementById('reg-cities-coords');
        if (citiesCoordsEl) {
          try {
            const raw = citiesCoordsEl.getAttribute('data-coords') || citiesCoordsEl.textContent || '{}';
            const cities = JSON.parse(raw);
            pos = cities[String(val)];
          } catch (err) {
            console.warn('branch_city_id map sync error:', err);
          }
        }
        if ((!pos || (!pos[0] && !pos[1])) && item && item.label) {
          pos = findCityCoordsByName(item.label);
        }
        if (pos && (pos[0] || pos[1])) {
          // If city has parent governorate (pos[2]), auto-select branch_governorate_id
          if (pos[2] && typeof window.dawaComboboxSet === 'function') {
            const curGov = window.dawaComboboxValue ? window.dawaComboboxValue('branch_governorate_id') : '';
            if (!curGov || curGov !== String(pos[2])) {
              window.dawaComboboxSet('branch_governorate_id', String(pos[2]));
            }
          }
          setMapLocationHelper(mapContainer, pos[0], pos[1], 14);
          if (typeof window.fetchDetailedAddressFromCoords === 'function') {
            window.fetchDetailedAddressFromCoords(pos[0], pos[1]);
          }
        }
      } else {
        // City was cleared: if governorate is still selected, pan back to governorate center
        const govVal = window.dawaComboboxValue ? window.dawaComboboxValue('branch_governorate_id') : '';
        if (govVal) {
          const govsCoordsEl = document.getElementById('reg-govs-coords');
          if (govsCoordsEl) {
            try {
              const govs = JSON.parse(govsCoordsEl.textContent);
              const pos = govs[String(govVal)];
              if (pos && (pos[0] || pos[1])) {
                setMapLocationHelper(mapContainer, pos[0], pos[1], 11);
              }
            } catch (err) {}
          }
        }
      }
    }
  });
}

function findClosestEgyptianCity(lat, lon) {
  let closest = null;
  let minDist = Infinity;
  for (const c of EGYPT_CITIES_COORDS) {
    const d = Math.hypot(lat - c.lat, lon - c.lon);
    if (d < minDist) {
      minDist = d;
      closest = c;
    }
  }
  return closest;
}

function syncCityDropdownsWithCoordinates(lat, lon) {
  const coordsEl = document.getElementById('reg-cities-coords') ||
                   document.getElementById('customer-branch-cities-coords') ||
                   document.getElementById('vendor-branch-cities-coords') ||
                   document.getElementById('admin-branch-cities-coords');

  let closestCityId = null;
  let closestGovId = null;
  let minDist = Infinity;

  if (coordsEl) {
    try {
      const raw = coordsEl.getAttribute('data-coords') || coordsEl.textContent || '{}';
      const coords = JSON.parse(raw);
      for (const [cId, pos] of Object.entries(coords)) {
        if (Array.isArray(pos) && pos.length >= 2) {
          const d = Math.hypot(lat - pos[0], lon - pos[1]);
          if (d < minDist) {
            minDist = d;
            closestCityId = cId;
            closestGovId = pos[2] || null;
          }
        }
      }
    } catch (e) {
      console.warn('syncCityDropdownsWithCoordinates parse error:', e);
    }
  }

  // Fallback to static array if no coords script in DOM
  if (!closestCityId) {
    const closest = findClosestEgyptianCity(lat, lon);
    if (!closest) return null;
  }

  if (closestCityId && minDist < 0.45) {
    isMapSyncing = true;
    try {
      document.querySelectorAll('[data-map-city-id], input[name="branch_city_id"]').forEach((hi) => {
        hi.value = closestCityId;
      });
      if (typeof window.dawaComboboxSet === 'function') {
        if (closestGovId) {
          window.dawaComboboxSet('branch_governorate_id', String(closestGovId));
        }
        window.dawaComboboxSet('branch_city_id', String(closestCityId));
      }
    } finally {
      setTimeout(() => { isMapSyncing = false; }, 200);
    }
  }

  return closestCityId;
}
window.syncCityDropdownsWithCoordinates = syncCityDropdownsWithCoordinates;
window.initRegistrationStepper = initRegistrationStepper;
window.initRegistrationMapComboboxSync = initRegistrationMapComboboxSync;

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => {
    initRegistrationStepper();
    initRegistrationMapComboboxSync();
  });
} else {
  initRegistrationStepper();
  initRegistrationMapComboboxSync();
}
})();