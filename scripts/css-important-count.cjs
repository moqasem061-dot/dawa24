// Counts !important declarations outside comments and @media print blocks.
//
// Print rules must beat screen styles, including ones set from JavaScript, so
// they are exempt; so is invoice_printable.css, a standalone print document.
// Everything else should rank by cascade layer (see @layer overrides).
// Prints the total on stdout and a per-file breakdown on stderr.
'use strict';
const fs = require('fs');
const path = require('path');

const dir = path.join('internal', 'ui', 'static', 'css');
let total = 0;
for (const name of fs.readdirSync(dir).filter((f) => f.endsWith('.css')).sort()) {
  if (name === 'invoice_printable.css') continue;
  const src = fs.readFileSync(path.join(dir, name), 'utf8')
    .replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '));
  let depth = 0;
  let printDepth = -1;
  let count = 0;
  for (let i = 0; i < src.length; i++) {
    const c = src[i];
    if (c === '{') {
      let prelude = src.slice(src.lastIndexOf('}', i - 1) + 1, i);
      prelude = prelude.slice(prelude.lastIndexOf(';') + 1);
      if (printDepth < 0 && /@media\s+print/.test(prelude)) printDepth = depth;
      depth++;
    } else if (c === '}') {
      depth--;
      if (printDepth >= 0 && depth === printDepth) printDepth = -1;
    } else if (printDepth < 0 && src.startsWith('!important', i)) {
      count++;
    }
  }
  if (count) process.stderr.write(`${path.join(dir, name)}: ${count}\n`);
  total += count;
}
process.stdout.write(`${total}\n`);
