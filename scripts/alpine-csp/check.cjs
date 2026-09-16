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
]);

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

function check(expr) {
  const ast = new Parser(new Tokenizer(expr).tokenize()).parse();
  const bad = rootIdentifiers(ast, []).filter((n) => forbiddenGlobals.has(n));
  if (bad.length) throw new Error(`uses global ${[...new Set(bad)].join(", ")}`);
}

// attr="value" / attr='value' / attr={ go } on Alpine attributes.
const attrRe = /(?<=[\s"'])(x-[\w-]+(?::[\w.-]+)?(?:\.[\w.-]+)*|@[\w.:-]+|:[\w-]+(?:\.[\w-]+)*)\s*=\s*(?:"([^"]*)"|'([^']*)'|(\{))/g;

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
      if (m[4]) {
        dynamic.push(`${rel}:${line}: ${name}={...}`);
        continue;
      }
      let value = decodeEntities(m[2] !== undefined ? m[2] : m[3]).trim();
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
        check(value);
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
