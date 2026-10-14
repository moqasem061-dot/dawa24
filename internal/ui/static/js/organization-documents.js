function openDocViewerFromBtn(btn) {
	if (!btn) return;
	const docId = btn.getAttribute('data-doc-id');
	const title = btn.getAttribute('data-title') || '';
	const docType = btn.getAttribute('data-doctype') || '';
	const name = btn.getAttribute('data-name') || '';
	const status = btn.getAttribute('data-status') || '';
	const url = btn.getAttribute('data-url') || '';
	openDocViewer(docId, title, docType, name, status, url, btn.getAttribute('data-mime') || '');
}

function openReplaceDocModalFromBtn(btn) {
	if (!btn) return;
	const docId = btn.getAttribute('data-doc-id');
	const docType = btn.getAttribute('data-doctype') || '';
	const title = btn.getAttribute('data-title') || '';
	const name = btn.getAttribute('data-name') || '';
	openReplaceDocModal(docId, docType, title, name);
}

function openDocViewer(docId, title, docType, originalName, status, rawUrl, mimeType) {
	let viewUrl = '/documents/' + docId + '/view';
	let downloadUrl = '/documents/' + docId + '/download';
	const path = window.location.pathname || '';
	if (path.startsWith('/admin/organizations/')) {
		viewUrl = '/admin/documents/' + docId + '/view';
		downloadUrl = '/admin/documents/' + docId + '/download';
	} else if (path.startsWith('/vendor/documents')) {
		viewUrl = '/vendor/documents/' + docId + '/view';
		downloadUrl = '/vendor/documents/' + docId + '/download';
	} else if (path.startsWith('/customer/documents')) {
		viewUrl = '/customer/documents/' + docId + '/view';
		downloadUrl = '/customer/documents/' + docId + '/download';
	}

	const titleEl = document.getElementById('modal-doc-title');
	const filenameEl = document.getElementById('modal-doc-filename');
	const tabBtn = document.getElementById('modal-doc-tab');
	const downloadBtn = document.getElementById('modal-doc-download-btn');
	const iframe = document.getElementById('modal-doc-iframe');
	const imgContainer = document.getElementById('modal-doc-img-container');
	let img = document.getElementById('modal-doc-img');
	const loader = document.getElementById('modal-doc-loader');
	const errBox = document.getElementById('modal-doc-error');
	const errNewTab = document.getElementById('modal-doc-error-newtab');
	const errDownload = document.getElementById('modal-doc-error-download');
	const modal = document.getElementById('doc-viewer-modal');

	if (titleEl) titleEl.textContent = title || 'معاينة المستند';
	if (filenameEl) filenameEl.textContent = originalName || ('Document #' + docId);
	if (tabBtn) tabBtn.href = viewUrl;
	if (downloadBtn) downloadBtn.href = downloadUrl;
	if (errNewTab) errNewTab.href = viewUrl;
	if (errDownload) errDownload.href = downloadUrl;

	// Sniff image-vs-PDF from the original filename first, then the stored URL
	const sniffSrc = ((originalName || '') !== '' ? originalName : (rawUrl || '')).split('?')[0];
	const ext = (sniffSrc.split('.').pop() || '').toLowerCase();
	const isImage = (mimeType || '').startsWith('image/') || ['png', 'jpg', 'jpeg', 'webp', 'svg', 'gif', 'avif'].includes(ext);

	if (errBox) errBox.classList.add('hidden');
	if (loader) loader.classList.remove('hidden');

	if (isImage) {
		if (iframe) {
			iframe.onload = null;
			iframe.onerror = null;
			iframe.classList.add('hidden');
			iframe.src = 'about:blank';
		}
		if (img && imgContainer) {
			// Each request owns its media node, so a late response for a closed
			// document cannot change the next document's loading/error state.
			const nextImage = img.cloneNode(false);
			nextImage.removeAttribute('src');
			img.onload = null;
			img.onerror = null;
			img.replaceWith(nextImage);
			img = nextImage;
			imgContainer.classList.add('hidden');
			img.onload = function () {
				if (!img.isConnected || !modal?.open) return;
				if (!img.complete || img.naturalWidth === 0) return;
				if (loader) loader.classList.add('hidden');
				if (errBox) errBox.classList.add('hidden');
				imgContainer.classList.remove('hidden');
			};
			img.onerror = function () {
				if (!img.isConnected || !modal?.open) return;
				if (!img.hasAttribute('src') || !img.complete || img.naturalWidth > 0) return;
				if (loader) loader.classList.add('hidden');
				imgContainer.classList.add('hidden');
				// Images must decode as images. An iframe fallback can silently stay blank.
				if (errBox) errBox.classList.remove('hidden');
			};
			img.src = viewUrl;
		}
	} else {
		if (imgContainer) imgContainer.classList.add('hidden');
		if (img) {
			img.onload = null;
			img.onerror = null;
			img.removeAttribute('src');
		}
		if (iframe) {
			iframe.onload = function () {
				if (loader) loader.classList.add('hidden');
			};
			iframe.onerror = function () {
				if (loader) loader.classList.add('hidden');
				iframe.classList.add('hidden');
				if (errBox) errBox.classList.remove('hidden');
			};
			iframe.src = viewUrl;
			iframe.classList.remove('hidden');
		}
	}

	if (modal) {
		if (typeof modal.showModal === 'function') modal.showModal();
		else modal.setAttribute('open', '');
	}
}

function closeDocViewer() {
	const modal = document.getElementById('doc-viewer-modal');
	if (modal) {
		if (typeof modal.close === 'function') modal.close();
		else modal.removeAttribute('open');
	}
	clearDocViewer();
}

function clearDocViewer() {
	const modal = document.getElementById('doc-viewer-modal');
	if (modal && modal.open) return;
	const iframe = document.getElementById('modal-doc-iframe');
	const img = document.getElementById('modal-doc-img');
	const imgContainer = document.getElementById('modal-doc-img-container');
	const loader = document.getElementById('modal-doc-loader');
	const errBox = document.getElementById('modal-doc-error');
	if (iframe) {
		iframe.onload = null;
		iframe.onerror = null;
		iframe.src = 'about:blank';
		iframe.classList.add('hidden');
	}
	if (img) {
		img.onload = null;
		img.onerror = null;
		img.removeAttribute('src');
	}
	if (imgContainer) imgContainer.classList.add('hidden');
	if (loader) loader.classList.add('hidden');
	if (errBox) errBox.classList.add('hidden');
}

function openReplaceDocModal(docId, docType, titleAr, origName) {
	const modal = document.getElementById('replace-doc-modal');
	const titleEl = document.getElementById('replace-modal-title');
	const currFileEl = document.getElementById('replace-modal-current-file');
	const docTypeInput = document.getElementById('replace-doc-type');
	const existingIdInput = document.getElementById('replace-existing-id');
	const reasonInput = document.getElementById('replace-doc-reason');

	if (titleEl) titleEl.textContent = 'استبدال مستند: ' + (titleAr || '');
	if (currFileEl) currFileEl.textContent = origName || ('مستند #' + docId);
	if (docTypeInput) docTypeInput.value = docType || '';
	if (existingIdInput) existingIdInput.value = docId || '';
	if (reasonInput) reasonInput.value = '';

	if (modal) {
		if (typeof modal.showModal === 'function') modal.showModal();
		else modal.setAttribute('open', '');
	}
}

function closeReplaceDocModal() {
	const modal = document.getElementById('replace-doc-modal');
	if (modal) {
		if (typeof modal.close === 'function') modal.close();
		else modal.removeAttribute('open');
	}
}

// Delegation survives navigation swaps and cached history restoration.
document.addEventListener('click', function (event) {
  const modal = event.target;
  if (modal.id === 'doc-viewer-modal') closeDocViewer();
});
document.addEventListener('close', function (event) {
  if (event.target.id === 'doc-viewer-modal') clearDocViewer();
}, true);
