/* ==========================================================================
   DAWA 24 — INVENTORY & ORDER WIZARD MODULE (wizard.js)
   Import progress polling, dropzone preview & step flow
   ========================================================================== */

// Import progress polling.
//
// The catalogue import prepares in the background because AI enrichment turns a
// large file into minutes of work. This keeps the panel honest while that runs
// and reloads into the review the moment it finishes, so nobody has to know to
// press refresh.
function initImportProgress() {
  const panel = document.getElementById('import-progress');
  if (!panel) return;

  const sessionID = panel.dataset.session;
  if (!sessionID) return;

  const fill = document.getElementById('import-progress-fill');
  const message = document.getElementById('import-progress-message');
  const count = document.getElementById('import-progress-count');

  // Two seconds is faster than the phases change and slow enough that a
  // half-hour import is a few hundred requests, not tens of thousands.
  const POLL_MS = 2000;
  let stopped = false;

  async function poll() {
    if (stopped) return;
    try {
      const res = await fetch(`/admin/products/import/${sessionID}/progress`, {
        headers: { 'Accept': 'application/json' },
        cache: 'no-store',
      });
      if (!res.ok) throw new Error(`progress ${res.status}`);
      const data = await res.json();

      if (message && data.message) message.textContent = data.message;
      if (count) {
        count.textContent = data.total > 0
          ? `${data.current.toLocaleString('en')} / ${data.total.toLocaleString('en')}`
          : '';
      }
      if (fill) {
        if (typeof data.percent === 'number' && data.percent >= 0) {
          fill.classList.remove('is-indeterminate');
          fill.style.width = `${data.percent}%`;
        } else {
          fill.classList.add('is-indeterminate');
          fill.style.width = '';
        }
      }

      if (data.done) {
        stopped = true;
        // Reload rather than patch the DOM: the finished page is a different
        // page — structure, counts, per-row table — and the server already
        // knows how to render it.
        if (window.dawaRefresh) { window.dawaRefresh(); } else { window.location.reload(); }
        return;
      }
    } catch (err) {
      // A dropped poll is not worth surfacing; the next one will catch up. Only
      // a run that never reports again leaves the panel where it was, and the
      // page explains that the results appear automatically.
      console.debug('import progress poll failed', err);
    }
    window.setTimeout(poll, POLL_MS);
  }

  window.setTimeout(poll, POLL_MS);
}

// Show the chosen file's name in the upload drop zone, so the admin can see
// what they picked before committing to a long run.
function initImportDropZone() {
  const input = document.getElementById('import-file-input');
  const title = document.getElementById('import-drop-title');
  const drop = document.getElementById('import-drop');
  if (!input || !title || !drop) return;

  input.addEventListener('change', () => {
    const file = input.files && input.files[0];
    title.textContent = file ? file.name : 'اختر الملف أو اسحبه إلى هنا';
  });

  ['dragenter', 'dragover'].forEach((evt) => {
    drop.addEventListener(evt, (e) => {
      e.preventDefault();
      drop.classList.add('is-over');
    });
  });
  ['dragleave', 'drop'].forEach((evt) => {
    drop.addEventListener(evt, (e) => {
      e.preventDefault();
      drop.classList.remove('is-over');
    });
  });
  drop.addEventListener('drop', (e) => {
    if (e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files.length) {
      input.files = e.dataTransfer.files;
      input.dispatchEvent(new Event('change'));
    }
  });
}

document.addEventListener('DOMContentLoaded', () => {
  initImportProgress();
  initImportDropZone();
});
