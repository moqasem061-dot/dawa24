/*
 * The upload half of the progress story.
 *
 * import-progress.js drives a bar from a SERVER-reported percentage: it is what
 * a background import run uses, and it is honest about the fact that it cannot
 * see inside the work. This drives the part the browser can measure exactly —
 * the bytes leaving the machine — and it exists because that is the part that
 * was taking the longest and reporting nothing at all.
 *
 * What a vendor used to see when they dropped ten price lists on the compare
 * tool: the button greyed out, the label changed to "جاري رفع ومعالجة...", and
 * then nothing moved for a minute and a half. There was no way to tell an
 * upload in progress from a dead connection, so people pressed refresh — which
 * abandoned the upload and started it again.
 *
 * Two phases, shown as what they are:
 *
 *   Uploading   xhr.upload.onprogress gives bytes sent against bytes total.
 *               This is a real percentage and it is presented as one.
 *   Processing  The browser cannot see the server parsing and matching. The
 *               bar goes indeterminate rather than inventing a figure, and the
 *               caption says what is happening.
 *
 * Nothing here claims progress it does not have. That is the whole design rule.
 */
(function () {
	'use strict';

	// Where the uploading phase ends. The processing that follows gets the rest,
	// so the bar never rewinds when it changes phase.
	var UPLOAD_BAND_END = 70;

	function el(id) { return document.getElementById(id); }

	/**
	 * UploadProgress drives one upload-progress dialog.
	 *
	 * modalId is the id given to components.UploadProgressModal; every element
	 * inside it is addressed by that prefix.
	 */
	function UploadProgress(modalId) {
		this.modal = el(modalId);
		this.fill = el(modalId + '-fill');
		this.caption = el(modalId + '-caption');
		this.percent = el(modalId + '-percent');
		this.count = el(modalId + '-count');
		this.detail = el(modalId + '-detail');
		this.files = el(modalId + '-files');
		this.note = el(modalId + '-note');
		this.shown = 0;
	}

	/** open shows the dialog and locks it while work is in flight. */
	UploadProgress.prototype.open = function () {
		if (!this.modal) return;
		this.modal.classList.add('is-busy');
		if (typeof this.modal.showModal === 'function' && !this.modal.open) {
			this.modal.showModal();
		} else {
			this.modal.setAttribute('open', '');
		}
	};

	UploadProgress.prototype.close = function () {
		if (!this.modal) return;
		this.modal.classList.remove('is-busy');
		if (typeof this.modal.close === 'function' && this.modal.open) {
			this.modal.close();
		} else {
			this.modal.removeAttribute('open');
		}
	};

	UploadProgress.prototype.setCaption = function (text) {
		if (this.caption && text) this.caption.textContent = text;
	};

	UploadProgress.prototype.setDetail = function (text) {
		if (this.detail) this.detail.textContent = text || '';
	};

	UploadProgress.prototype.setCount = function (text) {
		if (this.count) this.count.textContent = text || '';
	};

	/** setPercent renders a measured figure. Never goes backwards. */
	UploadProgress.prototype.setPercent = function (value) {
		var v = Math.max(0, Math.min(100, Math.round(value)));
		if (v < this.shown) v = this.shown;
		this.shown = v;
		if (this.fill) {
			this.fill.classList.remove('is-indeterminate');
			this.fill.style.width = v + '%';
		}
		if (this.percent) this.percent.textContent = v + '%';
		var track = this.fill && this.fill.parentNode;
		if (track && track.setAttribute) track.setAttribute('aria-valuenow', String(v));
	};

	/**
	 * setIndeterminate is for the phase the browser cannot measure.
	 *
	 * The inline width is CLEARED rather than overridden, so the stylesheet's
	 * sweep animation has nothing to fight — an inline style would otherwise
	 * beat it and the bar would sit still at whatever figure it last showed.
	 */
	UploadProgress.prototype.setIndeterminate = function (caption) {
		if (this.fill) {
			this.fill.style.width = '';
			this.fill.classList.add('is-indeterminate');
		}
		if (this.percent) this.percent.textContent = '';
		this.setCaption(caption);
	};

	/** fail stops the bar where it got to, so the reader can see how far it went. */
	UploadProgress.prototype.fail = function (message) {
		if (this.fill) {
			this.fill.classList.remove('is-indeterminate');
			this.fill.classList.add('is-failed');
			this.fill.style.width = Math.max(this.shown, 6) + '%';
		}
		this.setCaption(message || 'تعذّر إكمال العملية.');
		if (this.note) this.note.textContent = 'يمكنك إغلاق النافذة والمحاولة مرة أخرى.';
		if (this.modal) this.modal.classList.remove('is-busy');
	};

	/**
	 * setFiles renders the per-file list.
	 *
	 * A single bar answers "how far along"; ten files also need "which one".
	 * entries: [{ name, state }] where state is 'pending' | 'done' | 'failed'
	 * | 'skipped'.
	 */
	UploadProgress.prototype.setFiles = function (entries) {
		if (!this.files) return;
		this.files.innerHTML = '';
		(entries || []).forEach(function (entry) {
			var li = document.createElement('li');
			li.className = 'upload-progress-file is-' + (entry.state || 'pending');

			var name = document.createElement('span');
			name.className = 'upload-progress-file-name';
			name.textContent = entry.name;
			name.title = entry.name;

			var state = document.createElement('span');
			state.className = 'upload-progress-file-state';
			state.textContent = entry.label || '';

			li.appendChild(name);
			li.appendChild(state);
			this.files.appendChild(li);
		}, this);
	};

	/**
	 * submit uploads a form over XHR, reporting real progress, and then follows
	 * wherever the server sends the browser.
	 *
	 * The form is NOT submitted normally. A normal submit gives the browser no
	 * way to report how far the transfer has got — which is exactly the
	 * information the person watching wants — and no way to keep the page alive
	 * while it happens.
	 *
	 * opts:
	 *   uploadingCaption  shown while bytes are moving
	 *   processingCaption shown once the server has them
	 *   files             [{name}] to list, in the order they were selected
	 *   onDone            called with the final URL before navigation
	 */
	UploadProgress.prototype.submit = function (form, opts) {
		var self = this;
		opts = opts || {};

		var data = new FormData(form);
		var xhr = new XMLHttpRequest();
		xhr.open(form.method || 'POST', form.action, true);
		xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');

		var csrfCookie = document.cookie.match(/(?:^|;\s*)dawa_csrf=([^;]*)/);
		if (csrfCookie && csrfCookie[1]) {
			xhr.setRequestHeader('X-CSRF-Token', decodeURIComponent(csrfCookie[1]));
		}

		if (opts.files && opts.files.length) {
			self.setFiles(opts.files.map(function (f) {
				return { name: f.name, state: 'pending', label: 'في الانتظار' };
			}));
		}

		this.open();
		this.setCaption(opts.uploadingCaption || 'جارٍ رفع الملفات…');
		this.setPercent(0);

		xhr.upload.onprogress = function (e) {
			if (!e.lengthComputable) {
				self.setIndeterminate(opts.uploadingCaption || 'جارٍ رفع الملفات…');
				return;
			}
			var fraction = e.loaded / e.total;
			self.setPercent(fraction * UPLOAD_BAND_END);
			self.setCount(formatBytes(e.loaded) + ' / ' + formatBytes(e.total));
		};

		// The bytes are gone; everything after this happens on the server and
		// the browser cannot see any of it. Say so rather than pretending.
		xhr.upload.onload = function () {
			self.setCount('');
			self.setIndeterminate(opts.processingCaption || 'تم الرفع. جارٍ قراءة الملفات ومطابقة الأصناف…');
			self.setDetail('قد تستغرق المعالجة دقيقة أو أكثر حسب حجم الكشوف.');
			if (opts.files && opts.files.length) {
				self.setFiles(opts.files.map(function (f) {
					return { name: f.name, state: 'done', label: 'تم الرفع' };
				}));
			}
		};

		xhr.onload = function () {
			if (xhr.status >= 200 && xhr.status < 400) {
				self.setPercent(100);
				self.setCaption('اكتملت المعالجة');
				var target = xhr.responseURL || form.action;
				if (opts.onDone) opts.onDone(target);
				window.location.assign(target);
				return;
			}
			self.fail('تعذّر رفع الملفات (رمز ' + xhr.status + '). يرجى المحاولة مرة أخرى.');
		};

		xhr.onerror = function () {
			self.fail('انقطع الاتصال أثناء الرفع. تحقّق من الشبكة وحاول مرة أخرى.');
		};

		xhr.onabort = function () {
			self.fail('تم إلغاء الرفع.');
		};

		xhr.send(data);
		return xhr;
	};

	function formatBytes(n) {
		if (n < 1024) return n + ' B';
		if (n < 1024 * 1024) return (n / 1024).toFixed(0) + ' KB';
		return (n / (1024 * 1024)).toFixed(1) + ' MB';
	}

	window.UploadProgress = UploadProgress;
})();
