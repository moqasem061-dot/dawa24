#!/usr/bin/env node
// Checks every Alpine expression in the templates against the parser of the
// @alpinejs/csp build the site loads. That build never calls eval or
// new Function, which is what lets the Content-Security-Policy drop
// 'unsafe-eval' — but it only understands a subset of JavaScript: one
// expression per attribute, no arrow functions, template literals, spread,
// `??`, `?.`, `+=`, `typeof` or `new`, and no globals (window, document,
// Math, JSON, parseInt, ...). An expression outside that subset fails
// silently at runtime with a console error, so this runs as a gate instead.
//
// Usage: node scripts/alpine-csp/check.cjs [--dynamic]
//   --dynamic  also list attributes whose value is a templ Go expression;
//              those are only checked at runtime.
"use strict";

const fs = require("fs");
const path = require("path");
const { Tokenizer, Parser } = require("./parser.cjs");

const root = path.resolve(__dirname, "..", "..");
const scanDirs = ["internal"];

// Directives whose value is not an expression.
const nonExpression = /^x-(transition|ref|cloak|teleport|ignore|mask(?!:dynamic)|collapse|trap\.|anchor\.)/;

// Globals the CSP build refuses to resolve. Anything a template needs from
// here belongs in an Alpine.data component or an Alpine.magic.
const forbiddenGlobals = new Set([
  "window", "document", "globalThis", "self", "top", "parent", "frames",
  "Math", "JSON", "Number", "String", "Boolean", "Object", "Array", "Date",
  "Intl", "RegExp", "Promise", "Symbol", "Reflect", "Proxy", "Error",
  "parseInt", "parseFloat", "isNaN", "isFinite", "encodeURIComponent",
  "decodeURIComponent", "encodeURI", "decodeURI", "console", "alert",
  "confirm", "prompt", "fetch", "setTimeout", "setInterval", "clearTimeout",
  "clearInterval", "requestAnimationFrame", "location", "history",
  "navigator", "localStorage", "sessionStorage", "htmx", "Alpine", "event",
  "structuredClone", "queueMicrotask", "FormData", "URL", "URLSearchParams",
  "Event", "CustomEvent", "Element", "HTMLElement", "Node",
  // Application helpers on window: invisible to the CSP build as well.
  "getCsrfToken", "getCookie", "showToast", "dawaNavigate", "dawaRefresh",
  "dawaT", "dawaHTML", "dawaData",
]);

// The CSP build refuses property assignments on DOM objects; these magics
// always hold one.
const domRoots = new Set(["$el", "$refs", "$root", "$event"]);

function assignsToDom(node) {
  if (!node || typeof node !== "object") return false;
  if (node.type === "AssignmentExpression" || node.type === "UpdateExpression") {
    let target = node.type === "AssignmentExpression" ? node.left : node.argument;
    while (target && target.type === "MemberExpression") target = target.object;
    if (target && target.type === "Identifier" && domRoots.has(target.name)) return true;
  }
  return Object.values(node).some((v) =>
    Array.isArray(v) ? v.some(assignsToDom) : v && typeof v === "object" && assignsToDom(v));
}

function walk(dir, out) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(p, out);
    else if (entry.name.endsWith(".templ")) out.push(p);
  }
  return out;
}

function decodeEntities(s) {
  return s
    .replace(/&quot;/g, '"')
    .replace(/&#39;|&#x27;|&apos;/g, "'")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&amp;/g, "&");
}

// Collects identifiers read from the component scope: roots of member chains
// and bare names, but not property names or object keys.
function rootIdentifiers(node, out) {
  if (!node || typeof node !== "object") return out;
  switch (node.type) {
    case "Identifier":
      out.push(node.name);
      break;
    case "MemberExpression":
      rootIdentifiers(node.object, out);
      if (node.computed) rootIdentifiers(node.property, out);
      break;
    case "Property":
      if (node.computed) rootIdentifiers(node.key, out);
      rootIdentifiers(node.value, out);
      break;
    default:
      for (const [key, value] of Object.entries(node)) {
        if (key === "type") continue;
        if (Array.isArray(value)) value.forEach((v) => rootIdentifiers(v, out));
        else if (value && typeof value === "object") rootIdentifiers(value, out);
      }
  }
  return out;
}

// Components registered with dawaData(...) / Alpine.data(...). x-data may
// only call these: a plain global function is invisible to the CSP build.
function registeredComponents() {
  const names = new Set();
  const re = /(?:dawaData|Alpine\.data)\(\s*['"]([\w$]+)['"]/g;
  const walkAll = (dir) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const p = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        if (entry.name !== "vendor") walkAll(p);
      } else if (/\.(templ|js)$/.test(entry.name)) {
        let m;
        const src = fs.readFileSync(p, "utf8");
        while ((m = re.exec(src))) names.add(m[1]);
      }
    }
  };
  walkAll(path.join(root, "internal"));
  return names;
}
const registered = registeredComponents();

function check(expr, directive) {
  const ast = new Parser(new Tokenizer(expr).tokenize()).parse();
  const bad = rootIdentifiers(ast, []).filter((n) => forbiddenGlobals.has(n));
  if (bad.length) throw new Error(`uses global ${[...new Set(bad)].join(", ")}`);
  if (assignsToDom(ast)) throw new Error("assigns a property of a DOM object ($el, $refs, $event)");
  if (directive === "x-data") {
    const callee = ast.type === "CallExpression" ? ast.callee : ast;
    if (callee.type === "Identifier" && !registered.has(callee.name)) {
      throw new Error(`component ${callee.name} is not registered with dawaData()`);
    }
  }
}

// attr="value" / attr='value' / attr={ go } on Alpine attributes.
const attrRe = /(?<=[\s"'])(x-[\w-]+(?::[\w.-]+)?(?:\.[\w.-]+)*|@[\w.:-]+|:[\w-]+(?:\.[\w-]+)*)\s*=\s*(?:"([^"]*)"|'([^']*)'|(\{))/g;

// goExpression returns the Go source between the braces of attr={ ... }.
function goExpression(src, open) {
  let depth = 0;
  let quote = null;
  for (let i = open; i < src.length; i++) {
    const c = src[i];
    if (quote) {
      if (c === "\\" && quote !== "`") { i++; continue; }
      if (c === quote) quote = null;
      continue;
    }
    if (c === '"' || c === "`") { quote = c; continue; }
    if (c === "{") depth++;
    if (c === "}" && --depth === 0) return src.slice(open + 1, i).trim();
  }
  return "";
}

// simulateGo turns the common Go shapes into a representative expression:
//   fmt.Sprintf("fmt", ...)   verbs replaced by sample values
//   "a" + goValue + "b"       Go values replaced by a sample word
// Anything else (a helper function's result) returns null and is listed as
// runtime-checked only.
function simulateGo(expr) {
  const goString = (lit) => {
    if (lit[0] === "`") return lit.slice(1, -1);
    return JSON.parse(lit);
  };
  const sample = (fmtStr) =>
    fmtStr
      .replace(/'%s'/g, "'x'")
      .replace(/%%/g, "%")
      .replace(/%q/g, '"x"')
      .replace(/%[-+ 0#]*\d*(?:\.\d+)?[dfgv]/g, "1")
      .replace(/%t/g, "true")
      .replace(/%s/g, "null");
  // \x5c is a backslash: a Go string literal is "(non-quote|escape)*".
  const goStr = '"(?:[^"\\x5c]|\\x5c.)*"';
  let m = new RegExp("^fmt\\.Sprintf\\(\\s*(" + goStr + "|`[^`]*`)").exec(expr);
  if (m) return sample(goString(m[1]));
  // components.AlpineCall("name", args...) renders name(<json>, ...).
  m = /^(?:components\.)?AlpineCall\(\s*"([\w$]+)"/.exec(expr);
  if (m) return m[1] + "()";
  // "lit" + goValue + "lit" ...: split on top-level +, literals kept, Go
  // values (identifiers, calls such as i18n.T(...)) replaced by x.
  const parts = [];
  let depth = 0;
  let cur = "";
  for (let i = 0; i < expr.length; i++) {
    const c = expr[i];
    if (c === '"') {
      const lit = new RegExp("^" + goStr).exec(expr.slice(i));
      if (!lit) return null;
      cur += lit[0];
      i += lit[0].length - 1;
      continue;
    }
    if (c === "(") depth++;
    if (c === ")") depth--;
    if (c === "+" && depth === 0) {
      parts.push(cur.trim());
      cur = "";
      continue;
    }
    cur += c;
  }
  parts.push(cur.trim());
  if (!parts.some((p) => p[0] === '"')) return null;
  return parts.map((p) => (new RegExp("^" + goStr + "$").test(p) ? JSON.parse(p) : "x")).join("");
}

const failures = [];
const dynamic = [];
let checked = 0;

for (const dir of scanDirs) {
  for (const file of walk(path.join(root, dir), [])) {
    const src = fs.readFileSync(file, "utf8");
    const rel = path.relative(root, file).split(path.sep).join("/");
    let m;
    attrRe.lastIndex = 0;
    while ((m = attrRe.exec(src))) {
      const name = m[1];
      const line = src.slice(0, m.index).split("\n").length;
      if (nonExpression.test(name)) continue;
      let value;
      if (m[4]) {
        const goExpr = goExpression(src, m.index + m[0].length - 1);
        value = simulateGo(goExpr);
        if (value === null) {
          dynamic.push(`${rel}:${line}: ${name}={ ${goExpr.replace(/\s+/g, " ").slice(0, 80)} }`);
          continue;
        }
      } else {
        value = decodeEntities(m[2] !== undefined ? m[2] : m[3]).trim();
      }
      if (value === "") continue;
      if (name === "x-for") {
        const at = value.search(/\s+(in|of)\s+/);
        if (at < 0) {
          failures.push(`${rel}:${line}: ${name}: no "in" clause`);
          continue;
        }
        value = value.slice(at).replace(/^\s+(in|of)\s+/, "");
      }
      checked++;
      try {
        check(value, name);
      } catch (err) {
        const shown = value.length > 90 ? value.slice(0, 87) + "..." : value;
        failures.push(`${rel}:${line}: ${name}="${shown.replace(/\s+/g, " ")}" -> ${err.message}`);
      }
    }
  }
}

if (process.argv.includes("--dynamic")) {
  console.log(`Dynamic Alpine attributes (runtime-checked only): ${dynamic.length}`);
  dynamic.forEach((d) => console.log("  " + d));
}
if (failures.length) {
  console.error(`Alpine CSP check: ${failures.length} of ${checked} expressions are not supported by @alpinejs/csp`);
  failures.forEach((f) => console.error("  " + f));
  process.exit(1);
}
console.log(`Alpine CSP check: ${checked} expressions OK`);
