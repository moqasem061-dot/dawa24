/*
 * خصومات السوق العامة — the board's behaviour.
 *
 * The page is a shell (header and controls) and one results region,
 * #md-results. This script owns the filter form, so the generic filter
 * machinery (actions.js auto-submit, boost-nav.js) is switched off for it with
 * data-no-filter / data-no-boost.
 *
 *  - Typing never touches the board. It asks /market-discounts/suggest for a
 *    handful of lines and shows them in a listbox under the search box. The
 *    board changes only when the reader commits: Enter, "view all", or picking
 *    a suggestion (a product suggestion filters the board to that product).
 *  - A commit — and every sort, warehouse, range, page or page-size change —
 *    fetches only the results fragment and replaces the contents of
 *    #md-results. The header, the controls and the sidebar are never
 *    re-rendered. The URL is kept in step (pushState), so a result is
 *    shareable and the back button works.
 *  - The pager's next and previous pages are prefetched on hover or focus into
 *    a small in-memory map (6 entries, 30 s), so the click is usually instant.
 *  - List and grid are two CSS layouts of the same markup; switching is a class
 *    on .md-offers and costs no request.
 *
 * Both kinds of request are guarded the same way: the previous one is aborted,
 * and a response that arrives for a superseded request (a revision counter is
 * the final guard when aborting is not enough) is dropped.
 *
 * HTML only ever reaches the DOM through window.dawaHTML (security.js): the
 * responses are this application's own templ output.
 */
(function () {
	'use strict';

	var BASE = '/market-discounts';
	var SUGGEST_URL = BASE + '/suggest';
	var SUGGEST_DEBOUNCE_MS = 200;
	var SUGGEST_MIN_CHARS = 2;
	var PREFETCH_MAX = 6;
	var PREFETCH_TTL_MS = 30000;
	var VIEW_KEY = 'd24_market_view';

	if (window.__dawaMarketBoard && typeof window.__dawaMarketBoard.destroy === 'function') {
		window.__dawaMarketBoard.destroy();
		window.__dawaMarketBoard = null;
	}

	var root = document.getElementById('md-page');
	var form = document.getElementById('market-filter-form');
	var results = document.getElementById('md-results');
	var search = document.getElementById('market-search-input');
	var panel = document.getElementById('md-suggest');
	var list = document.getElementById('md-suggest-list');
	var stateEl = document.getElementById('md-suggest-state');
	if (!root || !form || !results || !search || !panel || !list || !stateEl) return;

	var msgFailed = root.getAttribute('data-msg-failed') || '';
	var msgLoading = root.getAttribute('data-msg-loading') || '';
	var msgSuggestError = root.getAttribute('data-msg-suggest-error') || '';
	var reduceMotion = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

	var view = root.getAttribute('data-view') === 'grid' ? 'grid' : 'list';
	var committedQuery = (search.value || '').trim();
	var disposers = [];

	function on(target, type, handler, options) {
		target.addEventListener(type, handler, options);
		disposers.push(function () { target.removeEventListener(type, handler, options); });
	}

	function runeLength(s) { return Array.from(s).length; }

	function safeStorage(fn) { try { return fn(); } catch (_) { return null; } }

	// -- URLs ---------------------------------------------------------------------

	function currentLang() {
		return new URLSearchParams(window.location.search).get('lang');
	}

	// buildURL turns the form into the board's URL for a page. Empty fields are
	// left out, and so is the default ordering, to keep shared links short.
	function buildURL(page) {
		var params = new URLSearchParams();
		new FormData(form).forEach(function (value, key) {
			value = String(value).trim();
			if (value !== '') params.append(key, value);
		});
		if (params.get('sort') === 'newest') params.delete('sort');
		params.set('view', view);
		var lang = currentLang();
		if (lang) params.set('lang', lang);
		if (page > 1) params.set('page', String(page));
		var qs = params.toString();
		return BASE + (qs ? '?' + qs : '');
	}

	// normalise accepts a link or option value from the server and returns the
	// board URL to fetch, with the layout the reader is looking at right now.
	function normalise(href) {
		var u;
		try { u = new URL(href, window.location.href); } catch (_) { return null; }
		if (u.origin !== window.location.origin || u.pathname !== BASE) return null;
		u.searchParams.set('view', view);
		var lang = currentLang();
		if (lang && !u.searchParams.has('lang')) u.searchParams.set('lang', lang);
		return u.pathname + (u.search || '');
	}

	// -- Results: fetch, prefetch, swap --------------------------------------------

	var prefetched = new Map();
	var revision = 0;
	var controller = null;

	function fetchFragment(key, signal) {
		var hit = prefetched.get(key);
		if (hit && Date.now() - hit.at < PREFETCH_TTL_MS && hit.html !== null) {
			return Promise.resolve(hit.html);
		}
		if (hit && hit.promise && Date.now() - hit.at < PREFETCH_TTL_MS) {
			return hit.promise;
		}
		return requestFragment(key, signal);
	}

	function requestFragment(key, signal) {
		return fetch(key, {
			credentials: 'same-origin',
			signal: signal,
			headers: { 'HX-Request': 'true', 'HX-Target': 'md-results', 'Accept': 'text/html' }
		}).then(function (res) {
			if (res.redirected) {
				// The session ended or the caller may no longer read the board:
				// the redirect is a whole page, not a fragment.
				window.location.assign(res.url);
				return new Promise(function () {});
			}
			if (!res.ok) throw new Error('status ' + res.status);
			return res.text();
		}).then(function (html) {
			if (html.indexOf('data-md-fragment') === -1) {
				window.location.assign(key);
				return new Promise(function () {});
			}
			return html;
		});
	}

	function remember(key, entry) {
		prefetched.delete(key);
		prefetched.set(key, entry);
		while (prefetched.size > PREFETCH_MAX) {
			prefetched.delete(prefetched.keys().next().value);
		}
	}

	function prefetch(href) {
		var key = normalise(href);
		if (!key) return;
		var hit = prefetched.get(key);
		if (hit && Date.now() - hit.at < PREFETCH_TTL_MS) return;
		var entry = { at: Date.now(), html: null, promise: null };
		entry.promise = requestFragment(key, undefined).then(function (html) {
			entry.html = html;
			return html;
		}, function (err) {
			prefetched.delete(key);
			throw err;
		});
		entry.promise.catch(function () {});
		remember(key, entry);
	}

	function setBusy(busy) {
		if (busy) {
			// Hold the region's height while the old results dim, so the page
			// does not jump under the reader's pointer.
			results.style.minHeight = results.offsetHeight + 'px';
			results.setAttribute('aria-busy', 'true');
		} else {
			results.setAttribute('aria-busy', 'false');
			window.requestAnimationFrame(function () { results.style.minHeight = ''; });
		}
	}

	function clearError() {
		var old = results.querySelector('.md-error');
		if (old) old.remove();
	}

	function showError() {
		clearError();
		var box = document.createElement('div');
		box.className = 'alert alert-danger md-error';
		box.setAttribute('role', 'alert');
		box.textContent = msgFailed;
		results.insertBefore(box, results.firstChild);
	}

	function applyViewClass() {
		var offers = results.querySelector('.md-offers');
		if (!offers) return;
		offers.classList.toggle('is-grid', view === 'grid');
		offers.classList.toggle('is-list', view !== 'grid');
	}

	function scrollToResults() {
		var rect = results.getBoundingClientRect();
		if (rect.top < 0) {
			results.scrollIntoView({ block: 'start', behavior: reduceMotion ? 'auto' : 'smooth' });
		}
	}

	// commit loads a board URL into #md-results. opts.push false is for the back
	// button; opts.scroll brings the results into view if they are above it;
	// opts.focus moves focus to the results heading (keyboard and screen-reader
	// users who followed a link or the pager).
	function commit(href, opts) {
		opts = opts || {};
		var key = normalise(href);
		if (!key) {
			window.location.assign(href);
			return;
		}
		closeSuggest();
		revision += 1;
		var mine = revision;
		if (controller) controller.abort();
		controller = typeof AbortController === 'function' ? new AbortController() : null;
		clearError();
		setBusy(true);

		fetchFragment(key, controller ? controller.signal : undefined).then(function (html) {
			if (mine !== revision || !results.isConnected) return;
			results.innerHTML = window.dawaHTML.fromServer(html);
			applyViewClass();
			syncFormFromURL(key);
			if (opts.push !== false && key !== window.location.pathname + window.location.search) {
				window.history.pushState({ mdBoard: true }, '', key);
			}
			setBusy(false);
			if (opts.scroll) scrollToResults();
			if (opts.focus) {
				var heading = document.getElementById('md-results-heading');
				if (heading) heading.focus({ preventScroll: true });
			}
		}).catch(function (err) {
			if (mine !== revision) return;
			if (err && err.name === 'AbortError') return;
			setBusy(false);
			showError();
		});
	}

	// syncFormFromURL makes the controls say what the board on screen is
	// filtered by, after a link, a chip or the back button changed it.
	function syncFormFromURL(key) {
		var params = new URL(key, window.location.href).searchParams;
		['q', 'supplier', 'date_from', 'date_to', 'min_price', 'max_price', 'min_discount', 'max_discount'].forEach(function (name) {
			var el = form.elements[name];
			// Never overwrite what the reader is typing right now.
			if (el && el !== document.activeElement) el.value = params.get(name) || '';
		});
		var sort = form.elements.sort;
		if (sort) sort.value = params.get('sort') || 'newest';
		var limit = form.elements.limit;
		if (limit && params.get('limit')) limit.value = params.get('limit');
		setProduct(params.get('product'));
		committedQuery = (search.value || '').trim();
		if (['date_from', 'date_to', 'min_price', 'max_price', 'min_discount', 'max_discount'].some(function (n) { return params.get(n); })) {
			setAdvanced(true);
		}
	}

	function setProduct(id) {
		var el = form.elements.product;
		if (id) {
			if (!el) {
				el = document.createElement('input');
				el.type = 'hidden';
				el.name = 'product';
				form.appendChild(el);
			}
			el.value = id;
		} else if (el) {
			el.remove();
		}
	}

	// A new typed search replaces a product filter: the product chip is removed
	// when the committed text differs from the last committed text.
	function dropProductIfSearchChanged() {
		if ((search.value || '').trim() !== committedQuery) setProduct(null);
	}

	function commitForm(opts) {
		dropProductIfSearchChanged();
		commit(buildURL(1), opts || {});
	}

	// -- Search combobox ----------------------------------------------------------

	var sugRevision = 0;
	var sugController = null;
	var sugTimer = null;
	var composing = false;
	var activeIndex = -1;

	function options() { return list.querySelectorAll('[role="option"]'); }

	function setExpanded(open) {
		panel.hidden = !open;
		search.setAttribute('aria-expanded', open ? 'true' : 'false');
		if (!open) {
			search.removeAttribute('aria-activedescendant');
			activeIndex = -1;
		}
	}

	function closeSuggest() {
		window.clearTimeout(sugTimer);
		sugRevision += 1;
		if (sugController) sugController.abort();
		sugController = null;
		list.removeAttribute('aria-busy');
		setExpanded(false);
	}

	function setActive(index) {
		var opts = options();
		activeIndex = index;
		for (var i = 0; i < opts.length; i++) opts[i].setAttribute('aria-selected', i === index ? 'true' : 'false');
		if (index >= 0 && opts[index]) {
			search.setAttribute('aria-activedescendant', opts[index].id);
			opts[index].scrollIntoView({ block: 'nearest' });
		} else {
			search.removeAttribute('aria-activedescendant');
		}
	}

	function showState(text, kind) {
		stateEl.hidden = !text;
		stateEl.textContent = text || '';
		stateEl.className = 'md-suggest-state' + (kind ? ' is-' + kind : '');
	}

	function renderSuggestions(html) {
		list.innerHTML = window.dawaHTML.fromServer(html);
		// A note ("no suggestions", "could not load") is not an option; the
		// listbox holds options only, so the note moves to the status line.
		var note = list.querySelector('[data-md-note]');
		var text = note ? note.textContent : '';
		var kind = note ? note.getAttribute('data-md-note') : '';
		if (note) note.remove();
		showState(text, kind);
		list.removeAttribute('aria-busy');
		setActive(-1);
		setExpanded(true);
	}

	function queueSuggest() {
		window.clearTimeout(sugTimer);
		var q = (search.value || '').trim().replace(/\s+/g, ' ');
		if (runeLength(q) < SUGGEST_MIN_CHARS) {
			closeSuggest();
			return;
		}
		// Loading looks different from "nothing found": the old lines stay but
		// the listbox is marked busy and the status line says so.
		list.setAttribute('aria-busy', 'true');
		showState(msgLoading, 'loading');
		setExpanded(true);
		sugTimer = window.setTimeout(function () { fetchSuggest(q); }, SUGGEST_DEBOUNCE_MS);
	}

	function fetchSuggest(q) {
		sugRevision += 1;
		var mine = sugRevision;
		if (sugController) sugController.abort();
		sugController = typeof AbortController === 'function' ? new AbortController() : null;
		var url = SUGGEST_URL + '?q=' + encodeURIComponent(q);
		var lang = currentLang();
		if (lang) url += '&lang=' + encodeURIComponent(lang);
		fetch(url, {
			credentials: 'same-origin',
			signal: sugController ? sugController.signal : undefined,
			headers: { 'X-Requested-With': 'XMLHttpRequest', 'Accept': 'text/html' }
		}).then(function (res) {
			if (!res.ok) throw new Error('status ' + res.status);
			return res.text();
		}).then(function (html) {
			if (mine !== sugRevision || document.activeElement !== search) return;
			if (html.trim() === '') { closeSuggest(); return; }
			renderSuggestions(html);
		}).catch(function (err) {
			if (mine !== sugRevision || (err && err.name === 'AbortError')) return;
			list.textContent = '';
			list.removeAttribute('aria-busy');
			showState(msgSuggestError, 'error');
			setExpanded(true);
		});
	}

	function selectOption(opt) {
		var kind = opt.getAttribute('data-md-kind');
		var text = opt.getAttribute('data-md-q') || '';
		if (kind === 'product') {
			setProduct(opt.getAttribute('data-md-product'));
			search.value = '';
		} else {
			if (kind === 'text') search.value = text;
			setProduct(null);
			if (kind === 'all') search.value = text;
		}
		closeSuggest();
		commit(buildURL(1), {});
	}

	on(search, 'input', function (evt) {
		if (composing || evt.isComposing) return;
		queueSuggest();
	});
	on(search, 'compositionstart', function () { composing = true; window.clearTimeout(sugTimer); });
	on(search, 'compositionend', function () { composing = false; queueSuggest(); });
	on(search, 'focus', function () {
		if (options().length > 0 && runeLength((search.value || '').trim()) >= SUGGEST_MIN_CHARS) setExpanded(true);
	});
	on(search, 'blur', function () { closeSuggest(); });

	on(search, 'keydown', function (evt) {
		if (evt.isComposing || composing) return;
		var opts = options();
		var open = !panel.hidden;
		switch (evt.key) {
			case 'ArrowDown':
				if (!open && opts.length > 0) { setExpanded(true); }
				if (opts.length === 0) return;
				evt.preventDefault();
				setActive(activeIndex + 1 >= opts.length ? 0 : activeIndex + 1);
				break;
			case 'ArrowUp':
				if (opts.length === 0) return;
				evt.preventDefault();
				if (!open) setExpanded(true);
				setActive(activeIndex <= 0 ? opts.length - 1 : activeIndex - 1);
				break;
			case 'Home':
				if (!open || activeIndex < 0) return;
				evt.preventDefault();
				setActive(0);
				break;
			case 'End':
				if (!open || activeIndex < 0) return;
				evt.preventDefault();
				setActive(opts.length - 1);
				break;
			case 'Enter':
				evt.preventDefault();
				if (open && activeIndex >= 0 && opts[activeIndex]) {
					selectOption(opts[activeIndex]);
				} else {
					closeSuggest();
					commitForm({});
				}
				break;
			case 'Escape':
				if (open) {
					evt.preventDefault();
					closeSuggest();
				}
				break;
			case 'Tab':
				closeSuggest();
				break;
			default:
		}
	});

	// The option is taken on mousedown with the default prevented: the input
	// must not blur first, because the blur closes the list and the click would
	// land on nothing (the same ordering combobox.js relies on).
	on(list, 'mousedown', function (evt) {
		var opt = evt.target.closest ? evt.target.closest('[role="option"]') : null;
		if (!opt) return;
		evt.preventDefault();
		selectOption(opt);
	});
	on(document, 'mousedown', function (evt) {
		if (panel.hidden) return;
		if (!(evt.target.closest && evt.target.closest('[data-md-combobox]'))) closeSuggest();
	});

	// Clearing the box (the native clear button, or Escape in a search field) is
	// a commit: the reader asked for the unfiltered board.
	on(search, 'search', function () {
		if ((search.value || '').trim() === '' && committedQuery !== '') commitForm({});
	});

	// -- Form, controls and links -------------------------------------------------

	on(form, 'submit', function (evt) {
		evt.preventDefault();
		closeSuggest();
		commitForm({});
	});

	on(form, 'change', function (evt) {
		var el = evt.target;
		if (!el || !el.name) return;
		if (el.name === 'sort' || el.name === 'supplier') commitForm({});
	});

	var advancedToggle = form.querySelector('[data-md-advanced-toggle]');
	var advancedPanel = document.getElementById('md-advanced');
	function setAdvanced(open) {
		if (!advancedToggle || !advancedPanel) return;
		advancedPanel.hidden = !open;
		advancedToggle.setAttribute('aria-expanded', open ? 'true' : 'false');
		advancedToggle.classList.toggle('active', open);
	}
	if (advancedToggle) {
		on(advancedToggle, 'click', function () { setAdvanced(advancedPanel.hidden); });
	}

	// Layout: a class on the offers container, remembered, and written into the
	// URL without adding a history entry. No request.
	function setView(next, remember) {
		view = next === 'grid' ? 'grid' : 'list';
		var hidden = form.elements.view;
		if (hidden) hidden.value = view;
		applyViewClass();
		root.setAttribute('data-view', view);
		form.querySelectorAll('[data-md-view]').forEach(function (btn) {
			var active = btn.getAttribute('data-md-view') === view;
			btn.classList.toggle('active', active);
			btn.setAttribute('aria-pressed', active ? 'true' : 'false');
		});
		if (remember) safeStorage(function () { window.localStorage.setItem(VIEW_KEY, view); });
		var u = new URL(window.location.href);
		u.searchParams.set('view', view);
		window.history.replaceState(window.history.state, '', u.pathname + u.search);
	}
	form.querySelectorAll('[data-md-view]').forEach(function (btn) {
		on(btn, 'click', function () { setView(btn.getAttribute('data-md-view'), true); });
	});
	(function initView() {
		if (new URLSearchParams(window.location.search).has('view')) return;
		var saved = safeStorage(function () { return window.localStorage.getItem(VIEW_KEY); });
		if ((saved === 'grid' || saved === 'list') && saved !== view) setView(saved, false);
	})();

	function isPlainClick(evt) {
		return evt.button === 0 && !evt.metaKey && !evt.ctrlKey && !evt.shiftKey && !evt.altKey;
	}

	// Capture on the region: pager links, chips and the empty state's link are
	// handled here, before htmx's boosted-link listener or the page-size
	// control's navigation can act on them.
	on(results, 'click', function (evt) {
		var link = evt.target.closest ? evt.target.closest('a[href]') : null;
		if (!link || !results.contains(link)) return;
		var inPager = !!link.closest('.b2b-pagination, [data-md-pager]');
		if (!inPager && !link.hasAttribute('data-md-commit')) return;
		if (!isPlainClick(evt)) return;
		var key = normalise(link.getAttribute('href'));
		if (!key) return;
		evt.preventDefault();
		evt.stopPropagation();
		commit(key, { scroll: true, focus: true });
	}, true);

	on(results, 'change', function (evt) {
		var sel = evt.target;
		if (!sel || !sel.matches || !sel.matches('select[data-navigate-on-change]')) return;
		evt.stopPropagation();
		var key = normalise(sel.value);
		if (key) commit(key, { scroll: true });
	}, true);

	function maybePrefetch(evt) {
		var link = evt.target.closest ? evt.target.closest('.b2b-pagination a[href], [data-md-pager] a[href]') : null;
		if (link && results.contains(link)) prefetch(link.getAttribute('href'));
	}
	on(results, 'mouseover', maybePrefetch);
	on(results, 'focusin', maybePrefetch);
	on(results, 'touchstart', maybePrefetch, { passive: true });

	on(window, 'popstate', function () {
		if (!results.isConnected || window.location.pathname !== BASE) return;
		commit(window.location.pathname + window.location.search, { push: false });
	});

	window.__dawaMarketBoard = {
		destroy: function () {
			window.clearTimeout(sugTimer);
			if (controller) controller.abort();
			if (sugController) sugController.abort();
			disposers.forEach(function (d) { d(); });
			disposers = [];
		}
	};
})();
