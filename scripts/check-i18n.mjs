#!/usr/bin/env node
/**
 * System-wide i18n linter.
 *
 * Catches keys that leak into the UI as raw slugs (e.g. activity log showing
 * `integrations.github.updated` instead of a human label).
 *
 * Checks:
 *   - en/tr locale JSON parity, empty / slug-like values
 *   - messages.ts catalog wiring
 *   - t() / tp() / labelKey / titleKey usages in frontend
 *   - activity.Record(action, resource) → activity.actions.* / activity.resources.*
 *   - RESOURCE_LABEL_KEYS wiring (UI otherwise prints the raw resource)
 *   - permission slugs → permissions.labels.*
 *   - ioengine LabelKey → backend catalog (PDF/CSV/XLSX)
 *   - bulkengine / searchengine LabelKey → frontend locales
 *   - backend i18n catalog en/tr parity
 *   - languages from frontend/src/config/i18n.ts (SUPPORTED_LOCALES, 13):
 *     tr/en must match key for key (error); any other language may miss
 *     keys (warning, error with --strict) but never has extra keys
 *   - {{param}} sets equal to en in every language
 *   - per-language catalog wiring (locales/<lang>/index.ts, catalog-loaders)
 *   - physical Tailwind utilities (ml-/pr-/left-/text-left...) are banned:
 *     RTL needs logical ones (TEC-137, K10)
 *
 * Usage (repo root):
 *   node scripts/check-i18n.mjs
 *   node scripts/check-i18n.mjs --json
 *   node scripts/check-i18n.mjs --strict
 *   make check-i18n
 */

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  findPhysicalClasses,
  isPhysicalClassIgnored,
} from "../frontend/scripts/physical-classes.mjs";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const I18N_CONFIG = path.join(ROOT, "frontend/src/config/i18n.ts");
/** tr and en are complete: every key in one must exist in the other. */
const REQUIRED_LOCALES = ["en", "tr"];
const LOCALES = readSupportedLocales();
/** Languages that may lag behind en (TEC-138 translates them). */
const PARTIAL_LOCALES = LOCALES.filter((l) => !REQUIRED_LOCALES.includes(l));
const LOCALES_DIR = path.join(ROOT, "frontend/src/locales");
const CATALOG_LOADERS = path.join(ROOT, "frontend/src/lib/i18n/catalog-loaders.ts");
const BACKEND_CATALOG = path.join(ROOT, "backend/internal/platform/i18n/catalog.go");
const BACKEND_CATALOG_LOCALES = path.join(
  ROOT,
  "backend/internal/platform/i18n/catalog_locales.go",
);
const RBAC_FILE = path.join(ROOT, "backend/internal/platform/rbac/rbac.go");
const PERMISSIONS_TS = path.join(ROOT, "frontend/src/config/permissions.ts");
const IO_TYPES = path.join(ROOT, "frontend/src/features/io/types.ts");
const IO_DISPLAY = path.join(ROOT, "frontend/src/features/io/lib/display.ts");
const PERM_GROUPS = path.join(
  ROOT,
  "frontend/src/components/permissions/permission-groups.ts",
);

const IGNORE_DIRS = new Set([
  "node_modules",
  ".next",
  "dist",
  "generated",
  "vendor",
  "coverage",
  "testdata",
  ".git",
]);

const KEY_RE = /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/;
const SLUG_VALUE_RE = /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/;
const TURKISH_RE = /[çğıöşüÇĞİÖŞÜ]/;

const argv = new Set(process.argv.slice(2));
const WANT_JSON = argv.has("--json");
const STRICT = argv.has("--strict");
const WANT_UNUSED = argv.has("--unused");
const VERBOSE = argv.has("--verbose");

if (argv.has("--help") || argv.has("-h")) {
  console.log(`check-i18n — missing / unwired translation keys

Usage:
  node scripts/check-i18n.mjs [--json] [--strict] [--unused] [--verbose]

  --json     machine-readable report
  --strict   treat warnings as errors (a language missing keys fails)
  --unused   also report locale keys that nothing references
  --verbose  list missing keys per namespace for partial languages
`);
  process.exit(0);
}

const findings = [];

function readSupportedLocales() {
  const src = fs.readFileSync(I18N_CONFIG, "utf8");
  const m = src.match(/SUPPORTED_LOCALES\s*=\s*\[([\s\S]*?)\]/);
  if (!m) {
    console.error(`Cannot read SUPPORTED_LOCALES from ${I18N_CONFIG}`);
    process.exit(2);
  }
  return [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
}

const PARAM_RE = /{{\s*([a-zA-Z0-9_]+)\s*}}/g;

function paramSet(text) {
  return [...new Set([...String(text ?? "").matchAll(PARAM_RE)].map((m) => m[1]))]
    .sort()
    .join(",");
}

/** Go identifier of a locale's catalog map: zh-CN -> zhCNCatalog. */
function goCatalogVar(locale) {
  return `${locale.replace(/-([A-Za-z]+)/g, (_, r) => r)}Catalog`;
}

function parseGoCatalog(src, varName) {
  const re = new RegExp(String.raw`\b${varName}\s*=\s*map\[string\]string\{`);
  const m = re.exec(src);
  if (!m) return null;
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

function add(level, code, message, extra = {}) {
  findings.push({ level, code, message, ...extra });
}

function rel(file) {
  return path.relative(ROOT, file).replaceAll(path.sep, "/");
}

function readFile(file) {
  return fs.readFileSync(file, "utf8");
}

function walk(dir, exts) {
  const out = [];
  if (!fs.existsSync(dir)) return out;
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    if (IGNORE_DIRS.has(ent.name) || (ent.name.startsWith(".") && ent.isDirectory())) {
      continue;
    }
    const p = path.join(dir, ent.name);
    if (ent.isDirectory()) out.push(...walk(p, exts));
    else if (exts.has(path.extname(ent.name))) out.push(p);
  }
  return out;
}

function flatten(value, prefix = "") {
  const out = {};
  if (value !== null && typeof value === "object" && !Array.isArray(value)) {
    for (const [k, v] of Object.entries(value)) {
      Object.assign(out, flatten(v, prefix ? `${prefix}.${k}` : k));
    }
    return out;
  }
  if (prefix) out[prefix] = value == null ? "" : String(value);
  return out;
}

function parseLocaleDir(locale) {
  const dir = path.join(LOCALES_DIR, locale);
  const namespaces = {};
  if (!fs.existsSync(dir)) {
    add("error", "locale-dir-missing", `Locale directory missing: ${rel(dir)}`);
    return namespaces;
  }
  for (const name of fs.readdirSync(dir).filter((n) => n.endsWith(".json"))) {
    const file = path.join(dir, name);
    const ns = name.slice(0, -5);
    try {
      const parsed = JSON.parse(readFile(file));
      if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
        add("error", "locale-json-shape", `Expected object in ${rel(file)}`);
        namespaces[ns] = { file, keys: {} };
        continue;
      }
      namespaces[ns] = { file, keys: flatten(parsed) };
    } catch (err) {
      add("error", "locale-json-parse", `Invalid JSON ${rel(file)}: ${err.message}`);
      namespaces[ns] = { file, keys: {} };
    }
  }
  return namespaces;
}

function frontendLookup(catalogs, locale, fullKey) {
  const dot = fullKey.indexOf(".");
  if (dot === -1) {
    return { ok: false, ns: fullKey, rest: "", value: undefined };
  }
  const ns = fullKey.slice(0, dot);
  const rest = fullKey.slice(dot + 1);
  const value = catalogs[locale]?.[ns]?.keys?.[rest];
  const ok = typeof value === "string" && value.trim() !== "";
  return { ok, ns, rest, value };
}

/** Namespaces a locales/<lang>/index.ts imports and registers. */
function parseLocaleIndex(src) {
  const files = new Set();
  for (const m of src.matchAll(/from\s+["']\.\/([a-z0-9_-]+)\.json["']/g)) {
    files.add(m[1]);
  }
  const namespaces = new Set();
  const block = src.match(/const catalog[^=]*=\s*\{([\s\S]*?)\};/);
  if (block) {
    for (const m of block[1].matchAll(/^\s*"?([a-z][a-z0-9_-]*)"?\s*[:,]/gm)) {
      namespaces.add(m[1]);
    }
  }
  return { files, namespaces };
}

function parseGoStringMap(src, varName) {
  const re = new RegExp(
    String.raw`var ${varName}\s*=\s*map\[string\]string\{([\s\S]*?)\n\}`,
  );
  const m = src.match(re);
  if (!m) return {};
  const map = {};
  for (const hit of m[1].matchAll(/"((?:\\.|[^"\\])*)"\s*:\s*"((?:\\.|[^"\\])*)"/g)) {
    map[JSON.parse(`"${hit[1]}"`)] = JSON.parse(`"${hit[2]}"`);
  }
  return map;
}

function unquoteGo(token) {
  const s = token.trim();
  if (s.length >= 2 && s.startsWith('"') && s.endsWith('"')) {
    try {
      return JSON.parse(s);
    } catch {
      return s.slice(1, -1);
    }
  }
  if (s.length >= 2 && s.startsWith("`") && s.endsWith("`")) return s.slice(1, -1);
  return null;
}

function splitCallArgs(src, openParenIdx) {
  const args = [];
  let current = "";
  let depthParen = 1;
  let depthBrace = 0;
  let depthBrack = 0;
  let inStr = null;
  for (let i = openParenIdx + 1; i < src.length && depthParen > 0; i++) {
    const c = src[i];
    const prev = src[i - 1];
    if (inStr) {
      current += c;
      if (inStr === "`" && c === "`") inStr = null;
      else if (inStr !== "`" && c === inStr && prev !== "\\") inStr = null;
      continue;
    }
    if (c === '"' || c === "'" || c === "`") {
      inStr = c;
      current += c;
      continue;
    }
    if (c === "(") depthParen++;
    if (c === ")") {
      depthParen--;
      if (depthParen === 0) {
        if (current.trim()) args.push(current.trim());
        break;
      }
    }
    if (c === "{") depthBrace++;
    if (c === "}") depthBrace--;
    if (c === "[") depthBrack++;
    if (c === "]") depthBrack--;
    if (c === "," && depthParen === 1 && depthBrace === 0 && depthBrack === 0) {
      args.push(current.trim());
      current = "";
      continue;
    }
    current += c;
  }
  return args;
}

function lineOf(src, idx) {
  return src.slice(0, idx).split("\n").length;
}

function pushLoc(map, key, loc) {
  if (!KEY_RE.test(key)) return;
  if (!map.has(key)) map.set(key, []);
  map.get(key).push(loc);
}

function collectActivityRecords(goFiles) {
  const actions = new Map();
  const resources = new Map();
  for (const file of goFiles) {
    if (file.endsWith("_test.go")) continue;
    const src = readFile(file);
    let from = 0;
    while (true) {
      const idx = src.indexOf(".Record(", from);
      if (idx === -1) break;
      from = idx + 8;
      const args = splitCallArgs(src, idx + ".Record".length);
      if (args.length < 4) continue;
      const action = unquoteGo(args[2]);
      const resource = unquoteGo(args[3]);
      const loc = `${rel(file)}:${lineOf(src, idx)}`;
      if (action) pushLoc(actions, action, loc);
      if (resource) pushLoc(resources, resource, loc);
    }
  }
  return { actions, resources };
}

function collectLabelKeys(goFiles) {
  const io = new Map();
  const bulk = new Map();
  const search = new Map();
  const meta = new Map();
  const translate = new Map();
  for (const file of goFiles) {
    if (file.endsWith("_test.go")) continue;
    const src = readFile(file);
    const locFor = (idx) => `${rel(file)}:${lineOf(src, idx)}`;
    const norm = file.replaceAll(path.sep, "/");
    for (const m of src.matchAll(/\bLabelKey:\s*"([^"]+)"/g)) {
      const key = m[1];
      const loc = locFor(m.index ?? 0);
      if (norm.includes("/ioengine/")) pushLoc(io, key, loc);
      else if (norm.includes("/bulkengine/")) pushLoc(bulk, key, loc);
      else if (norm.includes("/searchengine/")) pushLoc(search, key, loc);
      else pushLoc(meta, key, loc);
    }
    for (const m of src.matchAll(/\bi18n\.Translate\(\s*\w+\s*,\s*"([^"]+)"/g)) {
      pushLoc(translate, m[1], locFor(m.index ?? 0));
    }
  }
  return { io, bulk, search, meta, translate };
}

function collectStaticFrontendKeys(tsFiles) {
  const used = new Map();
  const fields = new Map();
  const dynamic = [];
  for (const file of tsFiles) {
    if (file.endsWith(".d.ts")) continue;
    if (file.includes(`${path.sep}locales${path.sep}`)) continue;
    const src = readFile(file);
    src.split("\n").forEach((line, i) => {
      const n = i + 1;
      const trimmed = line.trim();
      if (trimmed.startsWith("//")) return;
      for (const m of line.matchAll(
        /\b(?:t|tp)\(\s*(?:`([^`$]+)`|'([^']+)'|"([^"]+)")/g,
      )) {
        pushLoc(used, m[1] || m[2] || m[3], `${rel(file)}:${n}`);
      }
      for (const m of line.matchAll(
        /\b(?:labelKey|titleKey|label_key|title_key)\s*:\s*(?:`([^`$]+)`|'([^']+)'|"([^"]+)")/g,
      )) {
        pushLoc(fields, m[1] || m[2] || m[3], `${rel(file)}:${n}`);
      }
      for (const m of line.matchAll(/\b(?:t|tp)\(\s*`([^`]*\$\{)/g)) {
        dynamic.push({
          prefix: m[1].replace(/\$\{$/, ""),
          at: `${rel(file)}:${n}`,
        });
      }
      for (const m of line.matchAll(
        /\b(?:labelKey|titleKey)\s*:\s*`([^`]*\$\{)/g,
      )) {
        dynamic.push({
          prefix: m[1].replace(/\$\{$/, ""),
          at: `${rel(file)}:${n}`,
        });
      }
    });
  }
  return { used, fields, dynamic };
}

function looksUntranslated(key, value) {
  const v = String(value ?? "").trim();
  if (!v) return "empty";
  if (v === key) return "equals-key";
  const rest = key.includes(".") ? key.slice(key.indexOf(".") + 1) : key;
  if (v === rest && rest.includes(".")) return "equals-path";
  if (SLUG_VALUE_RE.test(v) && v.includes(".") && v === v.toLowerCase()) {
    return "value-is-slug";
  }
  return null;
}

function likelyEnglish(text) {
  if (TURKISH_RE.test(text)) return false;
  if (!/[a-zA-Z]/.test(text)) return false;
  if (!text.includes(" ")) return false;
  return text.length > 12;
}

function unionStrings(src, typeName) {
  const m = src.match(
    new RegExp(String.raw`(?:export\s+)?type\s+${typeName}\s*=\s*([\s\S]*?);`),
  );
  if (!m) return [];
  return [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
}

function objectKeys(src, ident) {
  const m = src.match(
    new RegExp(String.raw`(?:const|let)\s+${ident}\s*(?::[^=]+)?=\s*\{([\s\S]*?)\}`),
  );
  if (!m) return [];
  return [...m[1].matchAll(/["']([^"']+)["']\s*:/g)].map((x) => x[1]);
}

function collectQuoted(src, regex) {
  return [...src.matchAll(regex)].map((m) => m[1]);
}

function requireKey(map, key, at, why) {
  if (!key || !KEY_RE.test(key)) return;
  if (!map.has(key)) map.set(key, { at: [], why });
  const rec = map.get(key);
  for (const loc of [].concat(at ?? [])) {
    if (loc) rec.at.push(loc);
  }
}

function groupFindings(items) {
  const map = new Map();
  for (const item of items) {
    const list = map.get(item.code) ?? [];
    list.push(item);
    map.set(item.code, list);
  }
  return map;
}

function paint(code, text) {
  return process.stdout.isTTY ? `\x1b[${code}m${text}\x1b[0m` : text;
}

function printHuman() {
  const errors = findings.filter((f) => f.level === "error");
  const warnings = findings.filter((f) => f.level === "warning");

  const show = (items, title, color) => {
    if (!items.length) return;
    console.log(paint(color, `\n${title} (${items.length})`));
    for (const [code, list] of groupFindings(items)) {
      console.log(paint(color, `  [${code}]`));
      for (const item of list) {
        const at = [].concat(item.at ?? []).filter(Boolean);
        const shown = at.slice(0, 4).join(", ");
        const more = at.length > 4 ? ` (+${at.length - 4} more)` : "";
        const where = shown ? `  @ ${shown}${more}` : "";
        console.log(`    - ${item.message}${where}`);
      }
    }
  };

  show(errors, "ERRORS", "31");
  show(warnings, "WARNINGS", "33");
  console.log("");
  if (!errors.length && !warnings.length) {
    console.log(paint("32", "i18n check passed — no missing keys."));
    return;
  }
  console.log(
    `i18n check: ${errors.length} error(s), ${warnings.length} warning(s).`,
  );
}

/**
 * Languages beyond tr/en: a missing namespace or key is a warning (one line
 * per language and namespace, --strict makes it an error); a key en does
 * not have is always an error.
 */
function checkPartialLocales(catalogs, enNs) {
  for (const locale of PARTIAL_LOCALES) {
    const cat = catalogs[locale] ?? {};
    let missingTotal = 0;
    let total = 0;
    const gaps = [];
    for (const ns of enNs) {
      const enKeys = Object.keys(catalogs.en[ns]?.keys ?? {});
      const keys = cat[ns]?.keys ?? {};
      const missing = enKeys.filter((k) => !String(keys[k] ?? "").trim());
      total += enKeys.length;
      missingTotal += missing.length;
      if (missing.length) gaps.push(ns);
      if (VERBOSE && missing.length) {
        add(
          "warning",
          "locale-incomplete-namespace",
          `${locale} ${ns}: ${missing.length}/${enKeys.length} keys missing, e.g. ${missing.slice(0, 3).join(", ")}`,
          { at: rel(path.join(LOCALES_DIR, locale, `${ns}.json`)) },
        );
      }
      for (const k of Object.keys(keys)) {
        if (!(k in (catalogs.en[ns]?.keys ?? {}))) {
          add("error", "locale-extra-key", `${locale} has ${ns}.${k}, en does not`, {
            at: rel(cat[ns].file),
          });
        }
      }
    }
    for (const ns of Object.keys(cat)) {
      if (!enNs.has(ns)) {
        add("error", "namespace-locale-gap", `Namespace "${ns}" exists in ${locale} but not en`);
      }
    }
    if (missingTotal) {
      const done = total - missingTotal;
      const pct = total ? Math.round((done / total) * 100) : 0;
      add(
        "warning",
        "locale-incomplete",
        `${locale}: ${done}/${total} keys translated (${pct}%), ${gaps.length} namespace(s) fall back to en (--verbose lists them)`,
        { at: rel(path.join(LOCALES_DIR, locale)) },
      );
    }
  }
}

/** Every translation keeps exactly the {{params}} of the en text. */
function checkParams(catalogs) {
  for (const locale of LOCALES) {
    if (locale === "en") continue;
    for (const [ns, data] of Object.entries(catalogs[locale] ?? {})) {
      const enKeys = catalogs.en[ns]?.keys ?? {};
      for (const [k, v] of Object.entries(data.keys)) {
        if (!(k in enKeys) || !String(v).trim()) continue;
        const want = paramSet(enKeys[k]);
        const got = paramSet(v);
        if (want !== got) {
          add(
            "error",
            "param-mismatch",
            `${locale} ${ns}.${k}: {{params}} [${got}] differ from en [${want}]`,
            { at: rel(data.file) },
          );
        }
      }
    }
  }
}

/** Physical left/right utilities break RTL (K10); see frontend/scripts. */
function checkPhysicalClasses() {
  const frontend = path.join(ROOT, "frontend");
  const files = walk(path.join(frontend, "src"), new Set([".ts", ".tsx", ".css"]));
  for (const file of files) {
    if (file.endsWith(".d.ts")) continue;
    if (isPhysicalClassIgnored(path.relative(frontend, file))) continue;
    for (const hit of findPhysicalClasses(readFile(file))) {
      add(
        "error",
        "physical-class",
        `"${hit.token}" breaks RTL, use "${hit.logical}" (pnpm codemod:logical)`,
        { at: `${rel(file)}:${hit.line}` },
      );
    }
  }
}

function main() {
  const catalogs = Object.fromEntries(LOCALES.map((l) => [l, parseLocaleDir(l)]));

  const enNs = new Set(Object.keys(catalogs.en));
  const trNs = new Set(Object.keys(catalogs.tr));
  for (const ns of enNs) {
    if (!trNs.has(ns)) {
      add("error", "namespace-locale-gap", `Namespace "${ns}" exists in en but not tr`);
    }
  }
  for (const ns of trNs) {
    if (!enNs.has(ns)) {
      add("error", "namespace-locale-gap", `Namespace "${ns}" exists in tr but not en`);
    }
  }

  const allNs = new Set([...enNs, ...trNs]);
  for (const ns of allNs) {
    const enKeys = catalogs.en[ns]?.keys ?? {};
    const trKeys = catalogs.tr[ns]?.keys ?? {};
    for (const k of Object.keys(enKeys)) {
      if (!(k in trKeys)) {
        add("error", "key-locale-gap", `tr missing ${ns}.${k}`, {
          at: catalogs.en[ns] && rel(catalogs.en[ns].file),
        });
      }
      const reason = looksUntranslated(`${ns}.${k}`, enKeys[k]);
      if (reason) {
        add("error", "untranslated-value", `en ${ns}.${k} (${reason})`, {
          at: catalogs.en[ns] && rel(catalogs.en[ns].file),
        });
      }
    }
    for (const k of Object.keys(trKeys)) {
      if (!(k in enKeys)) {
        add("error", "key-locale-gap", `en missing ${ns}.${k}`, {
          at: catalogs.tr[ns] && rel(catalogs.tr[ns].file),
        });
      }
      const reason = looksUntranslated(`${ns}.${k}`, trKeys[k]);
      if (reason) {
        add("error", "untranslated-value", `tr ${ns}.${k} (${reason})`, {
          at: catalogs.tr[ns] && rel(catalogs.tr[ns].file),
        });
      }
      if (
        typeof enKeys[k] === "string" &&
        enKeys[k] &&
        enKeys[k] === trKeys[k] &&
        likelyEnglish(enKeys[k])
      ) {
        add("warning", "en-tr-identical", `en/tr identical English copy: ${ns}.${k}`, {
          at: catalogs.tr[ns] && rel(catalogs.tr[ns].file),
        });
      }
    }
  }

  checkPartialLocales(catalogs, enNs);
  checkParams(catalogs);

  const wired = { namespaces: new Set() };
  const loadersSrc = fs.existsSync(CATALOG_LOADERS) ? readFile(CATALOG_LOADERS) : "";
  if (!loadersSrc) {
    add("error", "catalog-loaders-missing", `Cannot read ${rel(CATALOG_LOADERS)}`);
  }
  for (const locale of LOCALES) {
    if (loadersSrc && !loadersSrc.includes(`import("@/locales/${locale}")`)) {
      add(
        "error",
        "unwired-locale",
        `${rel(CATALOG_LOADERS)} has no loader for ${locale} (pnpm i18n:gen)`,
      );
    }
    const indexFile = path.join(LOCALES_DIR, locale, "index.ts");
    if (!fs.existsSync(indexFile)) {
      add("error", "unwired-locale", `Missing ${rel(indexFile)} (pnpm i18n:gen)`);
      continue;
    }
    const index = parseLocaleIndex(readFile(indexFile));
    if (locale === "en") index.namespaces.forEach((ns) => wired.namespaces.add(ns));
    for (const ns of Object.keys(catalogs[locale] ?? {})) {
      if (!index.files.has(ns) || !index.namespaces.has(ns)) {
        add(
          REQUIRED_LOCALES.includes(locale) ? "error" : "warning",
          "unwired-namespace",
          `${locale}/${ns}.json is not registered in ${rel(indexFile)} — t("${ns}.*") cannot resolve it (pnpm i18n:gen)`,
        );
      }
    }
    for (const ns of index.namespaces) {
      if (!catalogs[locale]?.[ns]) {
        add(
          "error",
          "catalog-file-missing",
          `${rel(indexFile)} registers ${ns} but locales/${locale}/${ns}.json is missing`,
        );
      }
    }
  }

  checkPhysicalClasses();

  const backendSrc = fs.existsSync(BACKEND_CATALOG) ? readFile(BACKEND_CATALOG) : "";
  if (!backendSrc) {
    add("error", "backend-catalog-missing", `Cannot read ${rel(BACKEND_CATALOG)}`);
  }
  const backend = {
    en: parseGoStringMap(backendSrc, "enCatalog"),
    tr: parseGoStringMap(backendSrc, "trCatalog"),
  };
  if (backendSrc && !Object.keys(backend.en).length) {
    add("error", "backend-catalog-parse", "Failed to parse enCatalog from catalog.go");
  }
  if (backendSrc && !Object.keys(backend.tr).length) {
    add("error", "backend-catalog-parse", "Failed to parse trCatalog from catalog.go");
  }
  for (const k of Object.keys(backend.en)) {
    if (!(k in backend.tr)) add("error", "backend-locale-gap", `tr catalog missing ${k}`);
    const reason = looksUntranslated(k, backend.en[k]);
    if (reason) add("error", "backend-untranslated", `en catalog ${k} (${reason})`);
  }
  for (const k of Object.keys(backend.tr)) {
    if (!(k in backend.en)) add("error", "backend-locale-gap", `en catalog missing ${k}`);
    const reason = looksUntranslated(k, backend.tr[k]);
    if (reason) add("error", "backend-untranslated", `tr catalog ${k} (${reason})`);
    if (backend.en[k] && backend.en[k] === backend.tr[k] && likelyEnglish(backend.en[k])) {
      add("warning", "backend-en-tr-identical", `en/tr identical: ${k}`);
    }
  }

  const backendLocalesSrc = fs.existsSync(BACKEND_CATALOG_LOCALES)
    ? readFile(BACKEND_CATALOG_LOCALES)
    : "";
  for (const locale of PARTIAL_LOCALES) {
    const map =
      parseGoCatalog(backendLocalesSrc, goCatalogVar(locale)) ??
      parseGoCatalog(backendSrc, goCatalogVar(locale));
    if (!map) {
      add(
        "warning",
        "backend-locale-catalog-missing",
        `Backend catalog ${goCatalogVar(locale)} not found (falls back to en)`,
      );
      continue;
    }
    const missing = Object.keys(backend.en).filter((k) => !map[k]?.trim());
    if (missing.length) {
      add(
        "warning",
        "backend-locale-incomplete",
        `${locale} catalog: ${missing.length}/${Object.keys(backend.en).length} keys missing (fall back to en)`,
      );
    }
    for (const [k, v] of Object.entries(map)) {
      if (!(k in backend.en)) {
        add("error", "backend-locale-extra", `${locale} catalog has ${k}, en does not`);
      } else if (paramSet(v) !== paramSet(backend.en[k])) {
        add("error", "backend-param-mismatch", `${locale} catalog ${k}: {{params}} differ from en`);
      }
    }
  }

  const goFiles = walk(path.join(ROOT, "backend"), new Set([".go"]));
  const tsFiles = walk(path.join(ROOT, "frontend/src"), new Set([".ts", ".tsx"]));
  const activity = collectActivityRecords(goFiles);
  const labels = collectLabelKeys(goFiles);
  const feKeys = collectStaticFrontendKeys(tsFiles);

  const requiredFrontend = new Map();
  const requiredBackend = new Map();

  for (const [key, at] of feKeys.used) requireKey(requiredFrontend, key, at, "t()/tp()");
  for (const [key, at] of feKeys.fields) {
    requireKey(requiredFrontend, key, at, "labelKey/titleKey");
  }

  for (const [action, at] of activity.actions) {
    requireKey(requiredFrontend, `activity.actions.${action}`, at, "activity.Record action");
  }
  for (const [resource, at] of activity.resources) {
    requireKey(
      requiredFrontend,
      `activity.resources.${resource}`,
      at,
      "activity.Record resource",
    );
  }

  const ioTypesSrc = fs.existsSync(IO_TYPES) ? readFile(IO_TYPES) : "";
  const ioResources = unionStrings(ioTypesSrc, "IoResource");
  for (const resource of ioResources) {
    requireKey(requiredFrontend, `activity.resources.${resource}`, rel(IO_TYPES), "IoResource");
    requireKey(requiredFrontend, `exports.resources.${resource}`, rel(IO_TYPES), "IoResource export UI");
    requireKey(requiredBackend, `resources.${resource}`, rel(IO_TYPES), "IoResource export label");
    requireKey(requiredBackend, `export.title.${resource}`, rel(IO_TYPES), "IoResource export title");
  }
  for (const resource of objectKeys(ioTypesSrc, "IMPORT_PATHS")) {
    requireKey(requiredFrontend, `imports.resources.${resource}`, rel(IO_TYPES), "IoResource import UI");
  }

  const displaySrc = fs.existsSync(IO_DISPLAY) ? readFile(IO_DISPLAY) : "";
  const mappedResources = new Set(objectKeys(displaySrc, "RESOURCE_LABEL_KEYS"));
  const usesActivityResourceFallback = displaySrc.includes("formatActivityResource");
  for (const [resource, at] of activity.resources) {
    const activityKey = `activity.resources.${resource}`;
    const hasActivityLocale = REQUIRED_LOCALES.every(
      (loc) => frontendLookup(catalogs, loc, activityKey).ok,
    );
    if (hasActivityLocale) continue;
    if (mappedResources.has(resource) || ioResources.includes(resource)) continue;
    if (usesActivityResourceFallback) {
      add(
        "error",
        "activity-resource-unwired",
        `Activity resource "${resource}" has no activity.resources.${resource} in en/tr locales`,
        { at },
      );
      continue;
    }
    add(
      "error",
      "activity-resource-unwired",
      `Activity resource "${resource}" is recorded but not in RESOURCE_LABEL_KEYS — add activity.resources.${resource} or wire RESOURCE_LABEL_KEYS`,
      { at },
    );
  }

  const rbacSrc = fs.existsSync(RBAC_FILE) ? readFile(RBAC_FILE) : "";
  const permTsSrc = fs.existsSync(PERMISSIONS_TS) ? readFile(PERMISSIONS_TS) : "";
  const permSlugs = new Set([
    ...collectQuoted(
      rbacSrc,
      /Perm[A-Za-z0-9]+\s*=\s*"((?:platform|auth|notifications)\.[^"]+)"/g,
    ),
    ...collectQuoted(
      permTsSrc,
      /:\s*"((?:platform|auth|notifications)\.[^"]+)"/g,
    ),
  ]);
  for (const slug of permSlugs) {
    requireKey(
      requiredFrontend,
      `permissions.labels.${slug}`,
      [rel(RBAC_FILE), rel(PERMISSIONS_TS)],
      "permission slug",
    );
  }

  const groupKeys = new Set();
  if (fs.existsSync(PERM_GROUPS)) {
    const gsrc = readFile(PERM_GROUPS);
    const order = gsrc.match(/PERMISSION_GROUP_ORDER\s*=\s*\[([\s\S]*?)\]/);
    if (order) {
      for (const m of order[1].matchAll(/"([a-z0-9_]+)"/g)) groupKeys.add(m[1]);
    }
  }
  for (const slug of permSlugs) groupKeys.add(slug.split(".")[0]);
  for (const g of groupKeys) {
    requireKey(requiredFrontend, `permissions.groups.${g}`, rel(PERM_GROUPS), "permission group");
  }

  const purposes = new Set();
  for (const file of tsFiles) {
    if (file.endsWith(".d.ts")) continue;
    const src = readFile(file);
    for (const m of src.matchAll(/\bpurpose\s*=\s*["']([^"']+)["']/g)) {
      purposes.add(m[1]);
    }
  }
  for (const purpose of purposes) {
    requireKey(
      requiredFrontend,
      `stepup.gate.purpose.${purpose}`,
      "purpose= prop",
      "step-up purpose",
    );
  }

  for (const [key, at] of labels.io) requireKey(requiredBackend, key, at, "ioengine LabelKey");
  for (const [key, at] of labels.translate) {
    requireKey(requiredBackend, key, at, "i18n.Translate literal");
  }
  for (const [key, at] of labels.bulk) {
    requireKey(requiredFrontend, key, at, "bulkengine LabelKey");
  }
  for (const [key, at] of labels.search) {
    requireKey(requiredFrontend, key, at, "searchengine LabelKey");
  }
  for (const [key, at] of labels.meta) {
    const fe = REQUIRED_LOCALES.some((loc) => frontendLookup(catalogs, loc, key).ok);
    const be = Boolean(backend.en[key]);
    if (!fe && !be) {
      add(
        "error",
        "meta-label-missing",
        `LabelKey "${key}" missing from frontend locales and backend catalog`,
        { at },
      );
    }
  }

  for (const f of unionStrings(ioTypesSrc, "ExportFormat")) {
    requireKey(requiredFrontend, `exports.formats.${f}`, rel(IO_TYPES), "ExportFormat");
    requireKey(requiredBackend, `export.format.${f}`, rel(IO_TYPES), "ExportFormat backend");
  }
  for (const f of unionStrings(ioTypesSrc, "ImportFormat")) {
    requireKey(requiredFrontend, `imports.formats.${f}`, rel(IO_TYPES), "ImportFormat");
  }

  const searchIds = new Set();
  for (const [key] of labels.search) {
    const m = key.match(/^search\.specs_(.+)$/);
    if (m) searchIds.add(m[1]);
  }
  for (const file of tsFiles) {
    if (file.endsWith(".d.ts")) continue;
    const src = readFile(file);
    for (const m of src.matchAll(/labelKey:\s*"search\.specs_([^"]+)"/g)) {
      searchIds.add(m[1]);
    }
  }
  for (const id of searchIds) {
    requireKey(requiredFrontend, `search.specs_${id}`, "search spec", "search spec label");
    requireKey(requiredFrontend, `search.prefix_${id}`, "search spec", "search prefix alias");
  }

  const handledPrefix = (prefix) =>
    prefix === "activity.actions." ||
    prefix === "activity.resources." ||
    prefix === "activity." ||
    prefix === "permissions.labels." ||
    prefix === "permissions.groups." ||
    prefix.startsWith("stepup.gate.purpose.") ||
    prefix === "logs.levels." ||
    prefix === "users.status." ||
    prefix === "exports.formats." ||
    prefix === "exports." ||
    prefix === "imports.formats." ||
    prefix === "imports.resources." ||
    prefix === "imports.step." ||
    prefix === "imports." ||
    prefix === "notifications.status." ||
    prefix === "settings.paper_sizes." ||
    prefix.startsWith("storage.") ||
    prefix === "search.specs_" ||
    prefix === "search.prefix_";

  for (const dyn of feKeys.dynamic) {
    if (!handledPrefix(dyn.prefix) && dyn.prefix.length >= 3) {
      add("warning", "dynamic-key", `Unexpanded t(\`${dyn.prefix}\${…}\`) — verify keys exist`, {
        at: dyn.at,
      });
    }
  }

  for (const [key, rec] of requiredFrontend) {
    for (const loc of REQUIRED_LOCALES) {
      if (!frontendLookup(catalogs, loc, key).ok) {
        add("error", "missing-frontend", `${loc} missing ${key} (${rec.why})`, {
          at: rec.at.slice(0, 6),
        });
      }
    }
  }

  for (const [key, rec] of requiredBackend) {
    for (const loc of REQUIRED_LOCALES) {
      const val = backend[loc][key];
      if (typeof val !== "string" || !val.trim()) {
        add("error", "missing-backend", `${loc} catalog missing ${key} (${rec.why})`, {
          at: rec.at.slice(0, 6),
        });
      }
    }
  }

  if (WANT_UNUSED) {
    const usedSet = new Set(requiredFrontend.keys());
    for (const ns of wired.namespaces.size ? wired.namespaces : enNs) {
      const keys = catalogs.en[ns]?.keys ?? {};
      for (const k of Object.keys(keys)) {
        const full = `${ns}.${k}`;
        if (usedSet.has(full)) continue;
        if (/_(?:one|other|zero)$/.test(k)) {
          const base = full.replace(/_(?:one|other|zero)$/, "");
          if (usedSet.has(base)) continue;
        }
        add("warning", "unused-frontend", `Unused locale key ${full}`, {
          at: catalogs.en[ns] && rel(catalogs.en[ns].file),
        });
      }
    }
  }

  findings.sort(
    (a, b) =>
      (a.level === "error" ? 0 : 1) - (b.level === "error" ? 0 : 1) ||
      a.code.localeCompare(b.code) ||
      a.message.localeCompare(b.message),
  );

  if (WANT_JSON) {
    const errors = findings.filter((f) => f.level === "error").length;
    const warnings = findings.filter((f) => f.level === "warning").length;
    console.log(
      JSON.stringify(
        {
          ok: errors === 0 && (!STRICT || warnings === 0),
          errors,
          warnings,
          findings,
        },
        null,
        2,
      ),
    );
  } else {
    printHuman();
  }

  const failed =
    findings.some((f) => f.level === "error") ||
    (STRICT && findings.some((f) => f.level === "warning"));
  process.exit(failed ? 1 : 0);
}

main();
