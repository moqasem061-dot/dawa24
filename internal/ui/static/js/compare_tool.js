function toggleSelectAll(masterCheckbox) {
	const checkboxes = document.querySelectorAll('.file-checkbox');
	checkboxes.forEach(cb => cb.checked = masterCheckbox.checked);
}

function validateCompareSelection(event) {
	const checked = document.querySelectorAll('.file-checkbox:checked');
	if (checked.length === 0) {
		const all = document.querySelectorAll('.file-checkbox');
		if (all.length > 0) {
			all.forEach(cb => cb.checked = true);
			return true;
		}
		alert("يرجى رفع ملف كشف أسعار واحد على الأقل للمقارنة.");
		event.preventDefault();
		return false;
	}
	return true;
}

function triggerFileInput() {
	const input = document.getElementById('file-upload-input');
	if (input) {
		input.click();
	}
}

// el builds an element with a class and text content. Everything this file
// shows comes from uploaded price lists or file names, so it is never parsed
// as markup.
function compareEl(tag, cls, text) {
	const n = document.createElement(tag);
	if (cls) n.className = cls;
	if (text != null) n.textContent = text;
	return n;
}

function handleFileSelect(input) {
	const previewEl = document.getElementById('file-name-preview');
	if (!previewEl) return;

	previewEl.replaceChildren();
	if (!input.files || input.files.length === 0) return;

	if (input.files.length === 1) {
		const file = input.files[0];
		const size = (file.size / 1024 / 1024).toFixed(2);
		previewEl.append(compareEl('span', '', 'تم اختيار ملف:'), ' ',
			compareEl('strong', 'text-primary', file.name), ' (' + size + ' MB)');
		return;
	}

	let totalSize = 0;
	const names = compareEl('div', 'stack-sm');
	for (let i = 0; i < input.files.length; i++) {
		totalSize += input.files[i].size;
		names.append(compareEl('div', '', '• ' + input.files[i].name));
	}
	const totalMb = (totalSize / 1024 / 1024).toFixed(2);
	const box = compareEl('div', 'stack-sm');
	box.append(compareEl('div', 'stat-card-label',
		'تم اختيار (' + input.files.length + ') ملفات موردين (الإجمالي: ' + totalMb + ' MB):'), names);
	previewEl.append(box);
}

(function setupDropZone() {
	window.addEventListener('DOMContentLoaded', () => {
		const dropZone = document.getElementById('drop-zone-box');
		const fileInput = document.getElementById('file-upload-input');
		if (!dropZone || !fileInput) return;

		['dragenter', 'dragover'].forEach(eventName => {
			dropZone.addEventListener(eventName, (e) => {
				e.preventDefault();
				e.stopPropagation();
				dropZone.style.borderColor = 'var(--accent)';
				dropZone.style.background = 'var(--surface-raised)';
			}, false);
		});

		['dragleave', 'drop'].forEach(eventName => {
			dropZone.addEventListener(eventName, (e) => {
				e.preventDefault();
				e.stopPropagation();
				dropZone.style.borderColor = '';
				dropZone.style.background = '';
			}, false);
		});

		dropZone.addEventListener('drop', (e) => {
			const dt = e.dataTransfer;
			const files = dt.files;
			if (files && files.length > 0) {
				fileInput.files = files;
				handleFileSelect(fileInput);
			}
		}, false);
	});
})();

/*
 * The batch upload, with the two things it never had: a measured progress bar,
 * and a limit that takes what fits instead of refusing everything.
 *
 * It used to grey the button out and submit the form normally. A normal submit
 * gives the browser no way to report how far the transfer has got, so ten price
 * lists went up over a minute and a half with the page showing nothing - which
 * looks exactly like a dead connection, and people reloaded, which abandoned
 * the upload and started it over.
 */
function handleUploadSubmit(event) {
	const form = document.getElementById('compare-upload-form');
	const fileInput = document.getElementById('file-upload-input');
	const dropZone = document.getElementById('drop-zone-box');

	if (!fileInput || !fileInput.files || fileInput.files.length === 0) {
		alert("يرجى اختيار ملف Excel أو CSV واحد على الأقل للرفع والمعالجة.");
		event.preventDefault();
		triggerFileInput();
		return false;
	}

	event.preventDefault();

	let selected = Array.from(fileInput.files);
	let skipped = [];

	// Bulk import batch limit: allows uploading batches of 80+ files (up to 100 files at once).
	// File Center storage quota is enforced authoritatively on the server to keep active files within the plan.
	if (dropZone) {
		const batchLimit = parseInt(dropZone.dataset.batchLimit || '100', 10);
		if (batchLimit > 0 && selected.length > batchLimit) {
			skipped = selected.slice(batchLimit);
			selected = selected.slice(0, batchLimit);
		}
	}

	// Hand the trimmed selection back to the input so FormData sends exactly
	// what the dialog says it is sending.
	if (skipped.length && typeof DataTransfer === 'function') {
		const keep = new DataTransfer();
		selected.forEach((f) => keep.items.add(f));
		fileInput.files = keep.files;
	}

	const btn = document.getElementById('upload-submit-btn');
	const btnText = document.getElementById('upload-btn-text');
	if (btn && btnText) {
		btn.disabled = true;
		btn.style.opacity = '0.75';
		btnText.textContent = 'جارٍ رفع (' + selected.length + ') كشوف…';
	}

	// No dialog available (an older browser, or the component was not rendered):
	// fall back to the ordinary submit rather than doing nothing at all.
	if (typeof window.UploadProgress !== 'function' || !document.getElementById('compare-upload-progress')) {
		form.submit();
		return false;
	}

	const bar = new window.UploadProgress('compare-upload-progress');
	const listed = selected.map((f) => ({ name: f.name }))
		.concat(skipped.map((f) => ({ name: f.name, state: 'skipped', label: 'يتجاوز حد الباقة' })));

	bar.submit(form, {
		files: listed,
		uploadingCaption: 'جارٍ رفع (' + selected.length + ') من كشوف الموردين…',
		processingCaption: 'تم الرفع. جارٍ قراءة الكشوف ومطابقة الأصناف بالكتالوج…',
	});

	if (skipped.length) {
		bar.setDetail(skipped.length + ' ملف لم يُرفع لتجاوز حد الباقة');
	}
	return false;
}

function getCompareReturnUrl() {
	if (window.location.pathname.includes('/admin/organizations/import/')) {
		return window.location.pathname;
	}
	return '/compare/tool';
}

function openRenameModal(fileId, currentName) {
	const modal = document.getElementById('rename-file-modal');
	const form = document.getElementById('rename-file-form');
	const input = document.getElementById('rename-supplier-input');
	if (modal && form && input) {
		form.action = '/compare/files/' + encodeURIComponent(fileId) + '/rename';
		input.value = currentName || '';
		let retInput = form.querySelector('input[name="return_url"]');
		if (!retInput) {
			retInput = document.createElement('input');
			retInput.type = 'hidden';
			retInput.name = 'return_url';
			form.appendChild(retInput);
		}
		retInput.value = getCompareReturnUrl();
		if (typeof modal.showModal === 'function' && !modal.open) {
			modal.showModal();
		} else {
			modal.classList.remove('d-none');
			modal.classList.add('d-flex');
		}
		setTimeout(() => input.focus(), 50);
	}
}

function closeRenameModal() {
	const modal = document.getElementById('rename-file-modal');
	if (modal) {
		if (typeof modal.close === 'function' && modal.open) {
			modal.close();
		} else {
			modal.classList.add('d-none');
			modal.classList.remove('d-flex');
		}
	}
}

function deleteFileConfirm(id) {
	if (confirm("هل أنت متأكد من حذف هذا الملف نهائياً من مركز الملفات؟")) {
		const form = document.createElement('form');
		form.method = 'POST';
		form.action = '/compare/files/' + encodeURIComponent(id) + '/delete';
		const retInput = document.createElement('input');
		retInput.type = 'hidden';
		retInput.name = 'return_url';
		retInput.value = getCompareReturnUrl();
		form.appendChild(retInput);
		document.body.appendChild(form);
		form.submit();
	}
}

let searchTimeout = null;

function setSearchMessage(container, cls, text) {
	container.replaceChildren(compareEl('div', cls, text));
}

// renderSearchItem builds one result card. Names, suppliers and prices come
// from uploaded price lists, so they are set as text.
function renderSearchItem(item) {
	const card = compareEl('div', 'comparison-card card p-4 mb-3');
	const top = compareEl('div', 'd-flex justify-between items-start flex-wrap gap-3');
	const info = compareEl('div', 'stack-xs');
	const titleRow = compareEl('div', 'd-flex items-center gap-2');
	titleRow.append(compareEl('strong', 'product-title font-bold text-base', item.product_name || 'صنف دوائي'));

	if (item.catalog_status === 'catalog_and_suppliers') {
		titleRow.append(compareEl('span', 'badge badge-emerald', 'معتمد بالكتالوج ومتوفر'));
	} else if (item.catalog_status === 'catalog_only') {
		titleRow.append(compareEl('span', 'badge badge-sky', 'مسجل بالكتالوج (غير متوفر بالكشوف)'));
	} else {
		titleRow.append(compareEl('span', 'badge badge-amber', 'صنف خاص بكشف المورد'));
	}
	info.append(titleRow);
	if (item.sku) {
		info.append(compareEl('span', 'tabular-nums text-xs text-secondary', 'كود: ' + item.sku));
	}
	top.append(info);

	if (item.best_net_price && parseFloat(item.best_net_price) > 0) {
		const price = compareEl('div', 'text-end');
		price.append(
			compareEl('div', 'tabular-nums', item.best_net_price + ' ج.م'),
			compareEl('div', 'stack-sm', (item.best_supplier || 'أفضل سعر') + ' (' + (item.best_discount || 0) + '%)'));
		top.append(price);
	} else {
		top.append(compareEl('div', 'stack-sm', 'لا توجد عروض أسعار مرفوعة'));
	}
	card.append(top);

	if (item.offers && Object.keys(item.offers).length > 0) {
		const offers = compareEl('div', 'offers-list d-flex flex-wrap gap-2 mt-2');
		for (const [sup, off] of Object.entries(item.offers)) {
			const badgeClass = sup === item.best_supplier ? 'badge-emerald' : 'badge-secondary';
			offers.append(compareEl('span', 'badge ' + badgeClass,
				sup + ': ' + (off.discount || 0) + '% (' + (off.price_after_discount || '--') + ' ج.م)'));
		}
		card.append(offers);
	}
	return card;
}

function filterSearchLocal(query) {
	clearTimeout(searchTimeout);
	const resultsContainer = document.getElementById('instant-search-results');
	if (!query || query.trim().length < 2) {
		setSearchMessage(resultsContainer, 'stack-sm', 'اكتب اسم الصنف للبحث المباشر عبر جميع كشوف الموردين المرفوعة ومقارنة الخصومات فورياً.');
		return;
	}

	setSearchMessage(resultsContainer, 'stack-sm', '⏳ جاري البحث عبر الكتالوج وكشوف الموردين...');

	searchTimeout = setTimeout(() => {
		const orgInput = document.querySelector('input[name="org_id"]');
		let searchUrl = '/compare/search?q=' + encodeURIComponent(query.trim());
		if (orgInput && orgInput.value) {
			searchUrl += '&org_id=' + encodeURIComponent(orgInput.value);
		}
		fetch(searchUrl, {
			headers: { 'Accept': 'application/json' }
		})
		.then(r => {
			if (!r.ok) throw new Error('Search failed');
			return r.json();
		})
		.then(data => {
			const items = data.items || [];
			if (items.length === 0) {
				setSearchMessage(resultsContainer, 'stack-sm', 'لم يتم العثور على أصناف مطابقة لـ "' + query + '".');
				return;
			}

			const summary = compareEl('div', 'stack-sm');
			const found = compareEl('span', '', 'تم العثور على ');
			found.append(compareEl('strong', '', String(items.length)), ' صنف');
			summary.append(found, compareEl('span', '',
				'بالكتالوج: ' + (data.in_catalog_count || 0) + ' | أصناف موردين: ' + (data.custom_items_count || 0)));

			resultsContainer.replaceChildren(summary, ...items.slice(0, 15).map(renderSearchItem));
		})
		.catch(err => {
			setSearchMessage(resultsContainer, 'stat-card-label', 'حدث خطأ أثناء البحث. يرجى المحاولة بكلمة بحث أخرى.');
		});
	}, 250);
}

let isModalLoading = false;

// A placeholder dialog shown while the mapping modal loads. Built from nodes:
// the only dynamic values are step counters.
function mappingLoadingDialog(title, subtitle, message) {
	const dialog = compareEl('dialog', 'modal');
	dialog.id = 'compare-mapping-modal-backdrop';
	dialog.setAttribute('open', '');
	const box = compareEl('div', 'modal-box modal-xl');
	const header = compareEl('div', 'modal-header');
	const heading = compareEl('div', 'd-flex items-center gap-2');
	heading.append(compareEl('span', 'text-primary font-bold', '⚙️'));
	const titles = compareEl('div');
	titles.append(compareEl('h3', 'modal-title', title));
	if (subtitle) titles.append(compareEl('p', 'text-xs text-muted mt-0.5 m-0', subtitle));
	heading.append(titles);

	const closeForm = compareEl('form', 'm-0');
	closeForm.method = 'dialog';
	const close = compareEl('button', 'modal-close', '✕');
	close.type = 'button';
	close.setAttribute('aria-label', 'إغلاق');
	close.addEventListener('click', closeMappingModal);
	closeForm.append(close);
	header.append(heading, closeForm);

	const body = compareEl('div', 'modal-body p-6 text-center');
	const wait = compareEl('div', 'py-8 text-muted');
	wait.append(compareEl('div', 'text-2xl mb-2', '⏳'), compareEl('div', 'font-bold text-sm', message));
	body.append(wait);
	box.append(header, body);
	dialog.append(box);
	return dialog;
}

function openSetupModal(fileId, queue, step, total) {
	if (isModalLoading) return;
	const root = document.getElementById('mapping-modal-root');
	if (!root) return;

	isModalLoading = true;
	step = parseInt(step, 10) || 1;
	total = parseInt(total, 10) || 1;
	queue = queue || '';

	const existingDialog = root.querySelector('dialog');
	const existingBox = existingDialog ? existingDialog.querySelector('.modal-box') : null;

	if (existingBox) {
		const submitBtn = existingBox.querySelector('#mapping-submit-btn');
		if (submitBtn) {
			submitBtn.disabled = true;
			submitBtn.textContent = '⏳ جاري الانتقال للملف التالي...';
		}
		const form = existingBox.querySelector('#compare-mapping-form');
		if (form) {
			form.classList.add('opacity-50', 'pointer-events-none');
		}
	} else {
		root.replaceChildren(mappingLoadingDialog(
			'معالج ضبط أعمدة الملفات (' + step + ' من ' + total + ')',
			'جاري قراءة بيانات الكشف وإعداد المعاينة الذكية...',
			'جاري قراءة أعمدة الملف وتحليل المحتوى...'));
	}

	fetch('/compare/files/' + encodeURIComponent(fileId) + '/mapping-modal?setup=1&queue=' + encodeURIComponent(queue) + '&step=' + step + '&total=' + total)
		.then(res => {
			if (!res.ok) throw new Error('فشل فتح معالج ضبط الأعمدة');
			return res.text();
		})
		.then(html => {
			isModalLoading = false;
			// Our own templ output: parsed through the Trusted Types policy.
			const doc = window.dawaHTML.parse(html);
			const newDialog = doc.querySelector('dialog');
			const currentDialog = root.querySelector('dialog');

			if (currentDialog && newDialog) {
				const newBox = newDialog.querySelector('.modal-box');
				const currentBox = currentDialog.querySelector('.modal-box');
				if (newBox && currentBox) {
					currentBox.replaceWith(newBox);
					return;
				}
			}

			root.innerHTML = window.dawaHTML.fromServer(html);
		})
		.catch(err => {
			isModalLoading = false;
			alert('تعذر فتح معالج ضبط الأعمدة: ' + err.message);
			closeMappingModal();
		});
}

function openMappingModal(fileId) {
	if (isModalLoading) return;
	const root = document.getElementById('mapping-modal-root');
	if (!root) return;

	isModalLoading = true;
	root.replaceChildren(mappingLoadingDialog(
		'تعيين أعمدة كشف المورد', '', 'جاري قراءة أعمدة الملف ومعاينتها...'));

	fetch('/compare/files/' + encodeURIComponent(fileId) + '/mapping-modal')
		.then(res => {
			if (!res.ok) throw new Error('فشل جلب نافذة تعيين الأعمدة');
			return res.text();
		})
		.then(html => {
			isModalLoading = false;
			root.innerHTML = window.dawaHTML.fromServer(html);
		})
		.catch(err => {
			isModalLoading = false;
			alert('تعذر فتح نافذة تعيين الأعمدة: ' + err.message);
			closeMappingModal();
		});
}

function submitMappingFormAsync(event) {
	event.preventDefault();
	const form = event.target;
	const submitBtn = document.getElementById('mapping-submit-btn');
	if (submitBtn) {
		submitBtn.disabled = true;
		submitBtn.style.opacity = '0.7';
		submitBtn.textContent = '⏳ جاري الحفظ والمعالجة...';
	}

	const formData = new FormData(form);

	fetch(form.action, {
		method: 'POST',
		body: formData,
		headers: {
			'Accept': 'application/json'
		}
	})
	.then(res => {
		if (!res.ok) throw new Error('حدث خطأ أثناء حفظ تعيين الأعمدة.');
		return res.json();
	})
	.then(data => {
		if (data.next_file_id && data.next_file_id > 0) {
			openSetupModal(data.next_file_id, data.remaining_queue, data.step, data.total);
		} else {
			closeMappingModal();
			window.location.href = getCompareReturnUrl() + '?notice=success&msg=' + encodeURIComponent('تم حفظ وتطبيق ضبط أعمدة كافة ملفات الموردين بنجاح.');
		}
	})
	.catch(err => {
		alert(err.message || 'تعذر حفظ تعيين الأعمدة.');
		if (submitBtn) {
			submitBtn.disabled = false;
			submitBtn.style.opacity = '1';
			submitBtn.textContent = 'حفظ وإعادة المحاولة';
		}
	});

	return false;
}

function handleSetupSkip(fileId, remainingQueue, step, total) {
	fileId = parseInt(fileId, 10);
	step = parseInt(step, 10);
	total = parseInt(total, 10);
	if (!confirm('هل أنت متأكد من تخطي هذا الملف؟ سيتم حذفه من المقارنة والانتقال للملف التالي.')) {
		return;
	}

	const root = document.getElementById('mapping-modal-root');
	const currentDialog = root ? root.querySelector('dialog') : null;
	if (currentDialog) {
		const form = currentDialog.querySelector('#compare-mapping-form');
		if (form) {
			form.classList.add('opacity-50', 'pointer-events-none');
		}
		const submitBtn = currentDialog.querySelector('#mapping-submit-btn');
		if (submitBtn) {
			submitBtn.disabled = true;
			submitBtn.textContent = '⏩ جاري تخطي الملف...';
		}
	}

	const formData = new FormData();
	formData.append('setup_queue', remainingQueue);
	formData.append('step', step);
	formData.append('total', total);

	fetch('/compare/files/' + fileId + '/skip', {
		method: 'POST',
		body: formData,
		headers: {
			'Accept': 'application/json'
		}
	})
	.then(res => {
		if (!res.ok) throw new Error('فشل تخطي الملف.');
		return res.json();
	})
	.then(data => {
		if (data.next_file_id && data.next_file_id > 0) {
			openSetupModal(data.next_file_id, data.remaining_queue, data.step, data.total);
		} else {
			closeMappingModal();
			window.location.href = getCompareReturnUrl() + '?notice=success&msg=' + encodeURIComponent('تم تخطي الملف والانتهاء من معالج الإعداد.');
		}
	})
	.catch(err => {
		alert(err.message || 'تعذر تخطي الملف.');
		closeMappingModal();
	});
}

function closeMappingModal() {
	const root = document.getElementById('mapping-modal-root');
	if (root) {
		const dialog = root.querySelector('dialog');
		if (dialog && typeof dialog.close === 'function') {
			try { dialog.close(); } catch(e) {}
		}
		root.replaceChildren();
	}
}

window.addEventListener('DOMContentLoaded', () => {
	const params = new URLSearchParams(window.location.search);
	const setupFile = params.get('setup_file');
	const setupQueue = params.get('setup_queue');
	const setupStep = parseInt(params.get('setup_step') || '1', 10);
	const setupTotal = parseInt(params.get('setup_total') || '1', 10);

	if (setupFile) {
		// Wait for the batch to be readable before offering a column mapping.
		//
		// The upload records the files and hands the parse to a goroutine that
		// outlives the request — that is what stops ten files from holding the
		// browser open for minutes. It also means the columns of these files
		// may not have been read yet, and a mapping wizard opened on an
		// unparsed file shows a preview of nothing and asks the pharmacy to map
		// columns that were never detected.
		whenStagingReady(setupQueue || setupFile, () => {
			openSetupModal(setupFile, setupQueue, setupStep, setupTotal);
		});
	} else {
		const openFileId = params.get('open_mapping') || params.get('mapping_file');
		if (openFileId) {
			openMappingModal(openFileId);
		}
	}
});

/*
 * whenStagingReady polls the batch's readiness and calls done() once every file
 * has left `processing`.
 *
 * It shows the shared upload dialog while it waits, in its indeterminate state:
 * the server can honestly say how many FILES are finished, and nothing
 * trustworthy about how far through a given file it is, so the bar reports the
 * former and does not invent the latter.
 *
 * Failure to reach the endpoint is not failure of the import. The files are
 * recorded and the parse is running regardless, so after a run of errors this
 * stops waiting and opens the wizard anyway — a wizard that may be early is a
 * better outcome than a dialog that never goes away.
 */
function whenStagingReady(ids, done) {
	const modalId = 'compare-upload-progress';
	const hasBar = typeof window.UploadProgress === 'function' && document.getElementById(modalId);
	let bar = null;
	let failures = 0;
	let shown = false;

	const finish = () => {
		if (bar) {
			bar.setPercent(100);
			bar.setCaption('اكتملت قراءة الكشوف');
			bar.close();
		}
		done();
	};

	const tick = () => {
		fetch('/compare/files/staging?ids=' + encodeURIComponent(ids), {
			headers: { Accept: 'application/json' },
		})
			.then((res) => {
				if (!res.ok) throw new Error('HTTP ' + res.status);
				return res.json();
			})
			.then((data) => {
				failures = 0;
				if (!data || data.ready) {
					finish();
					return;
				}
				if (hasBar && !shown) {
					shown = true;
					bar = new window.UploadProgress(modalId);
					bar.open();
				}
				if (bar) {
					bar.setCaption('جارٍ قراءة أصناف الكشوف…');
					bar.setPercent(data.percent || 0);
					bar.setCount((data.done || 0) + ' من ' + (data.total || 0));
					bar.setFiles((data.files || []).map((f) => ({
						name: f.name || ('#' + f.id),
						state: f.status === 'failed' ? 'failed' : (f.done ? 'done' : 'pending'),
						label: f.status === 'failed'
							? 'تعذّرت القراءة'
							: (f.done ? f.row_count + ' صنف' : 'جارٍ القراءة…'),
					})));
				}
				setTimeout(tick, 1000);
			})
			.catch(() => {
				failures++;
				if (failures >= 5) {
					finish();
					return;
				}
				setTimeout(tick, 2000);
			});
	};

	tick();
}
