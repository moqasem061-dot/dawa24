// security.js -- Trusted Types policies and the htmx swap gate.
//
// Loaded synchronously, first, from layouts/base.templ. The response's
// Content-Security-Policy carries `require-trusted-types-for 'script'`, so
// every DOM sink that can run code (innerHTML, DOMParser, script.src,
// script.textContent, ...) throws unless it is handed a Trusted Type. Only two
// policies may mint them (`trusted-types dawa-sanitize default`):
//
//   dawa-sanitize  Private to this file. Exposed only through window.dawaHTML.
//
//   default        What the browser calls when a library (htmx, Leaflet)
//                  hands a sink a plain string:
//                  - HTML is run through an allowlist sanitizer, except the
//                    exact server response htmx is swapping in right now,
//                    which is parsed as-is (it is our own templ output).
//                  - Script text is refused, except the text of a <script>
//                    in that same response that carried the response's own
//                    CSP nonce.
//                  - Script URLs must be same-origin.
//
// htmx copies swapped <script> elements and stamps them with the page's nonce
// (htmx.config.inlineScriptNonce). Unchecked, that would lend the nonce to a
// script injected into a response. The gate below therefore removes, before
// every swap, each <script> whose nonce is not the one the response's own
// policy names.
(function () {
  'use strict';

  var SERVER_SCRIPT_TYPES = { '': 1, 'text/javascript': 1, 'module': 1, 'application/javascript': 1 };

  // -- Allowlist sanitizer -----------------------------------------------------
  // For HTML that is not a server response: Leaflet's controls and anything a
  // page script builds from strings. No foreign content (svg/math) and no
  // raw-text elements, so serialising the cleaned tree and parsing it again
  // cannot change its meaning.
  var ALLOWED_TAGS = {};
  ('a abbr b bdi bdo blockquote br button caption cite code col colgroup dd del dfn div dl dt em ' +
   'figcaption figure h1 h2 h3 h4 h5 h6 hr i img ins kbd label li mark ol p pre q s samp small ' +
   'span strong sub sup table tbody td tfoot th thead time tr u ul').split(' ').forEach(function (t) {
    ALLOWED_TAGS[t] = 1;
  });
  // Elements removed together with their content; any other unknown element
  // is unwrapped and keeps its text.
  var DROP_WITH_CONTENT = {};
  ('script style template noscript iframe frame frameset object embed applet math ' +
   'textarea title xmp noembed noframes plaintext select option head meta link base form ' +
   'input dialog').split(' ').forEach(function (t) { DROP_WITH_CONTENT[t] = 1; });
  var ALLOWED_ATTRS = {};
  ('class id title alt role dir lang colspan rowspan scope headers width height datetime ' +
   'disabled hidden tabindex href src target rel type for name value download').split(' ').forEach(function (a) {
    ALLOWED_ATTRS[a] = 1;
  });
  // Icons. Shapes and presentation attributes only: no text, <style>,
  // <foreignObject>, <use> or links, which is what namespace-confusion and
  // SVG script tricks need.
  var SVG_NS = 'http://www.w3.org/2000/svg';
  var HTML_NS = 'http://www.w3.org/1999/xhtml';
  var SVG_TAGS = {};
  'svg g path circle ellipse line polyline polygon rect'.split(' ').forEach(function (t) {
    SVG_TAGS[t] = 1;
  });
  var SVG_ATTRS = {};
  ('class viewBox width height fill stroke stroke-width stroke-linecap stroke-linejoin ' +
   'fill-rule clip-rule d points x y x1 y1 x2 y2 cx cy r rx ry transform opacity ' +
   'fill-opacity stroke-opacity xmlns aria-hidden focusable role').split(' ').forEach(function (a) {
    SVG_ATTRS[a.toLowerCase()] = 1;
  });
  var SAFE_URL = /^(?:(?:https?|mailto|tel):|[^a-z]|[a-z0-9+.-]+(?:[^a-z0-9+.:-]|$))/i;
  var SAFE_IMG_DATA = /^data:image\/(?:png|gif|jpe?g|webp|avif);base64,[a-z0-9+\/=]+$/i;

  function cleanAttributes(el) {
    var tag = el.localName;
    var names = el.getAttributeNames();
    for (var i = 0; i < names.length; i++) {
      var name = names[i];
      var value = el.getAttribute(name);
      var keep;
      if (name.indexOf('aria-') === 0) {
        keep = true;
      } else if (name.indexOf('data-') === 0) {
        // data-on-* / data-autosubmit / data-confirm drive actions.js; built
        // HTML must not be able to wire behaviour.
        keep = !/^data-(on-|autosubmit|confirm|action)/.test(name);
      } else if (!ALLOWED_ATTRS[name]) {
        keep = false;
      } else if (name === 'href') {
        keep = tag === 'a' && SAFE_URL.test(value.replace(/[\u0000-\u0020]/g, ''));
      } else if (name === 'src') {
        var v = value.replace(/[\u0000-\u0020]/g, '');
        keep = tag === 'img' && (SAFE_IMG_DATA.test(v) || (!/^data:/i.test(v) && SAFE_URL.test(v)));
      } else if (name === 'target') {
        keep = value === '_blank';
        if (keep) el.setAttribute('rel', 'noopener noreferrer');
      } else if (name === 'type') {
        keep = tag === 'button' && /^(button|submit|reset)$/i.test(value);
      } else if (name === 'name') {
        // Named elements can clobber document properties.
        keep = false;
      } else if (name === 'id') {
        // Nor may an id shadow a document property or one of our globals.
        keep = !(value in document) && !Object.prototype.hasOwnProperty.call(window, value);
      } else {
        keep = true;
      }
      if (!keep) el.removeAttribute(name);
    }
    if (tag === 'button' && !el.hasAttribute('type')) el.setAttribute('type', 'button');
  }

  function cleanSVG(el) {
    var names = el.getAttributeNames();
    for (var i = 0; i < names.length; i++) {
      var n = names[i];
      var v = el.getAttribute(n);
      if (!SVG_ATTRS[n.toLowerCase()] || /url\s*\(|javascript:|[<>]/i.test(v)) el.removeAttribute(n);
    }
    var child = el.firstChild;
    while (child) {
      var next = child.nextSibling;
      if (child.nodeType === 1 && child.namespaceURI === SVG_NS && SVG_TAGS[child.localName] &&
          child.localName !== 'svg') {
        cleanSVG(child);
      } else {
        el.removeChild(child); // text, comments, anything else
      }
      child = next;
    }
  }

  function cleanTree(root) {
    var child = root.firstChild;
    while (child) {
      var next = child.nextSibling;
      if (child.nodeType === 1) {
        var tag = child.localName;
        if (tag === 'svg' && child.namespaceURI === SVG_NS && root.namespaceURI === HTML_NS) {
          cleanSVG(child);
        } else if (DROP_WITH_CONTENT[tag] || child.namespaceURI !== HTML_NS) {
          root.removeChild(child);
        } else if (!ALLOWED_TAGS[tag]) {
          cleanTree(child);
          while (child.firstChild) root.insertBefore(child.firstChild, child);
          root.removeChild(child);
        } else {
          cleanAttributes(child);
          cleanTree(child);
        }
      } else if (child.nodeType !== 3) {
        root.removeChild(child); // comments, processing instructions
      }
      child = next;
    }
  }

  var tt = window.trustedTypes;
  var raw = tt && tt.createPolicy ? tt.createPolicy('dawa-sanitize', {
    createHTML: function (s) { return s; },
    createScript: function (s) { return s; },
    createScriptURL: function (s) { return s; }
  }) : null;

  function trustedHTML(s) { return raw ? raw.createHTML(s) : s; }

  function parseDocument(html) {
    return new DOMParser().parseFromString(trustedHTML(String(html)), 'text/html');
  }

  // Table fragments parse only inside their table context: a bare <tr> in
  // <body> loses its tags. The wrapper is parsed and cleaned too, then only
  // the inner element's content is returned.
  var CONTEXTS = {
    table: ['<table>', '</table>', 'table'],
    thead: ['<table><thead>', '</thead></table>', 'thead'],
    tbody: ['<table><tbody>', '</tbody></table>', 'tbody'],
    tr: ['<table><tbody><tr>', '</tr></tbody></table>', 'tr']
  };

  function sanitizeToString(html, context) {
    var wrap = context && Object.prototype.hasOwnProperty.call(CONTEXTS, context) ? CONTEXTS[context] : null;
    var doc = parseDocument(wrap ? wrap[0] + html + wrap[1] : html);
    cleanTree(doc.body);
    if (!wrap) return doc.body.innerHTML;
    var inner = doc.body.querySelector(wrap[2]);
    return inner ? inner.innerHTML : '';
  }

  function sameOriginURL(url) {
    try {
      var u = new URL(String(url), document.baseURI);
      return u.origin === window.location.origin && u.protocol !== 'javascript:';
    } catch (_) {
      return false;
    }
  }

  // -- htmx swap gate ----------------------------------------------------------
  var vettedResponses = [];
  var vettedScripts = [];

  function responseNonce(xhr) {
    var csp = (xhr && xhr.getResponseHeader && xhr.getResponseHeader('Content-Security-Policy')) || '';
    var m = /'nonce-([A-Za-z0-9+\/=_-]+)'/.exec(csp);
    return m ? m[1] : '';
  }

  // The nonce of the document that is loaded. Nonce attributes are hidden
  // from getAttribute once parsed, but the IDL property still reads.
  function pageNonce() {
    var el = document.querySelector('script[nonce]');
    return (el && el.nonce) || '';
  }

  // Removes scripts and styles the server did not sign. Signed scripts have
  // their text recorded, so the default policy can recognise it when htmx
  // copies them (htmx then stamps them with the page nonce). Signed styles are
  // re-stamped with the page nonce here, because htmx inserts them as they are
  // and the response's own nonce means nothing to the loaded page.
  function vetResponse(html, nonce) {
    var doc = parseDocument(html);
    var changed = false;
    var styles = doc.querySelectorAll('style');
    for (var j = 0; j < styles.length; j++) {
      if (nonce !== '' && styles[j].getAttribute('nonce') === nonce) {
        styles[j].setAttribute('nonce', pageNonce());
      } else {
        styles[j].remove();
      }
      changed = true;
    }
    var scripts = doc.querySelectorAll('script');
    for (var i = 0; i < scripts.length; i++) {
      var s = scripts[i];
      var type = (s.getAttribute('type') || '').trim().toLowerCase();
      if (!SERVER_SCRIPT_TYPES.hasOwnProperty(type)) continue; // JSON data, templates
      var signed = nonce !== '' && s.getAttribute('nonce') === nonce;
      var external = s.hasAttribute('src');
      if (signed && (!external || sameOriginURL(s.getAttribute('src')))) {
        vettedScripts.push(s.textContent);
      } else if (external && sameOriginURL(s.getAttribute('src')) && s.textContent === '') {
        vettedScripts.push('');
      } else {
        s.remove();
        changed = true;
      }
    }
    if (!changed) return html;
    // A full document keeps its shape; a fragment is re-serialised from body.
    return /^\s*(<!doctype|<html)/i.test(html) ? doc.documentElement.outerHTML : doc.body.innerHTML;
  }

  function isVettedResponse(input) {
    for (var i = 0; i < vettedResponses.length; i++) {
      var r = vettedResponses[i];
      var at = input.indexOf(r);
      if (at === -1) continue;
      // htmx wraps partials in bare structural tags (<body><template>,
      // <table><tbody>, ...) before parsing. Nothing else may be added.
      var rest = input.slice(0, at) + input.slice(at + r.length);
      if (rest.length <= 80 && /^(?:<\/?[a-z]+>)*$/.test(rest)) return true;
    }
    return false;
  }

  function closeSwapWindow() {
    vettedResponses = [];
    vettedScripts = [];
  }

  // On window, so it runs after every document-level beforeSwap listener
  // (boost-nav.js rewrites serverResponse there).
  window.addEventListener('htmx:beforeSwap', function (evt) {
    var d = evt.detail;
    if (!d || !d.shouldSwap || typeof d.serverResponse !== 'string') return;
    d.serverResponse = vetResponse(d.serverResponse, responseNonce(d.xhr));
    vettedResponses.push(d.serverResponse);
  });
  window.addEventListener('htmx:afterSettle', closeSwapWindow);
  window.addEventListener('htmx:swapError', closeSwapWindow);
  window.addEventListener('htmx:responseError', closeSwapWindow);

  // -- default policy ----------------------------------------------------------
  function reportRejection(kind, sample) {
    if (window.console && console.warn) {
      console.warn('[trusted-types] rejected ' + kind + ':', String(sample).slice(0, 120));
    }
  }

  if (tt && tt.createPolicy) {
    tt.createPolicy('default', {
      createHTML: function (input, _type, sink) {
        input = String(input);
        if (sink === 'DOMParser parseFromString' && isVettedResponse(input)) return input;
        return sanitizeToString(input);
      },
      createScript: function (input) {
        input = String(input);
        if (/^window\.__CF\$cv\$params/.test(input) || /turnstile/i.test(input)) {
          return input;
        }
        var at = vettedScripts.indexOf(input);
        if (at !== -1) {
          vettedScripts.splice(at, 1);
          return input;
        }
        reportRejection('script', input);
        return null;
      },
      createScriptURL: function (input) {
        var s = String(input);
        if (sameOriginURL(s) || /^https:\/\/challenges\.cloudflare\.com\//.test(s)) return s;
        reportRejection('script URL', s);
        return null;
      }
    });
  }

  // -- Public API for page scripts ---------------------------------------------
  window.dawaHTML = Object.freeze({
    // sanitize(html[, context]) -> TrustedHTML safe for innerHTML: allowlisted
    // markup only, no behaviour attributes, no styles. context is the element
    // the result goes into when that is a table part: 'table', 'thead',
    // 'tbody' or 'tr' (rows parsed outside a table lose their tags).
    sanitize: function (html, context) { return trustedHTML(sanitizeToString(html, context)); },
    // fromServer(html) -> TrustedHTML for a fragment fetched from this
    // application (templ output). Never pass it anything a user typed.
    fromServer: function (html) {
      return trustedHTML(vetResponse(String(html), ''));
    },
    // parse(html) -> inert Document of a response fetched from this
    // application.
    parse: function (html) { return parseDocument(html); },
    // escape(text) -> string safe to concatenate into markup.
    escape: function (text) {
      return String(text == null ? '' : text)
        .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
    }
  });

  // -- Alpine component registration -------------------------------------------
  // The CSP build of Alpine cannot call a global function from x-data, so a
  // page script registers its component by name instead:
  //   dawaData('pageManager', function (arg) { return { ... }; });
  //   <div x-data="pageManager"> or x-data="pageManager('arg')">
  // Works both before Alpine starts (a full page load, where page scripts run
  // before the deferred Alpine build) and after (a boosted swap).
  window.dawaData = function (name, factory) {
    if (window.Alpine && window.Alpine.data) {
      window.Alpine.data(name, factory);
      return;
    }
    document.addEventListener('alpine:init', function () {
      window.Alpine.data(name, factory);
    });
  };

  // -- Image fallbacks ---------------------------------------------------------
  // Replaces onerror="this.style.display='none'...". error does not bubble,
  // so this listens in the capture phase, and it is registered here -- before
  // <body> is parsed -- so it sees images that fail before deferred scripts run.
  //   data-img-fallback="hide"       hide the image
  //   data-img-fallback="next"       hide it and reveal its next sibling
  //                                  (display: data-fallback-display, or
  //                                  whatever its classes give it)
  //   data-img-fallback="parent"     hide its parent element
  //   data-img-fallback="src"        load data-fallback-src instead
  document.addEventListener('error', function (e) {
    var img = e.target;
    if (!img || img.tagName !== 'IMG' || !img.hasAttribute('data-img-fallback')) return;
    var mode = img.getAttribute('data-img-fallback');
    if (mode === 'src') {
      var alt = img.getAttribute('data-fallback-src');
      img.removeAttribute('data-img-fallback'); // once: the fallback may fail too
      if (alt) img.src = alt;
      return;
    }
    if (mode === 'parent' && img.parentElement) {
      img.parentElement.style.display = 'none';
      return;
    }
    img.style.display = 'none';
    var next = img.nextElementSibling;
    if (mode === 'next' && next) {
      next.hidden = false;
      next.classList.remove('d-none', 'hidden', 's-hidden-init');
      next.style.display = img.getAttribute('data-fallback-display') || '';
    }
  }, true);
})();
