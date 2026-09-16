/* ==========================================================================
   خط سير التوصيل — the courier's route map.

   The server plans the round; this draws it and keeps it honest as the
   courier moves. Three jobs and nothing else:

     1. Draw the planned order on a map: an origin pin, numbered stop pins in
        the order the server chose, and a line through them.
     2. Re-plan from the courier's real position when they ask for it, by
        sending the fix back and redrawing from the new order.
     3. Degrade to nothing. Every fact on this page is also rendered as HTML
        by the server, so a phone that blocks location, refuses Leaflet or
        loses the tile host still shows the full ordered list with working
        navigation links. The map is an aid, never the only copy.
   ========================================================================== */

(function () {
  'use strict';

  var TILE_URL = 'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png';
  var el, map, layer, meMarker, state = null, watchId = null;

  document.addEventListener('DOMContentLoaded', function () {
    el = document.getElementById('courier-route-map');
    if (!el) return;

    bindControls();
    load(null).then(boot).catch(function (err) {
      console.warn('delivery route: initial load failed', err);
    });
  });

  /* ---- data ------------------------------------------------------------ */

  // load asks the server to plan the round, optionally from a position the
  // browser has just measured. The plan is the server's: the ordering rule
  // must be the same one the HTML list was rendered with, so it is never
  // recomputed here.
  function load(pos) {
    var url = el.getAttribute('data-route-endpoint') || '/vendor/delivery/route.json';
    if (pos) {
      url += (url.indexOf('?') === -1 ? '?' : '&') +
        'lat=' + encodeURIComponent(pos.lat) + '&lon=' + encodeURIComponent(pos.lon);
    }
    return fetch(url, { credentials: 'same-origin', headers: { 'Accept': 'application/json' } })
      .then(function (r) {
        if (!r.ok) throw new Error('route endpoint ' + r.status);
        return r.json();
      })
      .then(function (data) {
        state = data;
        return data;
      });
  }

  /* ---- map ------------------------------------------------------------- */

  function boot() {
    if (typeof ensureLeaflet !== 'function') {
      if (typeof L === 'undefined') return;
      return draw();
    }
    return ensureLeaflet().then(draw).catch(function (err) {
      console.warn('delivery route: leaflet unavailable', err);
    });
  }

  function draw() {
    if (typeof L === 'undefined' || !state) return;
    if (!map) {
      map = L.map(el, { zoomControl: false, attributionControl: false });
      L.tileLayer(TILE_URL, { maxZoom: 19 }).addTo(map);
      L.control.zoom({ position: 'topleft' }).addTo(map);
      layer = L.layerGroup().addTo(map);
    }
    layer.clearLayers();

    var path = [];
    if (state.has_origin) {
      var origin = [state.origin_lat, state.origin_lon];
      path.push(origin);
      L.marker(origin, { icon: pin('•', 'origin') })
        .addTo(layer)
        .bindPopup('<b>' + escapeHTML(el.getAttribute('data-origin-label') || '') + '</b>');
    }

    (state.stops || []).forEach(function (s) {
      var at = [s.lat, s.lon];
      path.push(at);
      L.marker(at, { icon: pin(String(s.seq), s.overdue ? 'late' : 'stop') })
        .addTo(layer)
        .bindPopup(popup(s));
    });

    if (path.length > 1) {
      // A dashed line, because it is a planned order and not a driven road.
      // Drawing it solid would claim a route through buildings.
      L.polyline(path, {
        color: '#2563eb', weight: 4, opacity: 0.75, dashArray: '10 8', lineJoin: 'round'
      }).addTo(layer);
    }

    fit();
  }

  function fit() {
    if (!map || !state) return;
    var pts = (state.stops || []).map(function (s) { return [s.lat, s.lon]; });
    if (state.has_origin) pts.unshift([state.origin_lat, state.origin_lon]);
    if (!pts.length) { map.setView([30.0444, 31.2357], 11); return; }
    if (pts.length === 1) { map.setView(pts[0], 15); return; }
    map.fitBounds(L.latLngBounds(pts), { padding: [36, 36], maxZoom: 16 });
  }

  // pin is a numbered marker. Numbering is the whole point of the map: the
  // courier reads the order off the pins without opening anything.
  function pin(label, kind) {
    return L.divIcon({
      className: '',
      html: '<span class="route-pin route-pin-' + kind + '">' + escapeHTML(label) + '</span>',
      iconSize: [30, 30],
      iconAnchor: [15, 15],
      popupAnchor: [0, -14]
    });
  }

  function popup(s) {
    var title = s.branch && s.branch !== s.name ? s.name + ' — ' + s.branch : s.name;
    var html = '<b>' + escapeHTML(String(s.seq) + '. ' + title) + '</b>';
    if (s.address) html += '<br><small>' + escapeHTML(s.address) + '</small>';
    html += '<br><small>' + escapeHTML(s.leg_km + ' km · ' + s.parcels + ' · ' + s.collect) + '</small>';
    html += '<br><a href="' + encodeURI(s.maps_url) + '" target="_blank" rel="noopener">→</a>';
    if (s.shipment_id) {
      html += ' <a href="/vendor/delivery/' + encodeURIComponent(s.shipment_id) + '">#</a>';
    }
    return html;
  }

  function escapeHTML(v) {
    return String(v == null ? '' : v).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  /* ---- controls -------------------------------------------------------- */

  function bindControls() {
    var fitBtn = document.querySelector('[data-route-fit]');
    if (fitBtn) fitBtn.addEventListener('click', fit);

    var followBtn = document.querySelector('[data-route-follow]');
    if (followBtn) followBtn.addEventListener('click', function () { toggleFollow(followBtn); });

    document.querySelectorAll('[data-route-locate]').forEach(function (btn) {
      btn.addEventListener('click', function () { replanFromHere(btn); });
    });
  }

  // replanFromHere is the button a courier presses halfway through the day.
  // It reloads the document rather than patching the DOM, because the ordered
  // list, the next-stop card and the summary are all server-rendered from the
  // same plan and must not be allowed to disagree with the map.
  function replanFromHere(btn) {
    if (!navigator.geolocation) { notify('التوقيع الجغرافي غير مدعوم على هذا الجهاز'); return; }
    busy(btn, true);
    navigator.geolocation.getCurrentPosition(function (p) {
      var url = new URL(window.location.href);
      url.searchParams.set('lat', p.coords.latitude.toFixed(6));
      url.searchParams.set('lon', p.coords.longitude.toFixed(6));
      window.location.assign(url.toString());
    }, function (err) {
      busy(btn, false);
      notify(err && err.code === 1
        ? 'لم يُسمح بالوصول إلى الموقع. فعّل إذن الموقع في المتصفح ثم أعد المحاولة.'
        : 'تعذّر تحديد موقعك الآن. حاول مرة أخرى في مكان مكشوف.');
    }, { enableHighAccuracy: true, timeout: 12000, maximumAge: 30000 });
  }

  // toggleFollow drops a live pin and keeps it current, without re-planning.
  // Re-ordering the round every few metres would make the list jump under the
  // reader's thumb; the courier asks for a new plan when they want one.
  function toggleFollow(btn) {
    if (!navigator.geolocation || !map) return;
    if (watchId !== null) {
      navigator.geolocation.clearWatch(watchId);
      watchId = null;
      if (meMarker) { map.removeLayer(meMarker); meMarker = null; }
      btn.classList.remove('is-active');
      return;
    }
    btn.classList.add('is-active');
    watchId = navigator.geolocation.watchPosition(function (p) {
      var at = [p.coords.latitude, p.coords.longitude];
      if (!meMarker) {
        meMarker = L.marker(at, { icon: pin('•', 'me') }).addTo(map);
      } else {
        meMarker.setLatLng(at);
      }
      map.panTo(at, { animate: true });
    }, function () {
      btn.classList.remove('is-active');
      watchId = null;
    }, { enableHighAccuracy: true, maximumAge: 10000, timeout: 20000 });
  }

  function busy(btn, on) {
    if (!btn) return;
    btn.disabled = !!on;
    btn.classList.toggle('is-busy', !!on);
  }

  function notify(msg) {
    if (typeof window.showToast === 'function') { window.showToast(msg, 'warning'); return; }
    window.alert(msg);
  }
})();
