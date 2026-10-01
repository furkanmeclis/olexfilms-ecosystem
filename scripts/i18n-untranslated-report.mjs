#!/usr/bin/env node
/**
 * Untranslated-copy report (TEC-138).
 *
 * For every language beyond tr/en, counts the values that are still the en
 * text word for word: frontend locale JSON and the backend label catalog.
 * Universal terms (OK, PDF, VIN, brand names...) do not count: a value made
 * only of such words, {{params}}, numbers and punctuation is skipped. Each
 * language adds its own list in frontend/scripts/i18n-glossary.<lang>.json
 * ("keep").
 *
 * Usage (repo root):
 *   node scripts/i18n-untranslated-report.mjs
 *   node scripts/i18n-untranslated-report.mjs --locales=bg,ar --max=3
 *   node scripts/i18n-untranslated-report.mjs --list      # print the values
 *   node scripts/i18n-untranslated-report.mjs --json
 *
 * --max=<pct> exits 1 when a language is above the threshold.
 */

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const LOCALES_DIR = path.join(ROOT, "frontend/src/locales");
const GLOSSARY_DIR = path.join(ROOT, "frontend/scripts");
const I18N_DIR = path.join(ROOT, "backend/internal/platform/i18n");
const I18N_CONFIG = path.join(ROOT, "frontend/src/config/i18n.ts");

/** Words that stay the same in every language. */
const UNIVERSAL = [
  "OK", "PDF", "CSV", "XLSX", "XLS", "JSON", "HTML", "XML", "ZIP", "PNG", "JPG", "JPEG",
  "SVG", "WEBP", "GIF", "URL", "URI", "API", "ID", "UUID", "VIN", "SMS", "OTP", "QR",
  "IBAN", "SWIFT", "BIC", "E.164", "ISO", "UTC", "GMT", "TCMB", "ECB", "KVKK", "GDPR",
  "SKU", "EAN", "HTTP", "HTTPS", "SMTP", "IMAP", "DNS", "IP", "TLS", "SSL", "S3", "JWT",
  "2FA", "TOTP", "MFA", "AI", "MCP", "LLM", "CPU", "RAM", "GB", "MB", "KB", "TB",
  "iOS", "Android", "Web", "Slug", "Markdown", "Webhook", "Cron", "Redis", "Postgres",
  "PostgreSQL", "Meilisearch", "SeaweedFS", "Centrifugo", "Gotenberg", "Asynq", "Sentry",
  "Lexical", "Expo", "WhatsApp", "Telegram", "Google", "Apple", "Microsoft", "GitHub",
  "Slack", "Stripe", "iyzico", "Anthropic", "Claude", "OpenAI", "Twilio", "Netgsm",
  "wuzapi", "Olexfilms", "Olex", "Glorian", "NexPTG", "Dokploy", "TRY", "EUR", "USD",
  "GBP", "Bold", "A4", "A5", "Letter", "Legal", "px", "Email", "E-mail", "Logo",
];

const argv = process.argv.slice(2);
const flag = (name) => argv.includes(`--${name}`);
const option = (name) => {
  const i = argv.findIndex((a) => a === `--${name}` || a.startsWith(`--${name}=`));
  if (i === -1) return null;
  return argv[i].includes("=") ? argv[i].slice(argv[i].indexOf("=") + 1) : argv[i + 1];
};

function supportedLocales() {
  const src = fs.readFileSync(I18N_CONFIG, "utf8");
  const m = src.match(/SUPPORTED_LOCALES\s*=\s*\[([\s\S]*?)\]/);
  return m ? [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]) : [];
}

function flatten(value, prefix = "", out = {}) {
  if (value !== null && typeof value === "object" && !Array.isArray(value)) {
    for (const [k, v] of Object.entries(value)) flatten(v, prefix ? `${prefix}.${k}` : k, out);
  } else if (prefix) {
    out[prefix] = value == null ? "" : String(value);
  }
  return out;
}

function readLocale(locale) {
  const dir = path.join(LOCALES_DIR, locale);
  const out = {};
  if (!fs.existsSync(dir)) return out;
  for (const name of fs.readdirSync(dir).filter((n) => n.endsWith(".json"))) {
    const ns = name.slice(0, -5);
    const flat = flatten(JSON.parse(fs.readFileSync(path.join(dir, name), "utf8")));
    for (const [k, v] of Object.entries(flat)) out[`${ns}.${k}`] = v;
  }
  return out;
}

/** Go identifier of a locale's catalog map: zh-CN -> zhCNCatalog. */
function goVar(locale) {
  return `${locale.replace(/-([A-Za-z]+)/g, (_, r) => r)}Catalog`;
}

function readGoCatalog(src, varName) {
  const m = new RegExp(String.raw`\b${varName}\s*=\s*map\[string\]string\{`).exec(src);
  if (!m) return {};
  let depth = 1;
  let i = m.index + m[0].length;
  const start = i;
  for (; i < src.length && depth; i++) {
    if (src[i] === "{") depth++;
    else if (src[i] === "}") depth--;
    else if (src[i] === '"' || src[i] === "`") {
      const q = src[i];
      for (i++; i < src.length && src[i] !== q; i++) if (q === '"' && src[i] === "\\") i++;
    }
  }
  const body = src.slice(start, i - 1);
  const out = {};
  const re = /"((?:[^"\\]|\\.)*)"\s*:\s*"((?:[^"\\]|\\.)*)"/g;
  for (const x of body.matchAll(re)) out[JSON.parse(`"${x[1]}"`)] = JSON.parse(`"${x[2]}"`);
  return out;
}

function backendSources() {
  return fs
    .readdirSync(I18N_DIR)
    .filter((n) => /^catalog.*\.go$/.test(n) && !n.endsWith("_test.go"))
    .map((n) => fs.readFileSync(path.join(I18N_DIR, n), "utf8"))
    .join("\n");
}

function keepSet(locale) {
  const file = path.join(GLOSSARY_DIR, `i18n-glossary.${locale}.json`);
  const words = [...UNIVERSAL];
  if (fs.existsSync(file)) {
    const g = JSON.parse(fs.readFileSync(file, "utf8"));
    words.push(...(g.keep ?? []), ...(g.brands ?? []));
  }
  return new Set(words.map((w) => w.toLowerCase()));
}

/** True when the text has nothing a translator should change. */
function isUniversal(text, keep) {
  const stripped = text
    .replace(/{{\s*[a-zA-Z0-9_]+\s*}}/g, " ")
    .replace(/<[^>]+>/g, " ")
    .replace(/https?:\/\/\S+/g, " ")
    .replace(/[\w.+-]+@[\w-]+\.[\w.]+/g, " ");
  if (keep.has(stripped.trim().toLowerCase())) return true;
  const words = stripped.match(/[\p{L}][\p{L}\p{N}.+-]*/gu) ?? [];
  return words.every((w) => keep.has(w.toLowerCase()) || keep.has(w.replace(/[.]+$/, "").toLowerCase()));
}

function compare(en, other, keep) {
  const same = [];
  let total = 0;
  for (const [k, v] of Object.entries(en)) {
    if (!/\p{L}/u.test(v)) continue;
    total++;
    if (other[k] === v && !isUniversal(v, keep)) same.push(k);
  }
  return { total, same };
}

function main() {
  const all = supportedLocales().filter((l) => l !== "en" && l !== "tr");
  const only = option("locales");
  const locales = only ? only.split(",").map((x) => x.trim()).filter(Boolean) : all;
  const max = option("max") == null ? null : Number(option("max"));
  const en = readLocale("en");
  const goSrc = backendSources();
  const goEn = readGoCatalog(goSrc, "enCatalog");

  const rows = locales.map((locale) => {
    const keep = keepSet(locale);
    const fe = compare(en, readLocale(locale), keep);
    const be = compare(goEn, readGoCatalog(goSrc, goVar(locale)), keep);
    const total = fe.total + be.total;
    const same = fe.same.length + be.same.length;
    const pct = total ? (same / total) * 100 : 0;
    return {
      locale,
      frontend: { total: fe.total, same: fe.same.length },
      backend: { total: be.total, same: be.same.length },
      pct: Math.round(pct * 100) / 100,
      keys: [...fe.same, ...be.same.map((k) => `backend:${k}`)],
      ok: max == null || pct < max,
    };
  });

  if (flag("json")) {
    console.log(JSON.stringify(rows, null, 2));
  } else {
    console.log("locale   frontend same/total   backend same/total   untranslated %");
    for (const r of rows) {
      console.log(
        `${r.locale.padEnd(8)} ${`${r.frontend.same}/${r.frontend.total}`.padStart(19)}   ${`${r.backend.same}/${r.backend.total}`.padStart(18)}   ${r.pct.toFixed(2).padStart(6)}%${r.ok ? "" : "  > max"}`,
      );
      if (flag("list")) {
        const fe = readLocale(r.locale);
        for (const k of r.keys) console.log(`    ${k}: ${JSON.stringify(k.startsWith("backend:") ? goEn[k.slice(8)] : fe[k])}`);
      }
    }
  }
  process.exit(rows.every((r) => r.ok) ? 0 : 1);
}

main();
