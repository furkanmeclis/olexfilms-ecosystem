#!/usr/bin/env node
/**
 * Mobile i18n check, same rules as the web's scripts/check-i18n.mjs:
 *   - tr and en match key for key (error)
 *   - other languages may miss keys (warning; error with --strict), never extra keys,
 *     except the plural forms their language needs (ru `_few`, ar `_two`): i18next has no
 *     `_other` fallback within a language, a missing form shows the English text
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

const PLURAL_FORM = /_(zero|two|few|many)$/;
/** en anahtarı; dile özgü çoğul biçimi en'deki `_other` ile eşleşir. */
const enKey = (en, k) => (k in en ? k : PLURAL_FORM.test(k) && k.replace(PLURAL_FORM, '_other') in en ? k.replace(PLURAL_FORM, '_other') : null);

const namespaces = fs.readdirSync(path.join(dir, 'en')).map((f) => f.replace(/\.json$/, ''));
let errors = 0;
let warnings = 0;
const err = (m) => (errors++, console.error(`error  ${m}`));
const warn = (m) => (warnings++, console.warn(`warn   ${m}`));

for (const ns of namespaces) {
  const en = read('en', ns);
  for (const lang of LANGUAGES) {
    const cat = read(lang, ns);
    // i18next runs with keySeparator: false, so a nested object never resolves
    // and the screen shows the raw key (e.g. "filter.all"); keys stay flat.
    for (const [k, v] of Object.entries(cat ?? {})) {
      if (v !== null && typeof v === 'object') err(`${lang}/${ns}: nested object under ${k} (use flat "a.b" keys)`);
    }
    if (lang === 'en') continue;
    if (!cat) {
      err(`${lang}/${ns}.json missing`);
      continue;
    }
    for (const k of Object.keys(cat)) {
      const source = enKey(en, k);
      if (!source) err(`${lang}/${ns}: extra key ${k}`);
      else if (!String(cat[k]).trim()) err(`${lang}/${ns}: empty ${k}`);
      else if (params(cat[k]) !== params(en[source])) err(`${lang}/${ns}: {{params}} differ for ${k}`);
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
