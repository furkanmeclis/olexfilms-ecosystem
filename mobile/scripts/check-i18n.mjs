#!/usr/bin/env node
/**
 * Mobile i18n check, same rules as the web's scripts/check-i18n.mjs:
 *   - tr and en match key for key (error)
 *   - other languages may miss keys (warning; error with --strict), never extra keys
 *   - {{param}} sets equal to en
 *   - no empty values
 *
 * Usage: pnpm i18n:check [--strict]
 */
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { LANGUAGES } from './i18n-languages.mjs';

const strict = process.argv.includes('--strict');
const dir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../src/core/i18n/locales');
const read = (lang, ns) => {
  const f = path.join(dir, lang, `${ns}.json`);
  return fs.existsSync(f) ? JSON.parse(fs.readFileSync(f, 'utf8')) : null;
};
const params = (s) => [...String(s).matchAll(/{{\s*(\w+)\s*}}/g)].map((m) => m[1]).sort().join(',');

const namespaces = fs.readdirSync(path.join(dir, 'en')).map((f) => f.replace(/\.json$/, ''));
let errors = 0;
let warnings = 0;
const err = (m) => (errors++, console.error(`error  ${m}`));
const warn = (m) => (warnings++, console.warn(`warn   ${m}`));

for (const ns of namespaces) {
  const en = read('en', ns);
  for (const lang of LANGUAGES) {
    if (lang === 'en') continue;
    const cat = read(lang, ns);
    if (!cat) {
      err(`${lang}/${ns}.json missing`);
      continue;
    }
    for (const k of Object.keys(cat)) {
      if (!(k in en)) err(`${lang}/${ns}: extra key ${k}`);
      else if (!String(cat[k]).trim()) err(`${lang}/${ns}: empty ${k}`);
      else if (params(cat[k]) !== params(en[k])) err(`${lang}/${ns}: {{params}} differ for ${k}`);
    }
    const missing = Object.keys(en).filter((k) => !(k in cat));
    if (missing.length) {
      const m = `${lang}/${ns}: ${missing.length} missing (falls back to en)`;
      lang === 'tr' || strict ? err(m) : warn(m);
    }
  }
}
console.log(`i18n: ${errors} errors, ${warnings} warnings`);
process.exit(errors ? 1 : 0);
