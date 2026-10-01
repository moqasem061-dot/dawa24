// components.js -- Alpine components shared by several screens.
//
// The CSP build of Alpine cannot read methods or getters from an x-data
// object literal, so components that need them are registered here and
// named from the template: x-data="dawaLinePager(12, 25)".
(function () {
  'use strict';

  // dawaLinePager pages a list of `total` rows client-side. pageSize -1 shows
  // everything. search is used by lists that also filter (the courier's item
  // checklist); others ignore it.
  window.dawaData('dawaLinePager', function (total, pageSize) {
    return {
      page: 1,
      pageSize: pageSize || 25,
      total: total || 0,
      search: '',
      get totalPages() {
        var size = this.pageSize > 0 ? this.pageSize : this.total;
        return Math.max(1, Math.ceil(this.total / (size || 1)));
      },
      get startIdx() {
        return this.pageSize === -1 ? 1 : (this.page - 1) * this.pageSize + 1;
      },
      get endIdx() {
        return this.pageSize === -1 ? this.total : Math.min(this.page * this.pageSize, this.total);
      },
      // onPage reports whether the row at index idx is on the current page.
      onPage: function (idx) {
        return this.pageSize === -1 || (idx >= (this.page - 1) * this.pageSize && idx < this.page * this.pageSize);
      },
      nextPage: function () { if (this.page < this.totalPages) this.page++; },
      prevPage: function () { if (this.page > 1) this.page--; },
      setPage: function (p) { if (p >= 1 && p <= this.totalPages) this.page = p; },
      changePageSize: function (s) {
        this.pageSize = parseInt(s, 10);
        this.page = 1;
      }
    };
  });

  // dawaColumnPreview shows sample values of an uploaded file's columns next
  // to the mapping selects. cols: [{ index, preview: [..] }].
  window.dawaData('dawaColumnPreview', function (cols) {
    return {
      cols: cols || [],
      sample: function (i) {
        var c = this.cols.find(function (col) { return String(col.index) === String(i); });
        return c && c.preview ? c.preview.join(' · ') : '';
      }
    };
  });

  // dawaMatchReviewKeyboard enables rapid keyboard-driven queue resolution:
  // j/k or arrows to move, Enter to expand, 1-5 to select, x to exclude.
  window.dawaData('dawaMatchReviewKeyboard', function () {
    return {
      focusedIndex: 0,
      rows: function () {
        return Array.from(document.querySelectorAll('tr.review-row, [data-review-row]'));
      },
      updateFocus: function (newIndex) {
        var list = this.rows();
        if (list.length === 0) return;
        if (newIndex < 0) newIndex = 0;
        if (newIndex >= list.length) newIndex = list.length - 1;
        this.focusedIndex = newIndex;
        list.forEach(function (r, i) {
          if (i === newIndex) {
            r.classList.add('ring-2', 'ring-primary-500', 'bg-primary-50/20');
            r.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
          } else {
            r.classList.remove('ring-2', 'ring-primary-500', 'bg-primary-50/20');
          }
        });
      },
      handleKey: function (e) {
        if (!e || !e.key) return;
        var tag = e.target && e.target.tagName;
        if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || (e.target && e.target.isContentEditable)) {
          return;
        }
        var key = e.key;
        if (key === 'j' || key === 'ArrowDown') {
          e.preventDefault();
          this.updateFocus(this.focusedIndex + 1);
        } else if (key === 'k' || key === 'ArrowUp') {
          e.preventDefault();
          this.updateFocus(this.focusedIndex - 1);
        } else if (key === 'Enter') {
          var list = this.rows();
          var row = list[this.focusedIndex];
          if (row) {
            var toggle = row.querySelector('[data-review-expand], button.review-expand-btn');
            if (toggle) {
              e.preventDefault();
              toggle.click();
            }
          }
        } else if (key >= '1' && key <= '5') {
          var candIdx = parseInt(key, 10) - 1;
          var list = this.rows();
          var row = list[this.focusedIndex];
          if (row) {
            var candButtons = Array.from(row.querySelectorAll('[data-review-candidate-select], button[data-candidate-index]'));
            if (candButtons[candIdx] && !candButtons[candIdx].disabled) {
              e.preventDefault();
              candButtons[candIdx].click();
            }
          }
        } else if (key === 'x' || key === 'X') {
          var list = this.rows();
          var row = list[this.focusedIndex];
          if (row) {
            var excludeBtn = row.querySelector('[data-review-exclude], button[data-action="exclude"]');
            if (excludeBtn && !excludeBtn.disabled) {
              e.preventDefault();
              excludeBtn.click();
            }
          }
        }
      }
    };
  });
})();
