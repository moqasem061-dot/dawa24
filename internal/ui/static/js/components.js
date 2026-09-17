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
})();
