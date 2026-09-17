// actions.js -- declarative event wiring without inline JavaScript.
//
// The Content-Security-Policy sets script-src-attr 'none', so onclick="..."
// and friends never run. Markup describes the behaviour with data attributes
// instead:
//
//   data-on-<event>="openModal(this, 'x')"
//
//     Bound directly on the element (not delegated), so it runs in the same
//     phase and order an inline handler did, and event.stopPropagation()
//     still keeps ancestors from seeing the event. Elements added later --
//     htmx swaps, Alpine templates -- are bound by a MutationObserver.
//
//     The value is a call list, not JavaScript: `[return] name(args)` joined
//     by `;`. name is a function on window (a dotted path is allowed). Args
//     are literals (strings, numbers, true/false/null, JSON arrays/objects)
//     or this, event, this.value, this.checked, this.form, this.dataset.<key>.
//     `return f()` cancels the event when f returns false, as it did inline.
//     Built-in statements: event.preventDefault(), event.stopPropagation(),
//     onlySelf(), print(), history.back(), location.reload(), navigate(url),
//     closeWindow(), setValue(id, value), openDialog(id), closeDialog(id),
//     clickElement(id), focusSection(id, fallbackId, fieldSelector).
//     A bare `name` with data-args='[...]' calls name(...args).
//
//   data-confirm="message"      ask before a form submits or a button/link
//                               acts; Cancel stops it.
//   data-autosubmit             submit the element's form when it changes;
//   data-autosubmit="formId"    ...or the form with that id.
//   data-navigate-on-change     navigate to the selected value.
//   data-stop-click             stop clicks inside from reaching ancestors.
//
// Native functions are refused (no `data-on-click="fetch(...)"`): only
// functions the application defined can be called this way.
(function () {
  'use strict';

  var EVENTS = ['click', 'dblclick', 'change', 'input', 'submit', 'reset', 'keydown', 'keyup',
    'keypress', 'focus', 'blur', 'focusin', 'focusout', 'dragstart', 'dragover', 'dragenter',
    'dragleave', 'drop', 'paste', 'mouseover', 'mouseout', 'mouseenter', 'mouseleave',
    'load', 'error', 'scroll', 'toggle', 'close', 'contextmenu'];
  var ATTRS = EVENTS.map(function (e) { return 'data-on-' + e; });
  // Per-record geometry and colour. style="" is refused by the policy
  // (style-src-attr 'none'), so the server writes the value into a typed
  // attribute and it is applied here through the CSSOM, after validation:
  //   data-pct="42"          width: 42%   (clamped to 0..100)
  //   data-width-px="40"     width: 40px
  //   data-height="320px"    height (px, rem, em, vh or %)
  //   data-bg="#0284c7"      background-color (hex only)
  //   data-tint="#0284c7"    tinted badge: faint background, coloured text/border
  var STYLE_ATTRS = ['data-pct', 'data-width-px', 'data-height', 'data-bg', 'data-tint'];
  var HEX = /^#[0-9a-f]{3}(?:[0-9a-f]{3})?$/i;
  var LENGTH = /^\d+(?:\.\d+)?(?:px|rem|em|vh|%)$/;
  var SELECTOR = ATTRS.concat(STYLE_ATTRS).map(function (a) { return '[' + a + ']'; }).join(',') + ',[data-stop-click]';

  function applyStyle(el) {
    var v;
    if ((v = el.getAttribute('data-pct')) !== null) {
      var pct = Math.max(0, Math.min(100, parseFloat(v) || 0));
      el.style.width = pct + '%';
    }
    if ((v = el.getAttribute('data-width-px')) !== null && /^\d+$/.test(v)) {
      el.style.width = v + 'px';
    }
    if ((v = el.getAttribute('data-height')) !== null && LENGTH.test(v)) {
      el.style.height = v;
    }
    if ((v = el.getAttribute('data-bg')) !== null && HEX.test(v)) {
      el.style.backgroundColor = v;
    }
    if ((v = el.getAttribute('data-tint')) !== null && HEX.test(v)) {
      var full = v.length === 4 ? '#' + v[1] + v[1] + v[2] + v[2] + v[3] + v[3] : v;
      el.style.backgroundColor = full + '15';
      el.style.color = full;
      el.style.border = '1px solid ' + full + '40';
    }
  }

  var BUILTINS = {
    print: function () { window.print(); },
    'history.back': function () { window.history.back(); },
    'location.reload': function () {
      if (window.dawaRefresh) window.dawaRefresh(); else window.location.reload();
    },
    // navigate('/path'): same-origin navigation, in place when possible.
    navigate: function (url) {
      var u = new URL(String(url), window.location.href);
      if (u.origin !== window.location.origin) return;
      if (window.dawaNavigate) window.dawaNavigate(u.href); else window.location.assign(u.href);
    },
    // closeWindow(): close a window this page opened, else go back.
    closeWindow: function () {
      window.close();
      if (!window.closed) window.history.back();
    },
    // setValue('input-id', value): set an input and let its listeners know.
    setValue: function (id, v) {
      var el = document.getElementById(id);
      if (!el) return;
      el.value = v;
      el.dispatchEvent(new Event('input', { bubbles: true }));
      el.dispatchEvent(new Event('change', { bubbles: true }));
    },
    // openDialog('id') / closeDialog('id'): a <dialog> by id.
    openDialog: function (id) {
      var d = document.getElementById(id);
      if (d && typeof d.showModal === 'function' && !d.open) d.showModal();
    },
    closeDialog: function (id) {
      var d = document.getElementById(id);
      if (d && typeof d.close === 'function' && d.open) d.close();
    },
    // clickElement('id'): e.g. a drop zone that opens its hidden file input.
    clickElement: function (id) {
      var el = document.getElementById(id);
      if (el) el.click();
    },
    // focusSection('first-id', 'fallback-id', '.field'): scroll to the first
    // section that exists and focus the matching field inside it.
    focusSection: function (id, fallbackId, fieldSelector) {
      var sec = document.getElementById(id) || (fallbackId && document.getElementById(fallbackId));
      if (!sec) return;
      sec.scrollIntoView({ behavior: 'smooth' });
      var field = fieldSelector && sec.querySelector(fieldSelector);
      if (field) field.focus();
    }
  };
  var nativeCode = /\{\s*\[native code\]\s*\}\s*$/;
  var cache = Object.create(null);

  // -- Parsing -----------------------------------------------------------------
  function Scanner(src) { this.s = src; this.i = 0; }
  Scanner.prototype.peek = function () {
    while (this.i < this.s.length && /\s/.test(this.s[this.i])) this.i++;
    return this.s[this.i];
  };
  Scanner.prototype.eat = function (ch) {
    if (this.peek() !== ch) throw new Error('expected ' + ch + ' at ' + this.i);
    this.i++;
  };
  Scanner.prototype.word = function () {
    this.peek();
    var m = /^[A-Za-z_$][\w$]*(?:\.[A-Za-z_$][\w$]*)*/.exec(this.s.slice(this.i));
    if (!m) throw new Error('expected a name at ' + this.i);
    this.i += m[0].length;
    return m[0];
  };
  Scanner.prototype.string = function () {
    var q = this.s[this.i++];
    var out = '';
    while (this.i < this.s.length) {
      var c = this.s[this.i++];
      if (c === q) return out;
      if (c === '\\') {
        var n = this.s[this.i++];
        out += n === 'n' ? '\n' : n === 't' ? '\t' : n;
      } else {
        out += c;
      }
    }
    throw new Error('unterminated string');
  };
  Scanner.prototype.json = function () {
    var start = this.i;
    var depth = 0;
    while (this.i < this.s.length) {
      var c = this.s[this.i];
      if (c === '"') { this.string(); continue; }
      if (c === '{' || c === '[') depth++;
      if (c === '}' || c === ']') depth--;
      this.i++;
      if (depth === 0) return JSON.parse(this.s.slice(start, this.i));
    }
    throw new Error('unterminated literal');
  };
  Scanner.prototype.arg = function () {
    var c = this.peek();
    if (c === '"' || c === "'") return { lit: this.string() };
    if (c === '{' || c === '[') return { lit: this.json() };
    var num = /^-?\d+(?:\.\d+)?/.exec(this.s.slice(this.i));
    if (num) { this.i += num[0].length; return { lit: Number(num[0]) }; }
    var w = this.word();
    if (w === 'true' || w === 'false') return { lit: w === 'true' };
    if (w === 'null' || w === 'undefined') return { lit: null };
    if (w === 'this' || w === 'event' || /^this\.(value|checked|form|dataset\.[\w$]+)$/.test(w)) {
      return { ref: w };
    }
    throw new Error('unsupported argument ' + w);
  };

  function parse(src) {
    var sc = new Scanner(src);
    var calls = [];
    while (sc.peek() !== undefined) {
      var call = { ret: false, name: '', args: [], bare: false };
      var name = sc.word();
      if (name === 'return') { call.ret = true; name = sc.word(); }
      call.name = name.replace(/^window\./, '');
      if (sc.peek() === '(') {
        sc.eat('(');
        if (sc.peek() !== ')') {
          call.args.push(sc.arg());
          while (sc.peek() === ',') { sc.i++; call.args.push(sc.arg()); }
        }
        sc.eat(')');
      } else {
        call.bare = true;
      }
      calls.push(call);
      if (sc.peek() === ';') sc.i++;
      else if (sc.peek() !== undefined) throw new Error('unexpected ' + sc.peek() + ' at ' + sc.i);
    }
    return calls;
  }

  function compile(src) {
    if (!(src in cache)) {
      try {
        cache[src] = parse(src);
      } catch (err) {
        console.error('[actions] cannot parse "' + src + '":', err.message);
        cache[src] = [];
      }
    }
    return cache[src];
  }

  // -- Running -----------------------------------------------------------------
  function resolve(name) {
    if (Object.prototype.hasOwnProperty.call(BUILTINS, name)) return { fn: BUILTINS[name], owner: window };
    var parts = name.split('.');
    var owner = window;
    for (var i = 0; i < parts.length - 1 && owner != null; i++) {
      // Only application namespaces: no walking into prototypes or DOM objects.
      if (parts[i] === '__proto__' || parts[i] === 'prototype' || parts[i] === 'constructor') return null;
      owner = owner[parts[i]];
    }
    var fn = owner != null ? owner[parts[parts.length - 1]] : null;
    if (typeof fn !== 'function') return null;
    if (nativeCode.test(Function.prototype.toString.call(fn))) return null;
    return { fn: fn, owner: owner };
  }

  function value(arg, el, evt) {
    if (!arg.ref) return arg.lit;
    switch (arg.ref) {
      case 'this': return el;
      case 'event': return evt;
      case 'this.value': return el.value;
      case 'this.checked': return el.checked;
      case 'this.form': return el.form || el.closest('form');
      default: return el.dataset[arg.ref.slice('this.dataset.'.length)];
    }
  }

  function run(src, el, evt) {
    var calls = compile(src);
    for (var i = 0; i < calls.length; i++) {
      var c = calls[i];
      if (c.name === 'event.preventDefault') { evt.preventDefault(); continue; }
      if (c.name === 'event.stopPropagation') { evt.stopPropagation(); continue; }
      // onlySelf(): stop here unless the element itself was the target, e.g.
      // a backdrop that closes its dialog but not when its content is clicked.
      if (c.name === 'onlySelf') { if (evt.target !== el) return; continue; }
      var target = resolve(c.name);
      if (!target) {
        console.error('[actions] no application function named ' + c.name);
        continue;
      }
      var args;
      if (c.bare) {
        try { args = JSON.parse(el.getAttribute('data-args') || '[]'); } catch (_) { args = []; }
        if (!Array.isArray(args)) args = [args];
      } else {
        args = c.args.map(function (a) { return value(a, el, evt); });
      }
      var result = target.fn.apply(target.owner, args);
      if (c.ret && result === false) {
        evt.preventDefault();
        return;
      }
    }
  }

  // -- Binding -----------------------------------------------------------------
  var bound = new WeakMap();

  function handlerFor(type) {
    return function (evt) {
      var src = this.getAttribute('data-on-' + type);
      if (src) run(src, this, evt);
    };
  }
  var handlers = {};
  EVENTS.forEach(function (type) { handlers[type] = handlerFor(type); });
  function stopClick(evt) { evt.stopPropagation(); }

  function bind(el) {
    applyStyle(el);
    var done = bound.get(el);
    if (!done) { done = {}; bound.set(el, done); }
    for (var i = 0; i < EVENTS.length; i++) {
      var type = EVENTS[i];
      if (!done[type] && el.hasAttribute(ATTRS[i])) {
        el.addEventListener(type, handlers[type]);
        done[type] = true;
      }
    }
    if (!done.stop && el.hasAttribute('data-stop-click')) {
      el.addEventListener('click', stopClick);
      done.stop = true;
    }
  }

  function bindTree(root) {
    if (root.nodeType !== 1) return;
    if (root.matches(SELECTOR)) bind(root);
    var list = root.querySelectorAll(SELECTOR);
    for (var i = 0; i < list.length; i++) bind(list[i]);
  }

  new MutationObserver(function (records) {
    for (var i = 0; i < records.length; i++) {
      var r = records[i];
      if (r.type === 'attributes') {
        bind(r.target);
        continue;
      }
      for (var j = 0; j < r.addedNodes.length; j++) bindTree(r.addedNodes[j]);
    }
  }).observe(document.documentElement, {
    childList: true,
    subtree: true,
    attributes: true,
    attributeFilter: ATTRS.concat(STYLE_ATTRS, ['data-stop-click'])
  });
  bindTree(document.documentElement);

  // -- Built-in behaviours -----------------------------------------------------
  function confirmed(el, evt) {
    var msg = el.getAttribute('data-confirm');
    if (msg && !window.confirm(msg)) {
      evt.preventDefault();
      evt.stopImmediatePropagation();
      return false;
    }
    return true;
  }

  // Capture phase, so a cancelled confirmation stops the event before any
  // other handler (Alpine, htmx, data-on-click, the double-submit lock) acts.
  document.addEventListener('click', function (evt) {
    var t = evt.target && evt.target.nodeType === 1 ? evt.target : evt.target && evt.target.parentElement;
    if (!t) return;
    var el = t.closest('a[data-confirm], button[data-confirm], input[type=submit][data-confirm], input[type=button][data-confirm]');
    if (el && !el.disabled) confirmed(el, evt);
  }, true);

  document.addEventListener('submit', function (evt) {
    var form = evt.target;
    if (form && form.hasAttribute && form.hasAttribute('data-confirm')) confirmed(form, evt);
  }, true);

  document.addEventListener('change', function (evt) {
    var el = evt.target;
    if (!el || !el.getAttribute) return;
    if (el.hasAttribute('data-autosubmit')) {
      var id = el.getAttribute('data-autosubmit');
      var form = id ? document.getElementById(id) : (el.form || el.closest('form'));
      if (form) form.submit();
    }
    if (el.hasAttribute('data-navigate-on-change') && el.value) {
      if (window.dawaNavigate) window.dawaNavigate(el.value);
      else window.location.assign(el.value);
    }
  });

  // x-safe-html="expr": Alpine's x-html for the CSP build, which has no
  // x-html. The value always goes through the sanitiser in security.js, so
  // it can carry formatting but never behaviour.
  document.addEventListener('alpine:init', function () {
    var A = window.Alpine;

    // Magics: what templates used to reach for through window, Math, JSON
    // and friends, which the CSP build refuses to resolve in an expression.
    //   $t('key', 'fallback')   translated string
    //   $lang                   'ar' | 'en'
    //   $str(v) / $json(v)      String(v) / pretty JSON.stringify(v)
    //   $round/$min/$max(...)   Math.round / Math.min / Math.max
    //   $pct(part, whole)       whole-number percentage, 0 when whole is 0
    //   $csrf()                 the CSRF token (app.js)
    //   $copy(text)             clipboard + toast
    //   $setHash('tab')         replace the URL hash without a history entry
    //   $query('name')          a query-string parameter ('' when absent)
    //   $urlHash                location.hash without '#'
    //   $objectURL(file)        blob: preview URL for a chosen file
    //   $focusId('id')          focus an element after the next render
    //   $scrollToId('id')       smooth-scroll an element into view
    //   $resetLater('key', value, ms)  set a component property after a delay
    A.magic('t', function () {
      return function (key, fallback) {
        return typeof window.dawaT === 'function' ? window.dawaT(key, fallback) : fallback;
      };
    });
    A.magic('lang', function () {
      return window.__DAWA_LANG__ || document.documentElement.lang || 'ar';
    });
    A.magic('str', function () { return function (v) { return v == null ? '' : String(v); }; });
    A.magic('json', function () {
      return function (v) {
        try { return JSON.stringify(v, null, 2); } catch (_) { return ''; }
      };
    });
    A.magic('round', function () { return function (v) { return Math.round(Number(v) || 0); }; });
    A.magic('min', function () { return function () { return Math.min.apply(null, arguments); }; });
    A.magic('max', function () { return function () { return Math.max.apply(null, arguments); }; });
    A.magic('pct', function () {
      return function (part, whole) {
        whole = Number(whole) || 0;
        return whole ? Math.round((Number(part) || 0) / whole * 100) : 0;
      };
    });
    A.magic('csrf', function () {
      return function () {
        return typeof window.getCsrfToken === 'function' ? window.getCsrfToken() : '';
      };
    });
    A.magic('copy', function () {
      return function (text) {
        var done = function () {
          if (typeof window.showToast === 'function') {
            var msg = typeof window.dawaT === 'function' ? window.dawaT('common.copied', 'تم النسخ') : 'تم النسخ';
            window.showToast(msg, 'success');
          }
        };
        if (navigator.clipboard && text != null) navigator.clipboard.writeText(String(text)).then(done);
      };
    });
    A.magic('setHash', function () {
      return function (hash) {
        window.history.replaceState(null, '', '#' + String(hash).replace(/^#/, ''));
      };
    });
    A.magic('query', function () {
      return function (name) {
        return new URLSearchParams(window.location.search).get(name) || '';
      };
    });
    A.magic('urlHash', function () { return window.location.hash.replace(/^#/, ''); });
    // $here: path + query of the current page. $dropQuery('p'): remove a
    // one-shot query parameter from the address bar.
    A.magic('here', function () { return window.location.pathname + window.location.search; });
    A.magic('dropQuery', function () {
      return function (name) {
        var u = new URL(window.location.href);
        if (!u.searchParams.has(name)) return;
        u.searchParams.delete(name);
        window.history.replaceState(window.history.state, '', u.pathname + u.search + u.hash);
      };
    });
    // $printInvoice(url): print through the hidden iframe when the page has
    // one (printInvoiceDirectly), else open the printable page.
    A.magic('printInvoice', function () {
      return function (url) {
        var u = new URL(String(url), window.location.href);
        if (u.origin !== window.location.origin) return;
        if (typeof window.printInvoiceDirectly === 'function') window.printInvoiceDirectly(u.href);
        else window.open(u.href, '_blank', 'noopener');
      };
    });
    // $clearInput('id'): reset a file or text input by id.
    A.magic('clearInput', function () {
      return function (id) {
        var el = document.getElementById(id);
        if (el) el.value = '';
      };
    });
    A.magic('objectURL', function () {
      return function (file) { return file ? URL.createObjectURL(file) : ''; };
    });
    A.magic('focusId', function () {
      return function (id) {
        A.nextTick(function () {
          var el = document.getElementById(id);
          if (el) el.focus();
        });
      };
    });
    A.magic('scrollToId', function () {
      return function (id) {
        var el = document.getElementById(id);
        if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
      };
    });
    A.magic('resetLater', function (el) {
      return function (key, value, ms) {
        var data = A.$data(el);
        setTimeout(function () { data[key] = value; }, ms);
      };
    });

    A.directive('safe-html', function (el, directive, utils) {
      var read = utils.evaluateLater(directive.expression);
      utils.effect(function () {
        read(function (v) {
          el.innerHTML = window.dawaHTML.sanitize(v == null ? '' : String(v));
        });
      });
    });
  });

  window.dawaActions = { parse: parse };
})();
