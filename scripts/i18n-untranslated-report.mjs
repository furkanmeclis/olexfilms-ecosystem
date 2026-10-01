#!/usr/bin/env node
/**
 * Untranslated-copy report (TEC-138).
 *
 * For every language beyond en/tr it counts the frontend keys whose value is
 * missing (falls back to en at runtime) or exactly the en text, and does the
 * same for the backend label catalog. Universal terms (brand names, acronyms
 * such as PDF/VIN/OTP/UUID, numbers, e-mail/URL samples, pure {{params}}) may
 * stay equal to en: they are listed apart as "universal" and left out of the
 * gated percentage (the raw share including them is printed too).
 *
 * Usage (repo root):
 *   node scripts/i18n-untranslated-report.mjs
 *   node scripts/i18n-untranslated-report.mjs --locales de,fr,es,it,ru,uk
 *   node scripts/i18n-untranslated-report.mjs --locales de --list
 *   node scripts/i18n-untranslated-report.mjs --threshold 3 --json
 *
 * Exits 1 when a selected language is at or above the threshold (default 3%).
 */

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const LOCALES_DIR = path.join(ROOT, "frontend/src/locales");
const I18N_CONFIG = path.join(ROOT, "frontend/src/config/i18n.ts");
const BACKEND_DIR = path.join(ROOT, "backend/internal/platform/i18n");
const SKIP = new Set(["en", "tr"]);

const args = process.argv.slice(2);
const flag = (name) => args.includes(name);
function option(name) {
  const i = args.findIndex((a) => a === name || a.startsWith(`${name}=`));
  if (i < 0) return null;
  return args[i].includes("=") ? args[i].split("=").slice(1).join("=") : args[i + 1];
}

if (flag("--help") || flag("-h")) {
  console.log(`i18n-untranslated-report — share of keys still equal to en

  --locales a,b    languages to report (default: every language but en/tr)
  --threshold N    fail when a language reaches N percent (default 3)
  --list           print the keys that equal en (and missing keys)
  --json           machine-readable output`);
  process.exit(0);
}

const supported = (() => {
  const m = fs.readFileSync(I18N_CONFIG, "utf8").match(/SUPPORTED_LOCALES\s*=\s*\[([\s\S]*?)\]/);
  return m ? [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]) : [];
})();
const selected = option("--locales")
  ? option("--locales")
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
  : supported.filter((l) => !SKIP.has(l));
const unknown = selected.filter((l) => !supported.includes(l));
if (unknown.length) {
  console.error(`Unknown locale(s): ${unknown.join(", ")}`);
  process.exit(2);
}
const threshold = Number(option("--threshold") ?? 3);

/** Tokens that stay the same in every language. */
const UNIVERSAL_WORDS = new Set(
  [
    "olex", "olexfilms", "glorian", "ppf", "pdf", "csv", "json", "xlsx", "excel", "vin", "otp",
    "whatsapp", "mcp", "uuid", "id", "tcmb", "ecb", "kvkk", "ubl-tr", "html", "xslt", "api",
    "url", "sms", "ok", "github", "google", "facebook", "apple", "a4", "a3", "png", "jpg",
    "jpeg", "webp", "svg", "cms", "ai", "totp", "2fa", "e-mail", "smtp", "iban", "regex",
    "oauth", "webhook", "markdown", "wuzapi", "expo", "centrifugo", "meilisearch", "s3",
    "gotenberg", "sentry", "dsn", "ip", "min", "max", "slug", "gtin", "ean", "sku", "qr", "passkey", "passkeys", "tsv",
  ].map((w) => w.toLowerCase()),
);

function isUniversal(value) {
  // PEM armour placeholders and other ASCII-only markers stay as they are.
  if (/^-+BEGIN [A-Z ]+-+$/.test(String(value).trim())) return true;
  const text = String(value)
    .replace(/{{\s*[a-zA-Z0-9_]+\s*}}/g, " ")
    .replace(/\S+@\S+\.\S+/g, " ")
    .replace(/https?:\/\/\S+/g, " ");
  const words = text.match(/[\p{L}][\p{L}\p{N}.-]*/gu) ?? [];
  return words.every((w) => UNIVERSAL_WORDS.has(w.toLowerCase().replace(/[.-]+$/, "")));
}

function flatten(value, prefix = "", out = {}) {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    for (const [k, v] of Object.entries(value)) flatten(v, prefix ? `${prefix}.${k}` : k, out);
  } else if (prefix) {
    out[prefix] = value == null ? "" : String(value);
  }
  return out;
}

function readLocale(locale) {
  const dir = path.join(LOCALES_DIR, locale);
  const keys = {};
  if (!fs.existsSync(dir)) return keys;
  for (const name of fs.readdirSync(dir).filter((n) => n.endsWith(".json"))) {
    const ns = name.slice(0, -5);
    const flat = flatten(JSON.parse(fs.readFileSync(path.join(dir, name), "utf8")));
    for (const [k, v] of Object.entries(flat)) keys[`${ns}.${k}`] = v;
  }
  return keys;
}

/** map[string]string literal named varName in the Go sources. */
function readGoCatalog(varName, locale) {
  const own = locale ? [`catalog_${locale.toLowerCase().replace(/-/g, "_")}.go`] : [];
  for (const file of [...own, "catalog_locales.go", "catalog.go"]) {
    const full = path.join(BACKEND_DIR, file);
    if (!fs.existsSync(full)) continue;
    const src = fs.readFileSync(full, "utf8");
    const m = new RegExp(String.raw`\b${varName}\s*=\s*map\[string\]string\{`).exec(src);
    if (!m) continue;
    let depth = 1;
    let i = m.index + m[0].length;
    const start = i;
    for (; i < src.length && depth > 0; i++) {
      if (src[i] === '"') {
        for (i++; i < src.length && src[i] !== '"'; i++) if (src[i] === "\\") i++;
        continue;
      }
      if (src[i] === "{") depth++;
      if (src[i] === "}") depth--;
    }
    const body = src.slice(start, i - 1);
    const map = {};
    for (const hit of body.matchAll(/"((?:\\.|[^"\\])*)"\s*:\s*"((?:\\.|[^"\\])*)"/g)) {
      map[JSON.parse(`"${hit[1]}"`)] = JSON.parse(`"${hit[2]}"`);
    }
    return map;
  }
  return null;
}

const goVar = (locale) => `${locale.replace(/-([A-Za-z]+)/g, (_, r) => r)}Catalog`;

function compare(en, other) {
  const missing = [];
  const identical = [];
  const universal = [];
  for (const [k, v] of Object.entries(en)) {
    const got = other[k];
    if (got == null || !String(got).trim()) missing.push(k);
    else if (String(got).trim() === String(v).trim()) {
      (isUniversal(v) ? universal : identical).push(k);
    }
  }
  const total = Object.keys(en).length;
  // Gate: missing keys plus copies equal to en that are not universal terms.
  const untranslated = missing.length + identical.length;
  const raw = untranslated + universal.length;
  return {
    total,
    missing,
    identical,
    universal,
    untranslated,
    pct: total ? (untranslated / total) * 100 : 0,
    rawPct: total ? (raw / total) * 100 : 0,
  };
}

const enFrontend = readLocale("en");
const enBackend = readGoCatalog("enCatalog") ?? {};
const report = [];
for (const locale of selected) {
  const fe = compare(enFrontend, readLocale(locale));
  const be = compare(enBackend, readGoCatalog(goVar(locale), locale) ?? {});
  report.push({ locale, frontend: fe, backend: be });
}

const failed = report.filter((r) => r.frontend.pct >= threshold || r.backend.pct >= threshold);

if (flag("--json")) {
  console.log(JSON.stringify({ threshold, ok: failed.length === 0, report }, null, 2));
} else {
  const fmt = (r) =>
    `${r.pct.toFixed(2).padStart(6)}%  (${r.untranslated}/${r.total}: missing ${r.missing.length}, same as en ${r.identical.length}; universal terms ${r.universal.length}, ${r.rawPct.toFixed(2)}% with them)`;
  console.log(`Untranslated share per language (threshold ${threshold}%)\n`);
  for (const r of report) {
    console.log(`${r.locale.padEnd(6)} frontend ${fmt(r.frontend)}`);
    console.log(`${"".padEnd(6)} backend  ${fmt(r.backend)}`);
    if (flag("--list")) {
      for (const [label, list] of [
        ["missing", [...r.frontend.missing, ...r.backend.missing.map((k) => `backend:${k}`)]],
        ["same as en", [...r.frontend.identical, ...r.backend.identical.map((k) => `backend:${k}`)]],
        ["universal", [...r.frontend.universal, ...r.backend.universal.map((k) => `backend:${k}`)]],
      ]) {
        if (!list.length) continue;
        console.log(`  ${label}:`);
        for (const k of list) {
          const key = k.startsWith("backend:") ? k.slice(8) : k;
          const v = k.startsWith("backend:") ? enBackend[key] : enFrontend[key];
          console.log(`    - ${k} = ${JSON.stringify(v)}`);
        }
      }
    }
  }
  console.log(
    failed.length
      ? `\nFAIL: ${failed.map((r) => r.locale).join(", ")} at or above ${threshold}%`
      : `\nOK: every language below ${threshold}%`,
  );
}
process.exit(failed.length ? 1 : 0);
