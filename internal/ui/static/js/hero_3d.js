/**
 * Dawa 24 — Interactive 3D Pharmaceutical Capsule Model
 * High-performance, lightweight pure Canvas 3D renderer.
 * Zero external libraries, 60fps on mobile, touch-inertia, theme-adaptive.
 */
(function () {
  'use strict';

  function initHero3D() {
    var canvas = document.getElementById('hero-3d-capsule-canvas');
    if (!canvas) return;

    // Prevent duplicate initializations
    if (canvas._hero3d_inited) return;
    canvas._hero3d_inited = true;

    var ctx = canvas.getContext('2d');
    if (!ctx) return;

    var container = canvas.parentElement;
    var width = 0;
    var height = 0;
    var dpr = Math.min(window.devicePixelRatio || 1, 2);

    // Rotation state
    var rotX = 0.35;
    var rotY = 0.75;
    var rotZ = 0.15;
    var velX = 0;
    var velY = 0.008; // gentle continuous spin
    var autoSpin = true;
    var isDragging = false;
    var lastMouseX = 0;
    var lastMouseY = 0;
    var isVisible = true;
    var prefersReducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

    // Capsule dimensions
    var capRadius = 46;
    var capHalfHeight = 44;
    var ringRadius = 78;

    // Internal medicine spheres (beads)
    var beads = [];
    var beadCount = 28;
    for (var b = 0; b < beadCount; b++) {
      var angle = Math.random() * Math.PI * 2;
      var dist = Math.random() * (capRadius - 12);
      var by = (Math.random() * (capRadius + capHalfHeight * 0.75)); // in the lower transparent half
      beads.push({
        x: Math.cos(angle) * dist,
        y: by,
        z: Math.sin(angle) * dist,
        r: 3.5 + Math.random() * 3,
        color: b % 3 === 0 ? '#0284c7' : (b % 3 === 1 ? '#10b981' : '#38bdf8'),
        alpha: 0.85 + Math.random() * 0.15,
        floatSpeed: 0.02 + Math.random() * 0.02,
        phase: Math.random() * Math.PI * 2
      });
    }

    // Geometry mesh points for the 3D capsule
    var meshPoints = [];
    var rings = 22;
    var segments = 24;

    for (var r = 0; r <= rings; r++) {
      var v = r / rings; // 0 (top cap) to 1 (bottom cap)
      var py = 0;
      var pr = 0;

      if (v < 0.28) {
        // Top hemisphere cap
        var capAngle = (v / 0.28) * (Math.PI / 2);
        py = -capHalfHeight - capRadius * Math.cos(capAngle);
        pr = capRadius * Math.sin(capAngle);
      } else if (v > 0.72) {
        // Bottom hemisphere cap
        var capAngle2 = ((v - 0.72) / 0.28) * (Math.PI / 2);
        py = capHalfHeight + capRadius * Math.sin(capAngle2);
        pr = capRadius * Math.cos(capAngle2);
      } else {
        // Middle cylinder body
        var t = (v - 0.28) / 0.44;
        py = -capHalfHeight + t * (2 * capHalfHeight);
        pr = capRadius;
      }

      for (var s = 0; s < segments; s++) {
        var u = s / segments;
        var rad = u * Math.PI * 2;
        var px = pr * Math.cos(rad);
        var pz = pr * Math.sin(rad);

        // Normal
        var nx = Math.cos(rad);
        var nz = Math.sin(rad);
        var ny = 0;
        if (v < 0.28) {
          ny = -Math.cos((v / 0.28) * (Math.PI / 2));
        } else if (v > 0.72) {
          ny = Math.sin(((v - 0.72) / 0.28) * (Math.PI / 2));
        }

        meshPoints.push({
          x: px,
          y: py,
          z: pz,
          nx: nx,
          ny: ny,
          nz: nz,
          v: v,
          u: u
        });
      }
    }

    function resize() {
      var rect = container.getBoundingClientRect();
      width = Math.floor(rect.width) || 320;
      height = Math.floor(rect.height) || 280;

      canvas.width = Math.floor(width * dpr);
      canvas.height = Math.floor(height * dpr);
      canvas.style.width = width + 'px';
      canvas.style.height = height + 'px';
    }

    window.addEventListener('resize', resize, { passive: true });
    resize();

    // 3D vector rotation
    function rotatePoint(x, y, z, rx, ry, rz) {
      // Rotate Y
      var cosY = Math.cos(ry);
      var sinY = Math.sin(ry);
      var x1 = x * cosY + z * sinY;
      var z1 = -x * sinY + z * cosY;
      var y1 = y;

      // Rotate X
      var cosX = Math.cos(rx);
      var sinX = Math.sin(rx);
      var y2 = y1 * cosX - z1 * sinX;
      var z2 = y1 * sinX + z1 * cosX;
      var x2 = x1;

      // Rotate Z
      var cosZ = Math.cos(rz);
      var sinZ = Math.sin(rz);
      var x3 = x2 * cosZ - y2 * sinZ;
      var y3 = x2 * sinZ + y2 * cosZ;
      var z3 = z2;

      return { x: x3, y: y3, z: z3 };
    }

    // Light direction (studio top-left lighting)
    var lightDir = { x: -0.5, y: -0.7, z: -0.5 };
    var lightLen = Math.sqrt(lightDir.x * lightDir.x + lightDir.y * lightDir.y + lightDir.z * lightDir.z);
    lightDir.x /= lightLen;
    lightDir.y /= lightLen;
    lightDir.z /= lightLen;

    var time = 0;

    function render() {
      if (!isVisible) return;

      time += 0.02;
      ctx.clearRect(0, 0, canvas.width, canvas.height);

      ctx.save();
      ctx.scale(dpr, dpr);

      var cx = width / 2;
      var cy = height / 2 + Math.sin(time * 1.5) * 6; // subtle natural levitation float

      var isDark = document.documentElement.getAttribute('data-theme') === 'dark';

      // Update rotation
      if (!isDragging && autoSpin && !prefersReducedMotion) {
        velY = 0.009;
        rotY += velY;
        rotX += (0.35 - rotX) * 0.02; // return to nice aesthetic tilt
      } else if (!isDragging) {
        velX *= 0.93;
        velY *= 0.93;
        rotX += velX;
        rotY += velY;
      }

      // 1. Render soft ambient shadow on ground
      var shadowY = cy + capRadius + capHalfHeight + 35;
      var shadowW = 110 + Math.sin(time * 1.5) * 6;
      var shadowH = 22;
      var shadowGrad = ctx.createRadialGradient(cx, shadowY, 0, cx, shadowY, shadowW);
      if (isDark) {
        shadowGrad.addColorStop(0, 'rgba(2, 132, 199, 0.28)');
        shadowGrad.addColorStop(0.5, 'rgba(15, 23, 42, 0.45)');
        shadowGrad.addColorStop(1, 'rgba(0, 0, 0, 0)');
      } else {
        shadowGrad.addColorStop(0, 'rgba(2, 132, 199, 0.20)');
        shadowGrad.addColorStop(0.5, 'rgba(100, 116, 139, 0.25)');
        shadowGrad.addColorStop(1, 'rgba(0, 0, 0, 0)');
      }
      ctx.fillStyle = shadowGrad;
      ctx.beginPath();
      ctx.ellipse(cx, shadowY, shadowW, shadowH, 0, 0, Math.PI * 2);
      ctx.fill();

      // 2. Cold-chain orbital safety rings (3D Ring)
      var ringPoints = [];
      var ringSegs = 40;
      var ringRot = time * 0.8;
      for (var i = 0; i < ringSegs; i++) {
        var angle = (i / ringSegs) * Math.PI * 2;
        var rx = Math.cos(angle) * ringRadius;
        var ry = Math.sin(angle * 2) * 8; // gentle saddle wave
        var rz = Math.sin(angle) * ringRadius;
        var p = rotatePoint(rx, ry, rz, rotX * 0.5 + 0.3, rotY + ringRot, rotZ);
        ringPoints.push(p);
      }

      // Draw back portion of ring (z < 0)
      ctx.lineWidth = 2.5;
      ctx.strokeStyle = isDark ? 'rgba(56, 189, 248, 0.35)' : 'rgba(2, 132, 199, 0.35)';
      ctx.beginPath();
      var firstBack = true;
      for (var i = 0; i < ringSegs; i++) {
        if (ringPoints[i].z < 0) {
          var px = cx + ringPoints[i].x;
          var py = cy + ringPoints[i].y;
          if (firstBack) {
            ctx.moveTo(px, py);
            firstBack = false;
          } else {
            ctx.lineTo(px, py);
          }
        } else {
          firstBack = true;
        }
      }
      ctx.stroke();

      // 3. Transform and project capsule mesh polygons
      var transformed = [];
      for (var m = 0; m < meshPoints.length; m++) {
        var pt = meshPoints[m];
        var pos = rotatePoint(pt.x, pt.y, pt.z, rotX, rotY, rotZ);
        var norm = rotatePoint(pt.nx, pt.ny, pt.nz, rotX, rotY, rotZ);
        transformed.push({
          x: pos.x,
          y: pos.y,
          z: pos.z,
          nx: norm.x,
          ny: norm.y,
          nz: norm.z,
          v: pt.v,
          u: pt.u
        });
      }

      // Build and sort quads by average depth Z
      var quads = [];
      for (var r = 0; r < rings; r++) {
        for (var s = 0; s < segments; s++) {
          var nextS = (s + 1) % segments;
          var idx00 = r * segments + s;
          var idx10 = (r + 1) * segments + s;
          var idx11 = (r + 1) * segments + nextS;
          var idx01 = r * segments + nextS;

          var p00 = transformed[idx00];
          var p10 = transformed[idx10];
          var p11 = transformed[idx11];
          var p01 = transformed[idx01];

          var avgZ = (p00.z + p10.z + p11.z + p01.z) * 0.25;

          quads.push({
            p00: p00,
            p10: p10,
            p11: p11,
            p01: p01,
            z: avgZ,
            v: p00.v
          });
        }
      }

      // Sort painters algorithm: furthest away first
      quads.sort(function (a, b) {
        return a.z - b.z;
      });

      // 4. Render Quads
      for (var q = 0; q < quads.length; q++) {
        var quad = quads[q];
        var p00 = quad.p00;
        var p10 = quad.p10;
        var p11 = quad.p11;
        var p01 = quad.p01;

        // Face normal / Lighting computation
        var normZ = (p00.nz + p10.nz + p11.nz + p01.nz) * 0.25;
        var normX = (p00.nx + p10.nx + p11.nx + p01.nx) * 0.25;
        var normY = (p00.ny + p10.ny + p11.ny + p01.ny) * 0.25;

        // Backface culling for solid top half; transparent bottom half shows interior
        var isTopHalf = quad.v < 0.5;
        if (isTopHalf && normZ < -0.15) {
          continue;
        }

        var dot = -(normX * lightDir.x + normY * lightDir.y + normZ * lightDir.z);
        var diffuse = Math.max(0.08, dot);
        var specular = Math.pow(Math.max(0, dot + 0.3), 14) * 0.55;

        ctx.beginPath();
        ctx.moveTo(cx + p00.x, cy + p00.y);
        ctx.lineTo(cx + p10.x, cy + p10.y);
        ctx.lineTo(cx + p11.x, cy + p11.y);
        ctx.lineTo(cx + p01.x, cy + p01.y);
        ctx.closePath();

        if (isTopHalf) {
          // Top Half: Glossy Medical Dawa Blue Lacquer
          var rCol = Math.floor(2 + diffuse * 30 + specular * 200);
          var gCol = Math.floor(132 + diffuse * 65 + specular * 200);
          var bCol = Math.floor(199 + diffuse * 45 + specular * 50);
          ctx.fillStyle = 'rgb(' + Math.min(255, rCol) + ',' + Math.min(255, gCol) + ',' + Math.min(255, bCol) + ')';
          ctx.strokeStyle = ctx.fillStyle;
          ctx.lineWidth = 0.75;
          ctx.fill();
          ctx.stroke();
        } else {
          // Bottom Half: Transparent Cryo-Glass Shell
          var glassAlpha = normZ > 0.4 ? 0.35 : 0.15;
          var glassR = Math.floor(220 + specular * 35);
          var glassG = Math.floor(245 + specular * 10);
          var glassB = 255;
          ctx.fillStyle = 'rgba(' + glassR + ',' + glassG + ',' + glassB + ',' + (glassAlpha + specular * 0.4) + ')';
          ctx.strokeStyle = 'rgba(56, 189, 248, ' + (0.18 + specular * 0.3) + ')';
          ctx.lineWidth = 0.5;
          ctx.fill();
          ctx.stroke();
        }
      }

      // 5. Render Internal Medicine Pellets / Spheres inside the clear half
      beads.sort(function (a, b) {
        var pA = rotatePoint(a.x, a.y, a.z, rotX, rotY, rotZ);
        var pB = rotatePoint(b.x, b.y, b.z, rotX, rotY, rotZ);
        return pA.z - pB.z;
      });

      for (var b = 0; b < beads.length; b++) {
        var bead = beads[b];
        var bPos = rotatePoint(bead.x, bead.y + Math.sin(time + bead.phase) * 3, bead.z, rotX, rotY, rotZ);

        var bx = cx + bPos.x;
        var by = cy + bPos.y;
        var bScale = Math.max(0.6, (bPos.z + 100) / 100);
        var bRad = bead.r * bScale;

        var beadGrad = ctx.createRadialGradient(bx - bRad * 0.3, by - bRad * 0.3, bRad * 0.1, bx, by, bRad);
        beadGrad.addColorStop(0, '#ffffff');
        beadGrad.addColorStop(0.35, bead.color);
        beadGrad.addColorStop(1, '#0f172a');

        ctx.fillStyle = beadGrad;
        ctx.beginPath();
        ctx.arc(bx, by, bRad, 0, Math.PI * 2);
        ctx.fill();
      }

      // 6. Specular Glass Highlight Ribbon (Reflection on capsule curve)
      ctx.save();
      ctx.beginPath();
      var hlStart = rotatePoint(-capRadius * 0.72, -capHalfHeight - capRadius * 0.5, 30, rotX, rotY, rotZ);
      var hlEnd = rotatePoint(-capRadius * 0.72, capHalfHeight + capRadius * 0.5, 30, rotX, rotY, rotZ);
      ctx.moveTo(cx + hlStart.x, cy + hlStart.y);
      ctx.lineTo(cx + hlEnd.x, cy + hlEnd.y);
      ctx.lineWidth = 7;
      ctx.lineCap = 'round';
      var hlGrad = ctx.createLinearGradient(cx + hlStart.x, cy + hlStart.y, cx + hlEnd.x, cy + hlEnd.y);
      hlGrad.addColorStop(0, 'rgba(255, 255, 255, 0.85)');
      hlGrad.addColorStop(0.48, 'rgba(255, 255, 255, 0.45)');
      hlGrad.addColorStop(0.52, 'rgba(255, 255, 255, 0.25)');
      hlGrad.addColorStop(1, 'rgba(255, 255, 255, 0.05)');
      ctx.strokeStyle = hlGrad;
      ctx.stroke();
      ctx.restore();

      // 7. Middle Golden Division Ring / Micro-Seal
      var divPoints = [];
      for (var d = 0; d <= 28; d++) {
        var dAngle = (d / 28) * Math.PI * 2;
        var dPos = rotatePoint(capRadius * 1.01 * Math.cos(dAngle), 0, capRadius * 1.01 * Math.sin(dAngle), rotX, rotY, rotZ);
        divPoints.push(dPos);
      }
      ctx.beginPath();
      for (var d = 0; d < divPoints.length; d++) {
        if (divPoints[d].z >= -5) {
          if (d === 0) ctx.moveTo(cx + divPoints[d].x, cy + divPoints[d].y);
          else ctx.lineTo(cx + divPoints[d].x, cy + divPoints[d].y);
        }
      }
      ctx.strokeStyle = '#38bdf8';
      ctx.lineWidth = 2;
      ctx.stroke();

      // 8. Draw Front Portion of Orbital Ring (z >= 0)
      ctx.lineWidth = 3;
      ctx.strokeStyle = '#0ea5e9';
      ctx.beginPath();
      var firstFront = true;
      for (var i = 0; i < ringSegs; i++) {
        if (ringPoints[i].z >= 0) {
          var px = cx + ringPoints[i].x;
          var py = cy + ringPoints[i].y;
          if (firstFront) {
            ctx.moveTo(px, py);
            firstFront = false;
          } else {
            ctx.lineTo(px, py);
          }
        } else {
          firstFront = true;
        }
      }
      ctx.stroke();

      // Orbiting Cold-chain Data Node
      var nodeIndex = Math.floor((time * 6) % ringSegs);
      var nodePt = ringPoints[nodeIndex];
      if (nodePt.z >= 0) {
        ctx.fillStyle = '#10b981';
        ctx.beginPath();
        ctx.arc(cx + nodePt.x, cy + nodePt.y, 4.5, 0, Math.PI * 2);
        ctx.fill();
        ctx.strokeStyle = '#ffffff';
        ctx.lineWidth = 1.5;
        ctx.stroke();
      }

      ctx.restore();

      if (!prefersReducedMotion) {
        requestAnimationFrame(render);
      }
    }

    // Interactive Touch & Mouse Drag Handlers
    function startDrag(clientX, clientY) {
      isDragging = true;
      autoSpin = false;
      lastMouseX = clientX;
      lastMouseY = clientY;
      velX = 0;
      velY = 0;
    }

    function moveDrag(clientX, clientY) {
      if (!isDragging) return;
      var dx = clientX - lastMouseX;
      var dy = clientY - lastMouseY;
      lastMouseX = clientX;
      lastMouseY = clientY;

      velY = dx * 0.007;
      velX = -dy * 0.007;

      rotY += velY;
      rotX += velX;

      // Clamp X rotation so capsule doesn't flip upside down
      rotX = Math.max(-1.1, Math.min(1.1, rotX));

      if (prefersReducedMotion) {
        render();
      }
    }

    function endDrag() {
      if (!isDragging) return;
      isDragging = false;
      // After 3.5s of no interaction, re-enable auto-spin gently
      setTimeout(function () {
        if (!isDragging) autoSpin = true;
      }, 3500);
    }

    // Mouse Events
    canvas.addEventListener('mousedown', function (e) {
      startDrag(e.clientX, e.clientY);
    });
    window.addEventListener('mousemove', function (e) {
      moveDrag(e.clientX, e.clientY);
    });
    window.addEventListener('mouseup', endDrag);

    // Touch Events (using passive: false for move only when horizontal drag detected)
    var touchStartX = 0;
    var touchStartY = 0;
    var isHorizontalSwipe = false;

    canvas.addEventListener('touchstart', function (e) {
      if (e.touches.length === 1) {
        var t = e.touches[0];
        touchStartX = t.clientX;
        touchStartY = t.clientY;
        isHorizontalSwipe = false;
        startDrag(t.clientX, t.clientY);
      }
    }, { passive: true });

    canvas.addEventListener('touchmove', function (e) {
      if (e.touches.length === 1) {
        var t = e.touches[0];
        var diffX = Math.abs(t.clientX - touchStartX);
        var diffY = Math.abs(t.clientY - touchStartY);

        if (!isHorizontalSwipe && diffX > diffY && diffX > 6) {
          isHorizontalSwipe = true;
        }

        if (isHorizontalSwipe) {
          // Prevent page horizontal bounce while spinning 3D model
          moveDrag(t.clientX, t.clientY);
        }
      }
    }, { passive: true });

    canvas.addEventListener('touchend', endDrag, { passive: true });
    canvas.addEventListener('touchcancel', endDrag, { passive: true });

    // IntersectionObserver to pause loop when not on screen (zero battery drain)
    if ('IntersectionObserver' in window) {
      var observer = new IntersectionObserver(function (entries) {
        entries.forEach(function (entry) {
          isVisible = entry.isIntersecting;
          if (isVisible && !prefersReducedMotion) {
            requestAnimationFrame(render);
          }
        });
      }, { threshold: 0.1 });
      observer.observe(canvas);
    }

    // Start render loop
    requestAnimationFrame(render);
  }

  // Hook into DOM ready and HTMX navigation
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initHero3D);
  } else {
    initHero3D();
  }

  document.addEventListener('htmx:afterSettle', initHero3D);
  window.initHero3D = initHero3D;
})();
