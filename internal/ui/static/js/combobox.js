/*
 * One searchable dropdown, shared by every screen that has to pick a row out of
 * a list too long to scroll.
 *
 * There were four hand-rolled copies — the advertisement wizard's stock picker,
 * the variant modal's master-catalogue search, the registration city select and
 * the map picker's governorate select — and between them they got the same
 * things wrong:
 *
 *  1. The selection was lost on click. A dropdown closed by `@click.outside`
 *     closes on mousedown, before the option's own click handler ever runs, so
 *     on some browsers the click landed on nothing. Options here commit on
 *     `mousedown` with the default prevented, which is the one ordering that
 *     works everywhere: the value is taken before the input can blur.
 *
 *  2. There was no keyboard. A combobox that cannot be driven with the arrow
 *     keys is not usable without a mouse and is invisible to a screen reader.
 *     This one carries role="combobox"/"listbox"/"option", aria-expanded and
 *     aria-activedescendant, and handles Up, Down, Home, End, Enter, Escape and
 *     Tab.
 *
 *  3. The whole list was inlined into the page. The ads wizard serialised every
 *     in-stock variant into an x-data attribute and filtered it in the browser.
 *     A remote source is fetched on demand, debounced, with the previous
 *     request aborted, so typing quickly cannot deliver results out of order.
 *
 *  4. "No results" and "still loading" looked identical — an empty box.
 *
 * Two sources are supported. `options` is an inline array, for a list that is
 * genuinely short and constant (the governorates). `url` is a search endpoint
 * returning a JSON array; `{q}` in it is replaced by the encoded query, and any
 * other `{name}` by the current value of another combobox on the page that
 * declared that name — which is how the city list follows the governorate.
 */
(function () {
	'use strict';

	var DEBOUNCE_MS = 250;
	var MIN_REMOTE_CHARS = 2;

	// registry lets one combobox read another's selection, for chained lists.
	var registry = Object.create(null);
	window.dawaComboboxRegistry = registry;

	function normalize(s) {
		if (s === null || s === undefined) return '';
		// Arabic search has to ignore the letter forms people do not type:
		// alef with any hamza, the two yaas, the two haas, and the tatweel.
		return String(s)
			.toLowerCase()
			.replace(/[أإآٱ]/g, 'ا')
			.replace(/ى/g, 'ي')
			.replace(/ة/g, 'ه')
			.replace(/[ً-ْـ]/g, '')
			.trim();
	}

	function dawaCombobox(config) {
		return {
			// --- configuration -------------------------------------------------
			name: config.name || '',
			url: config.url || '',
			options: config.options || [],
			minChars: config.url ? (config.minChars || MIN_REMOTE_CHARS) : 0,
			dependsOn: config.dependsOn || '',
			allowClear: config.allowClear !== false,

			// --- state ---------------------------------------------------------
			query: config.selectedLabel || '',
			selected: config.selectedValue ? { id: config.selectedValue, label: config.selectedLabel || '' } : null,
			results: [],
			open: false,
			loading: false,
			failed: false,
			active: -1,
			_timer: null,
			_abort: null,
			_uid: 'cb-' + Math.random().toString(36).slice(2, 9),

			init() {
				if (this.name) registry[this.name] = this;
				if (!this.url && this.options.length) this.results = this.options.slice(0, 50);
			},

			destroy() {
				if (this.name && registry[this.name] === this) delete registry[this.name];
				this._cancel();
			},

			_cancel() {
				if (this._timer) { clearTimeout(this._timer); this._timer = null; }
				if (this._abort) { this._abort.abort(); this._abort = null; }
			},

			/** dependencyValue is the value this list is filtered by, if any. */
			dependencyValue() {
				var other = registry[this.dependsOn];
				return other && other.selected ? String(other.selected.id) : '';
			},

			/** Called by a parent list when its selection changes. */
			dependencyChanged() {
				this.clear(false);
				this.results = [];
			},

			optionId(i) { return this._uid + '-opt-' + i; },

			onFocus() {
				this.open = true;
				this.search();
			},

			onInput() {
				// Typing after a selection means the selection is being replaced;
				// the hidden field must not keep pointing at the old row.
				if (this.selected && this.query !== this.selected.label) {
					this.selected = null;
					this._emit();
				}
				this.open = true;
				this.search();
			},

			search() {
				this._cancel();
				this.failed = false;
				var q = this.query.trim();

				if (!this.url) {
					var nq = normalize(q);
					var dep = this.dependsOn ? this.dependencyValue() : '';
					this.results = this.options.filter(function (o) {
						if (dep && String(o.parent || '') !== dep) return false;
						if (!nq) return true;
						return normalize(o.label).indexOf(nq) !== -1 ||
							normalize(o.hint || '').indexOf(nq) !== -1;
					}).slice(0, 50);
					this.active = this.results.length ? 0 : -1;
					return;
				}

				if (q.length < this.minChars) {
					this.results = [];
					this.active = -1;
					this.loading = false;
					return;
				}

				var self = this;
				this.loading = true;
				this._timer = setTimeout(function () { self._fetch(q); }, DEBOUNCE_MS);
			},

			_fetch(q) {
				var self = this;
				var url = this.url
					.replace('{q}', encodeURIComponent(q))
					.replace(/\{([a-z_]+)\}/gi, function (_, key) {
						var other = registry[key];
						return other && other.selected ? encodeURIComponent(other.selected.id) : '';
					});

				// One request in flight. Without this a slow response for "pan"
				// can land after a fast one for "panadol" and overwrite it.
				this._abort = typeof AbortController === 'function' ? new AbortController() : null;
				fetch(url, {
					headers: { Accept: 'application/json' },
					signal: this._abort ? this._abort.signal : undefined
				})
					.then(function (res) {
						if (!res.ok) throw new Error('HTTP ' + res.status);
						return res.json();
					})
					.then(function (data) {
						self.loading = false;
						self.results = Array.isArray(data) ? data.slice(0, 50) : [];
						self.active = self.results.length ? 0 : -1;
					})
					.catch(function (err) {
						if (err && err.name === 'AbortError') return;
						self.loading = false;
						self.results = [];
						self.failed = true;
					});
			},

			/** choose commits an option. Called from mousedown, not click. */
			choose(item) {
				this.selected = { id: item.id, label: item.label, item: item };
				this.query = item.label;
				this.open = false;
				this.active = -1;
				this._emit();
			},

			clear(focus) {
				this.selected = null;
				this.query = '';
				this.results = this.url ? [] : this.options.slice(0, 50);
				this.active = -1;
				this._emit();
				if (focus !== false && this.$refs.input) this.$refs.input.focus();
			},

			_emit() {
				// Chained lists reset when their parent changes.
				var self = this;
				Object.keys(registry).forEach(function (key) {
					var other = registry[key];
					if (other !== self && other.dependsOn === self.name) other.dependencyChanged();
				});
				if (this.$refs.hidden) {
					this.$refs.hidden.dispatchEvent(new Event('change', { bubbles: true }));
				}
				this.$dispatch('combobox-change', {
					name: this.name,
					value: this.selected ? this.selected.id : '',
					item: this.selected ? this.selected.item : null
				});
			},

			move(delta) {
				if (!this.open) { this.open = true; this.search(); return; }
				if (!this.results.length) return;
				var next = this.active + delta;
				if (next < 0) next = this.results.length - 1;
				if (next >= this.results.length) next = 0;
				this.active = next;
				this._scrollActiveIntoView();
			},

			jump(to) {
				if (!this.results.length) return;
				this.active = to === 'end' ? this.results.length - 1 : 0;
				this._scrollActiveIntoView();
			},

			_scrollActiveIntoView() {
				var el = document.getElementById(this.optionId(this.active));
				if (el && typeof el.scrollIntoView === 'function') {
					el.scrollIntoView({ block: 'nearest' });
				}
			},

			commitActive() {
				if (this.open && this.active >= 0 && this.results[this.active]) {
					this.choose(this.results[this.active]);
					return true;
				}
				return false;
			},

			close() {
				this.open = false;
				this.active = -1;
				// An abandoned half-typed query must not look like a selection.
				if (this.selected) this.query = this.selected.label;
				else if (!this.allowClear) this.query = '';
			}
		};
	}

	window.dawaCombobox = dawaCombobox;

	/* Setting a combobox from outside it.
	 *
	 * The map picker moves the marker and then wants the governorate and city
	 * controls to follow. It used to do that by writing to a <select>'s value
	 * and reading a data-city-id off the matched <option> — which only worked
	 * because both were plain selects. This is the equivalent for a combobox,
	 * and it is the only sanctioned way in: writing the hidden input directly
	 * would leave the visible text and the component's own state disagreeing.
	 */
	window.dawaComboboxSet = function (name, id, label) {
		var cb = registry[name];
		if (!cb) return false;
		if (!id) { cb.clear(false); return true; }
		if (cb.selected && String(cb.selected.id) === String(id)) return true;

		// Prefer this combobox's own record of the option: the caller knows an
		// id and a label, the option knows which parent it belongs to.
		var known = null;
		for (var i = 0; i < cb.options.length; i++) {
			if (String(cb.options[i].id) === String(id)) { known = cb.options[i]; break; }
		}
		var item = known || { id: String(id), label: label || String(id) };

		// Set the parent first when there is one. Choosing a city while its
		// governorate is still blank leaves the two controls disagreeing, and
		// the server rejects that pair — so the map has to set both or neither.
		if (cb.dependsOn && item.parent) {
			var parent = registry[cb.dependsOn];
			if (parent && (!parent.selected || String(parent.selected.id) !== String(item.parent))) {
				var parentOption = null;
				for (var j = 0; j < parent.options.length; j++) {
					if (String(parent.options[j].id) === String(item.parent)) { parentOption = parent.options[j]; break; }
				}
				if (parentOption) parent.choose(parentOption);
			}
		}

		cb.choose(item);
		return true;
	};

	/* Reading one, for code that needs the current selection. */
	window.dawaComboboxValue = function (name) {
		var cb = registry[name];
		return cb && cb.selected ? cb.selected.id : '';
	};
})();
