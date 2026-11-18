#!/usr/bin/env node
/**
 * i18n tarama raporu (TEC-1233): web, mobil ve backend genelinde her dil için
 *
 *   missing       kaynakta (en) olup dilde olmayan anahtar
 *   untranslated  değeri en'in ya da tr'nin aynısı kalmış anahtar (evrensel
 *                 terimler ve scripts/i18n-allowlist.json hariç)
 *   placeholder   {{param}} kümesi en'den farklı
 *   plural        dilin CLDR çoğul kategorilerinden (Intl.PluralRules) biri eksik
 *                 (ar: zero/one/two/few/many/other, ru/uk: one/few/many/other...)
 *
 * Alanlar:
 *   web             frontend/src/locales/<dil>/*.json
 *   mobile          mobile/src/core/i18n/locales/<dil>/*.json
 *   backend-labels  backend/internal/platform/i18n/catalog*.go (enCatalog)
 *   backend-texts   dil anahtarlı Go haritaları (bildirim, WhatsApp, e-posta,
 *                   PDF metinleri): dosya başına "en" girdisi kadar o dilin
 *                   girdisi olmalı
 *
 * Diller frontend/src/config/i18n.ts SUPPORTED_LOCALES'tan okunur; mobil ve
 * backend listeleri de aynı olmalı (fark varsa locale-list bulgusu).
 *
 * Kullanım (repo kökü):
 *   node scripts/i18n-scan.mjs                 # dil × alan özet tablosu
 *   node scripts/i18n-scan.mjs --list          # bulguları anahtar anahtar yaz
 *   node scripts/i18n-scan.mjs --locales de,fa
 *   node scripts/i18n-scan.mjs --markdown      # Linear/PR için Markdown tablo
 *   node scripts/i18n-scan.mjs --json
 *   node scripts/i18n-scan.mjs --fail          # herhangi bir bulguda çıkış 1
 *
 * Bilinçli istisnalar (marka, birim, kod, dilde aynı yazılan kelime):
 * scripts/i18n-allowlist.json.
 */

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const WEB_DIR = path.join(ROOT, "frontend/src/locales");
const MOBILE_DIR = path.join(ROOT, "mobile/src/core/i18n/locales");
const BACKEND_I18N = path.join(ROOT, "backend/internal/platform/i18n");
const BACKEND_SRC = path.join(ROOT, "backend/internal");
const ALLOWLIST_FILE = path.join(ROOT, "scripts/i18n-allowlist.json");
const AREAS = ["web", "mobile", "backend-labels", "backend-texts"];
const KINDS = ["missing", "untranslated", "placeholder", "plural"];

const args = process.argv.slice(2);
const flag = (n) => args.includes(n);
function option(name) {
  const i = args.findIndex((a) => a === name || a.startsWith(`${name}=`));
  if (i < 0) return null;
  return args[i].includes("=") ? args[i].split("=").slice(1).join("=") : args[i + 1];
}

if (flag("--help") || flag("-h")) {
  console.log(`i18n-scan — eksik / çevrilmemiş / yer tutucu / çoğul raporu

  --locales a,b   yalnız bu diller (varsayılan: SUPPORTED_LOCALES)
  --list          bulguları anahtar anahtar yaz
  --markdown      özet tabloyu Markdown yaz
  --json          makine okunur çıktı
  --fail          bir bulgu varsa çıkış kodu 1`);
  process.exit(0);
}

export function readSupportedLocales() {
  const src = fs.readFileSync(path.join(ROOT, "frontend/src/config/i18n.ts"), "utf8");
  const m = src.match(/SUPPORTED_LOCALES\s*=\s*\[([\s\S]*?)\]/);
  return m ? [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]) : [];
}

const SUPPORTED = readSupportedLocales();
const selected = option("--locales")
  ? option("--locales")
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
  : SUPPORTED;
const unknown = selected.filter((l) => !SUPPORTED.includes(l));
if (unknown.length) {
  console.error(`Bilinmeyen dil: ${unknown.join(", ")} (SUPPORTED_LOCALES: ${SUPPORTED.join(", ")})`);
  process.exit(2);
}

const allowlist = fs.existsSync(ALLOWLIST_FILE)
  ? JSON.parse(fs.readFileSync(ALLOWLIST_FILE, "utf8"))
  : {};

/** Her dilde aynı kalan kelimeler (marka, kısaltma, birim, dosya türü). */
const UNIVERSAL_WORDS = new Set(
  [
    "olex", "olexfilms", "glorian", "nexptg", "ppf", "pdf", "csv", "json", "xlsx", "xls", "excel",
    "vin", "otp", "whatsapp", "mcp", "uuid", "id", "tcmb", "ecb", "kvkk", "ubl-tr", "html",
    "xslt", "api", "url", "sms", "ok", "github", "google", "facebook", "apple", "a4", "a3",
    "a5", "png", "jpg", "jpeg", "webp", "svg", "heic", "gif", "mp4", "cms", "ai", "totp", "2fa",
    "e-mail", "smtp", "iban", "regex", "oauth", "webhook", "markdown", "wuzapi", "expo",
    "centrifugo", "meilisearch", "s3", "gotenberg", "sentry", "dsn", "ip", "min", "max",
    "slug", "gtin", "ean", "ean-13", "sku", "qr", "passkey", "passkeys", "tsv", "zpl", "tspl",
    "esc", "pos", "gs1", "code128", "iso", "utc", "gmt", "http", "https", "ios", "android",
    "mm", "cm", "km", "m", "m²", "kg", "g", "ml", "l", "px", "dpi", "kb", "mb", "gb", "ms",
    "usd", "eur", "try", "gbp", "tl", "vat", "kdv", "erp", "crm", "b2b", "b2c", "sla", "kpi",
    "roi", "nps", "gps", "nfc", "ble", "usb", "wi-fi", "wifi", "led", "uv", "ir", "tpu",
    "ral", "hex", "rgb", "cmyk", "bcc", "cc", "cron", "jwt", "rfc", "e164", "e.164", "isbn",
    "linkedin", "instagram", "tiktok", "youtube", "x", "telegram", "viber", "openai",
    "anthropic", "claude", "gpt", "llm", "seo", "og", "utm", "vs", "dhl", "ups", "fedex",
    "yurtiçi", "aras", "mng", "ptt", "e-arşiv", "e-fatura", "e-irsaliye", "gib", "mersis",
    "nace", "abc", "xyz", "lorem", "ipsum",
    ...(allowlist.universal ?? []),
  ].map((w) => w.toLowerCase()),
);

export function isUniversal(value) {
  const raw = String(value ?? "").trim();
  if (!raw) return true;
  if (/^-+BEGIN [A-Z ]+-+$/.test(raw)) return true;
  const text = raw
    .replace(/{{\s*[a-zA-Z0-9_]+\s*}}/g, " ")
    .replace(/{[a-zA-Z0-9_]+}/g, " ")
    .replace(/<\/?[a-zA-Z0-9]+>/g, " ")
    .replace(/\S+@\S+\.\S+/g, " ")
    .replace(/https?:\/\/\S+/g, " ")
    .replace(/\b[\w-]+\.(app|com|net|org|io|tr)\b\S*/g, " ");
  const words = text.match(/[\p{L}][\p{L}\p{N}.²³-]*/gu) ?? [];
  return words.every((w) => {
    const k = w.toLowerCase().replace(/[.-]+$/, "");
    // Tek harf ya da tamamı büyük harf kısaltma (ABC, TR34) ve sayı+birim (10mm) evrenseldir.
    return UNIVERSAL_WORDS.has(k) || k.length <= 1 || /^[A-Z0-9][A-Z0-9_-]+$/.test(w);
  });
}

/** Allowlist: { "<alan>": { "<dil>|*": { "<anahtar>": "gerekçe" } } } */
function allowed(area, locale, key) {
  const a = allowlist[area] ?? allowlist[area.replace(/-.*/, "")] ?? {};
  return Boolean(a["*"]?.[key] ?? a[locale]?.[key]);
}

const PLURAL_RE = /_(zero|one|two|few|many|other)$/;
const params = (s) =>
  [...String(s).matchAll(/{{\s*([\w.]+)\s*(?:,[^}]*)?}}/g)].map((m) => m[1]).sort().join(",");

function pluralCategories(locale) {
  try {
    return new Intl.PluralRules(locale).resolvedOptions().pluralCategories;
  } catch {
    return ["one", "other"];
  }
}

function flatten(value, prefix = "", out = {}) {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    for (const [k, v] of Object.entries(value)) flatten(v, prefix ? `${prefix}.${k}` : k, out);
  } else if (prefix) {
    out[prefix] = value == null ? "" : String(value);
  }
  return out;
}

function readJsonCatalogs(dir, locale) {
  const out = {};
  const d = path.join(dir, locale);
  if (!fs.existsSync(d)) return null;
  for (const name of fs.readdirSync(d).filter((n) => n.endsWith(".json")).sort()) {
    const ns = name.slice(0, -5);
    const flat = flatten(JSON.parse(fs.readFileSync(path.join(d, name), "utf8")));
    for (const [k, v] of Object.entries(flat)) out[`${ns}:${k}`] = v;
  }
  return out;
}

/** map[string]string literal named varName in platform/i18n. */
function readGoCatalog(varName, locale) {
  const own = locale ? [`catalog_${locale.toLowerCase().replace(/-/g, "_")}.go`] : [];
  for (const file of [...own, "catalog_locales.go", "catalog.go"]) {
    const full = path.join(BACKEND_I18N, file);
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
      if (src[i] === "`") {
        for (i++; i < src.length && src[i] !== "`"; i++);
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

/** Bir dili kaynağa (en, tr) karşı karşılaştırır. */
function compareCatalog(area, locale, en, tr, cat) {
  const out = Object.fromEntries(KINDS.map((k) => [k, []]));
  if (!cat) {
    out.missing.push(...Object.keys(en));
    return out;
  }
  const cats = new Set(pluralCategories(locale));
  const pluralBases = new Map();
  for (const k of Object.keys(en)) {
    const m = PLURAL_RE.exec(k);
    if (!m || m[1] === "other") continue;
    // Gerçek çoğul grubu: _other'ın yanında en az bir kardeş biçim (find_other tek başına çoğul değil).
    if (`${k.slice(0, m.index)}_other` in en) pluralBases.set(k.slice(0, m.index), true);
  }
  for (const [k, v] of Object.entries(en)) {
    const m = PLURAL_RE.exec(k);
    const base = m ? k.slice(0, m.index) : null;
    if (base && pluralBases.has(base)) continue; // çoğul grupları aşağıda
    const got = cat[k];
    if (got == null || !String(got).trim()) {
      out.missing.push(k);
      continue;
    }
    checkValue(area, locale, k, v, tr?.[k], got, out);
  }
  for (const base of pluralBases.keys()) {
    const forms = [...cats].map((c) => `${base}_${c}`);
    const has = forms.filter((f) => cat[f] != null && String(cat[f]).trim());
    if (!has.length) {
      out.missing.push(`${base}_other`);
      continue;
    }
    const lacking = forms.filter((f) => !has.includes(f));
    if (lacking.length) out.plural.push(`${base} (${lacking.map((f) => f.slice(base.length + 1)).join("/")})`);
    for (const f of has) {
      const src = f in en ? f : `${base}_other`;
      checkValue(area, locale, f, en[src], tr?.[f] ?? tr?.[src], cat[f], out, src);
    }
  }
  return out;
}

function checkValue(area, locale, key, enValue, trValue, got, out, srcKey = key) {
  // Çoğul biçimler: "one" biçiminde {{count}} bilinçli olarak düşebilir (ör. "Bir öğe").
  const p = params(got);
  const pe = params(enValue);
  if (p !== pe) {
    const isPluralForm = PLURAL_RE.test(key);
    const withoutCount = (s) => s.split(",").filter((x) => x && x !== "count").join(",");
    if (!(isPluralForm && withoutCount(p) === withoutCount(pe))) out.placeholder.push(key);
  }
  if (locale === "en") return;
  const g = String(got).trim();
  const sameEn = g === String(enValue ?? "").trim();
  const sameTr = locale !== "tr" && trValue != null && g === String(trValue).trim();
  if ((sameEn || sameTr) && !isUniversal(g) && !allowed(area, locale, srcKey) && !allowed(area, locale, key)) {
    out.untranslated.push(key);
  }
}

// --- backend-texts: dil anahtarlı Go haritaları ------------------------------
/** Go sabit adları locale.go'dan: "zh-CN" -> LocaleZhCN. */
const GO_LOCALE_CONST = Object.fromEntries(
  [
    ...fs
      .readFileSync(path.join(BACKEND_I18N, "locale.go"), "utf8")
      .matchAll(/(Locale[A-Za-z]+)\s+Locale = "([^"]+)"/g),
  ].map((m) => [m[2], m[1]]),
);

const LOCALE_KEY_RES = (locale) => {
  const go = GO_LOCALE_CONST[locale] ?? `Locale${locale.replace("-", "").toUpperCase()}`;
  const goAlt = go;
  const esc = locale.replace("-", "[-_]");
  return [
    new RegExp(String.raw`(?:^|[\s{,])"${esc}"\s*:`, "gm"),
    new RegExp(String.raw`\b(?:i18n\.)?(?:${go}|${goAlt})\s*:`, "g"),
    new RegExp(String.raw`\bcatalogs\[(?:i18n\.)?(?:${go}|${goAlt})\]`, "g"),
  ];
};

function walkGo(dir, out = []) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) {
      if (["testdata", "vendor", "generated"].includes(e.name)) continue;
      walkGo(p, out);
    } else if (e.name.endsWith(".go") && !e.name.endsWith("_test.go") && !e.name.endsWith(".sql.go")) {
      out.push(p);
    }
  }
  return out;
}

function countLocaleKeys(src, locale) {
  return LOCALE_KEY_RES(locale).reduce((n, re) => n + (src.match(re)?.length ?? 0), 0);
}

function scanBackendTexts(locales) {
  const files = walkGo(BACKEND_SRC).filter(
    (f) => !/platform\/i18n\/catalog(_[a-z_]+)?\.go$/.test(f) && !f.endsWith("catalog_locales.go"),
  );
  const res = Object.fromEntries(locales.map((l) => [l, Object.fromEntries(KINDS.map((k) => [k, []]))]));
  for (const f of files) {
    const src = fs.readFileSync(f, "utf8");
    const en = countLocaleKeys(src, "en");
    const tr = countLocaleKeys(src, "tr");
    // Yalnız hem tr hem en metni tutan (çok dilli) dosyalar.
    if (en < 1 || tr < 1) continue;
    const rel = path.relative(ROOT, f);
    for (const l of locales) {
      if (l === "en") continue;
      const n = countLocaleKeys(src, l);
      if (n < en && !allowed("backend-texts", l, rel)) {
        res[l].missing.push(`${rel} (${n}/${en})`);
      }
    }
  }
  return res;
}

// --- dil listeleri tutarlı mı? -----------------------------------------------
function localeListFindings() {
  const out = [];
  const mobileList = fs.readFileSync(path.join(ROOT, "mobile/scripts/i18n-languages.mjs"), "utf8");
  const mob = [...(mobileList.match(/\[([^\]]*)\]/)?.[1] ?? "").matchAll(/'([^']+)'/g)].map((m) => m[1]);
  const goSrc = fs.readFileSync(path.join(BACKEND_I18N, "locale.go"), "utf8");
  const goSupported = goSrc.match(/var Supported = \[\]Locale\{([\s\S]*?)\}/)?.[1] ?? "";
  const goConsts = Object.fromEntries(
    [...goSrc.matchAll(/(Locale[A-Za-z]+)\s+Locale = "([^"]+)"/g)].map((m) => [m[1], m[2]]),
  );
  const be = [...goSupported.matchAll(/Locale[A-Za-z]+/g)].map((m) => goConsts[m[0]]).filter(Boolean);
  for (const [name, list] of [
    ["mobile/scripts/i18n-languages.mjs", mob],
    ["backend/internal/platform/i18n/locale.go Supported", be],
  ]) {
    const miss = SUPPORTED.filter((l) => !list.includes(l));
    const extra = list.filter((l) => !SUPPORTED.includes(l));
    if (miss.length || extra.length) {
      out.push(`${name}: eksik [${miss.join(", ")}] fazla [${extra.join(", ")}]`);
    }
  }
  for (const l of SUPPORTED) {
    if (!fs.existsSync(path.join(MOBILE_DIR, l))) out.push(`mobil katalog klasörü yok: ${l}`);
    if (!fs.existsSync(path.join(WEB_DIR, l))) out.push(`web katalog klasörü yok: ${l}`);
  }
  return out;
}

// --- çalıştır ----------------------------------------------------------------
export function scan(locales = selected) {
  const report = {};
  const webEn = readJsonCatalogs(WEB_DIR, "en");
  const webTr = readJsonCatalogs(WEB_DIR, "tr");
  const mobEn = readJsonCatalogs(MOBILE_DIR, "en");
  const mobTr = readJsonCatalogs(MOBILE_DIR, "tr");
  const beEn = readGoCatalog("enCatalog") ?? {};
  const beTr = readGoCatalog("trCatalog") ?? {};
  const texts = scanBackendTexts(locales);
  for (const l of locales) {
    report[l] = {
      web: compareCatalog("web", l, webEn, webTr, l === "en" ? webEn : readJsonCatalogs(WEB_DIR, l)),
      mobile: compareCatalog("mobile", l, mobEn, mobTr, l === "en" ? mobEn : readJsonCatalogs(MOBILE_DIR, l)),
      "backend-labels": compareCatalog(
        "backend-labels",
        l,
        beEn,
        beTr,
        l === "en" ? beEn : l === "tr" ? beTr : readGoCatalog(goVar(l), l),
      ),
      "backend-texts": texts[l],
    };
  }
  return { locales, report, localeLists: localeListFindings() };
}

const isMain = process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (isMain) {
  const { report, localeLists } = scan();
  const total = (r) => KINDS.reduce((n, k) => n + r[k].length, 0);
  let findings = localeLists.length;
  for (const l of selected) for (const a of AREAS) findings += total(report[l][a]);

  if (flag("--json")) {
    console.log(JSON.stringify({ ok: findings === 0, localeLists, report }, null, 2));
  } else {
    const cell = (r) => (total(r) ? KINDS.map((k) => r[k].length).join("/") : "0");
    if (flag("--markdown")) {
      console.log("Hücre: eksik/çevrilmemiş/yer tutucu/çoğul (0 = temiz)\n");
      console.log(`| dil | ${AREAS.join(" | ")} |`);
      console.log(`|---|${AREAS.map(() => "---").join("|")}|`);
      for (const l of selected) console.log(`| ${l} | ${AREAS.map((a) => cell(report[l][a])).join(" | ")} |`);
    } else {
      console.log("i18n taraması — hücre: eksik/çevrilmemiş/yer tutucu/çoğul (0 = temiz)\n");
      console.log(`${"dil".padEnd(7)}${AREAS.map((a) => a.padEnd(18)).join("")}`);
      for (const l of selected) {
        console.log(`${l.padEnd(7)}${AREAS.map((a) => cell(report[l][a]).padEnd(18)).join("")}`);
      }
    }
    for (const f of localeLists) console.log(`\nlocale-list: ${f}`);
    if (flag("--list")) {
      for (const l of selected) {
        for (const a of AREAS) {
          for (const k of KINDS) {
            const list = report[l][a][k];
            if (!list.length) continue;
            console.log(`\n${l} ${a} ${k} (${list.length}):`);
            for (const key of list) console.log(`  - ${key}`);
          }
        }
      }
    }
    console.log(findings ? `\n${findings} bulgu.` : "\nTemiz: bulgu yok.");
  }
  process.exit(flag("--fail") && findings ? 1 : 0);
}
