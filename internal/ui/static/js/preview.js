/**
 * Universal In-Window File & Image Preview Lightbox
 * Dawa24 Platform
 */
(function () {
  'use strict';

  function getModalElements() {
    return {
      modal: document.getElementById('universal-file-preview-modal'),
      titleEl: document.getElementById('universal-preview-title'),
      filenameEl: document.getElementById('universal-preview-filename'),
      dlBtn: document.getElementById('universal-preview-download'),
      newtabBtn: document.getElementById('universal-preview-newtab'),
      iframe: document.getElementById('universal-preview-iframe'),
      imgContainer: document.getElementById('universal-preview-img-container'),
      img: document.getElementById('universal-preview-img'),
      errBox: document.getElementById('universal-preview-error')
    };
  }

  window.openFilePreview = function (opts) {
    if (!opts || !opts.url) return;
    const url = opts.url;
    const els = getModalElements();
    if (!els.modal) return;

    const clean = url.split('?')[0].split('#')[0];
    const defaultDigitalFile = typeof window.dawaT === 'function' ? window.dawaT('preview.digital_file', 'ملف رقمي') : 'ملف رقمي';
    const filename = opts.filename || (opts.title && opts.title.includes('.') ? opts.title : clean.split('/').pop()) || defaultDigitalFile;
    const ext = (filename.split('.').pop() || clean.split('.').pop() || '').toLowerCase();
    const isImage = ['png', 'jpg', 'jpeg', 'webp', 'gif', 'svg'].includes(ext);

    if (els.titleEl) {
      const defaultImgTitle = typeof window.dawaT === 'function' ? window.dawaT('preview.image', 'معاينة الصورة') : 'معاينة الصورة';
      const defaultDocTitle = typeof window.dawaT === 'function' ? window.dawaT('preview.document', 'معاينة المستند') : 'معاينة المستند';
      els.titleEl.textContent = opts.title || (isImage ? defaultImgTitle : defaultDocTitle);
    }
    if (els.filenameEl) {
      els.filenameEl.textContent = filename;
    }
    if (els.dlBtn) {
      els.dlBtn.href = opts.downloadUrl || url;
      if (filename) els.dlBtn.setAttribute('download', filename);
    }
    if (els.newtabBtn) {
      els.newtabBtn.href = url;
    }
    if (els.errBox) {
      els.errBox.classList.add('hidden');
    }

    if (isImage) {
      if (els.iframe) {
        els.iframe.classList.add('hidden');
        els.iframe.src = 'about:blank';
      }
      if (els.img && els.imgContainer) {
        els.img.onload = function () {
          if (els.errBox) els.errBox.classList.add('hidden');
        };
        els.img.onerror = function () {
          if (els.imgContainer) els.imgContainer.classList.add('hidden');
          if (els.errBox) els.errBox.classList.remove('hidden');
        };
        els.img.src = url;
        els.imgContainer.classList.remove('hidden');
      }
    } else {
      if (els.imgContainer) {
        els.imgContainer.classList.add('hidden');
      }
      if (els.img) {
        els.img.src = '';
      }
      if (els.iframe) {
        els.iframe.classList.remove('hidden');
        els.iframe.src = url;
      }
    }

    if (typeof els.modal.showModal === 'function') {
      try {
        els.modal.showModal();
      } catch (e) {
        els.modal.setAttribute('open', '');
      }
    } else {
      els.modal.setAttribute('open', '');
    }
  };

  window.closeFilePreview = function () {
    const els = getModalElements();
    if (!els.modal) return;
    if (typeof els.modal.close === 'function') {
      try {
        els.modal.close();
      } catch (e) {
        els.modal.removeAttribute('open');
      }
    } else {
      els.modal.removeAttribute('open');
    }
    if (els.iframe) els.iframe.src = 'about:blank';
    if (els.img) els.img.src = '';
  };

  // Close when clicking modal backdrop
  document.addEventListener('DOMContentLoaded', function () {
    const modal = document.getElementById('universal-file-preview-modal');
    if (modal) {
      modal.addEventListener('click', function (e) {
        const box = modal.querySelector('.modal-box');
        if (box && !box.contains(e.target)) {
          window.closeFilePreview();
        }
      });
      modal.addEventListener('close', function () {
        const els = getModalElements();
        if (els.iframe) els.iframe.src = 'about:blank';
        if (els.img) els.img.src = '';
      });
    }
  });

  // Global click interceptor for previewable files/documents
  document.addEventListener('click', function (e) {
    const target = e.target;
    if (!target) return;

    // 1. Explicit preview triggers: [data-preview-file], [data-preview-doc]
    const explicit = target.closest('[data-preview-file], [data-preview-doc]');
    if (explicit) {
      const url = explicit.getAttribute('data-preview-file') || explicit.getAttribute('href');
      if (url && url !== '#' && !url.startsWith('javascript:')) {
        e.preventDefault();
        e.stopPropagation();
        window.openFilePreview({
          url: url,
          title: explicit.getAttribute('data-title') || explicit.getAttribute('title') || explicit.textContent.trim(),
          filename: explicit.getAttribute('data-filename') || explicit.getAttribute('data-orig-name') || '',
          downloadUrl: explicit.getAttribute('data-download-url') || (url.includes('/view') ? url.replace(/\/view$/, '/download') : url)
        });
        return;
      }
    }

    // 2. Standard document view routes: /documents/{id}/view, /admin/documents/{id}/view, /customer/documents/{id}/view, /vendor/documents/{id}/view
    const docLink = target.closest('a[href*="/documents/"][href*="/view"]');
    if (docLink) {
      const href = docLink.getAttribute('href');
      if (href) {
        e.preventDefault();
        e.stopPropagation();
        const cardOrRow = docLink.closest('tr, .document-card, .doc-row, .card, li') || docLink;
        let title = docLink.getAttribute('data-title') || docLink.getAttribute('title');
        let filename = docLink.getAttribute('data-filename');
        if (!title && cardOrRow) {
          const titleEl = cardOrRow.querySelector('.document-title, .doc-title, .font-bold, h4, h3, .card-title');
          if (titleEl) title = titleEl.textContent.trim();
        }
        if (!filename && cardOrRow) {
          const fnEl = cardOrRow.querySelector('.document-filename, .doc-filename, .font-mono, [dir="ltr"]');
          if (fnEl) filename = fnEl.textContent.trim();
        }
        window.openFilePreview({
          url: href,
          title: title || 'معاينة المستند الرسمي',
          filename: filename || '',
          downloadUrl: href.replace(/\/view$/, '/download')
        });
        return;
      }
    }

    // 3. Receipt / image attachment links in tables (e.g. wallet receipts, payment proofs)
    const attLink = target.closest('a[href*="/receipts/"], a[href*="/uploads/"], a[data-receipt-preview]');
    if (attLink) {
      const href = attLink.getAttribute('href');
      if (href && !href.startsWith('javascript:') && href !== '#') {
        const clean = href.split('?')[0].toLowerCase();
        if (clean.endsWith('.pdf') || clean.endsWith('.png') || clean.endsWith('.jpg') || clean.endsWith('.jpeg') || clean.endsWith('.webp')) {
          e.preventDefault();
          e.stopPropagation();
          window.openFilePreview({
            url: href,
            title: attLink.getAttribute('data-title') || attLink.textContent.trim() || 'معاينة المرفق',
            filename: attLink.getAttribute('data-filename') || clean.split('/').pop(),
            downloadUrl: href
          });
          return;
        }
      }
    }
  }, true);
})();
