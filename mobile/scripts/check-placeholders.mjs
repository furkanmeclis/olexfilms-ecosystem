#!/usr/bin/env node
/**
 * Every text field in src/**\/*.tsx must carry a non-empty placeholder:
 *   - literal <Input …/> (ui/input) and raw <TextInput …/>;
 *   - placeholder="" / placeholder={''} counts as missing;
 *   - comments are ignored (a commented-out field, an apostrophe in a comment);
 *   - a field whose props are spread ({...rest}) without its own placeholder is
 *     only accepted inside the wrappers listed in WRAPPERS, which forward the
 *     caller's placeholder; their callers are checked like any field.
 * Exits 1 and lists file:line for each violation.
 *
 * Usage: npm run placeholders:check
 */
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../src');

/** Field components checked at every use. */
const FIELDS = ['Input', 'TextInput'];
/** Files (relative to src/) that define a field wrapper: the spread there forwards the caller's placeholder. */
const WRAPPERS = new Set(['ui/input.tsx']);

const walk = (d) =>
  fs.readdirSync(d, { withFileTypes: true }).flatMap((e) => {
    const p = path.join(d, e.name);
    if (e.isDirectory()) return walk(p);
    return e.name.endsWith('.tsx') ? [p] : [];
  });

/**
 * Blanks out comments (keeping line breaks and offsets). Strings are tracked
 * only inside braces and tags, so an apostrophe in JSX text ("Don't") does not
 * open a string.
 */
function stripComments(src) {
  let out = '';
  let quote = null;
  for (let i = 0; i < src.length; i++) {
    const c = src[i];
    if (quote) {
      out += c;
      if (c === '\\') out += src[++i] ?? '';
      else if (c === quote || (quote !== '`' && c === '\n')) quote = null;
      continue;
    }
    if (c === '/' && src[i + 1] === '/' && src[i - 1] !== ':') {
      while (i < src.length && src[i] !== '\n') {
        out += ' ';
        i++;
      }
      out += src[i] ?? '';
      continue;
    }
    if (c === '/' && src[i + 1] === '*') {
      const end = src.indexOf('*/', i + 2);
      const stop = end === -1 ? src.length : end + 2;
      out += src.slice(i, stop).replace(/[^\n]/g, ' ');
      i = stop - 1;
      continue;
    }
    if (c === '"' || c === '`' || (c === "'" && !/\w/.test(src[i - 1] ?? ''))) quote = c;
    out += c;
  }
  return out;
}

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

/** The tag's own attributes: attribute expressions kept, anything nested deeper (callbacks, objects) dropped. */
function ownAttributes(block) {
  let depth = 0;
  let out = '';
  for (const c of block) {
    if (c === '{') depth++;
    if (depth <= 1) out += c;
    if (c === '}') depth--;
  }
  return out;
}

const EMPTY = /(^|\s)placeholder\s*=\s*(""|''|\{\s*(""|''|``)\s*\})/;
const SPREAD = /\{\s*\.\.\./;

let violations = 0;
let total = 0;
const pattern = new RegExp(`<(${FIELDS.join('|')})(?=[\\s/>])`, 'g');
for (const file of walk(root)) {
  const rel = path.relative(root, file).split(path.sep).join('/');
  const src = stripComments(fs.readFileSync(file, 'utf8'));
  for (const m of src.matchAll(pattern)) {
    total++;
    const block = src.slice(m.index, tagEnd(src, m.index));
    const attrs = ownAttributes(block);
    const line = src.slice(0, m.index).split('\n').length;
    let why = null;
    if (EMPTY.test(attrs)) why = 'has an empty placeholder';
    else if (/(^|\s)placeholder\s*=/.test(attrs)) why = null;
    else if (SPREAD.test(attrs)) why = WRAPPERS.has(rel) ? null : 'spreads props without its own placeholder (add one, or list the wrapper in WRAPPERS)';
    else why = 'has no placeholder';
    if (why) {
      violations++;
      console.error(`error  src/${rel}:${line} <${m[1]}> ${why}`);
    }
  }
}
console.log(`placeholders: ${total} fields, ${violations} violations`);
process.exit(violations ? 1 : 0);
