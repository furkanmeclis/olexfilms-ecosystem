#!/usr/bin/env node
/**
 * Every literal <Input ... /> in src/**\/*.tsx must carry a placeholder prop.
 * Exits 1 and lists file:line for each violation.
 *
 * Usage: npm run placeholders:check
 */
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../src');

const walk = (d) =>
  fs.readdirSync(d, { withFileTypes: true }).flatMap((e) => {
    const p = path.join(d, e.name);
    if (e.isDirectory()) return walk(p);
    return e.name.endsWith('.tsx') ? [p] : [];
  });

/** End index of the opening tag starting at `start`, skipping braces, strings and template literals. */
function tagEnd(src, start) {
  let depth = 0;
  let quote = null;
  for (let i = start; i < src.length; i++) {
    const c = src[i];
    if (quote) {
      if (c === '\\') i++;
      else if (c === quote) quote = null;
      continue;
    }
    if (c === '"' || c === "'" || c === '`') quote = c;
    else if (c === '{') depth++;
    else if (c === '}') depth--;
    else if (c === '>' && depth === 0) return i;
  }
  return src.length;
}

let violations = 0;
let total = 0;
for (const file of walk(root)) {
  const src = fs.readFileSync(file, 'utf8');
  for (const m of src.matchAll(/<Input(?=[\s/>])/g)) {
    total++;
    const block = src.slice(m.index, tagEnd(src, m.index));
    if (!/(^|\s)placeholder\s*=/.test(block)) {
      violations++;
      const line = src.slice(0, m.index).split('\n').length;
      console.error(`error  ${path.relative(path.resolve(root, '..'), file)}:${line} <Input> has no placeholder`);
    }
  }
}
console.log(`placeholders: ${total} inputs, ${violations} violations`);
process.exit(violations ? 1 : 0);
