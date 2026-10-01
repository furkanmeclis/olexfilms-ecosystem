/**
 * Physical (left/right) Tailwind utilities and their logical (start/end)
 * replacements (TEC-137, K10: RTL needs logical properties).
 *
 * Shared by the codemod (scripts/logical-classes-codemod.mjs), the ESLint
 * rule (eslint.config.mjs) and the repo-level scripts/check-i18n.mjs.
 *
 * Centering utilities (left-1/2, left-[50%] ...) are direction neutral when
 * paired with translate-x and stay physical on purpose.
 */

// Tailwind spacing / inset values: 4, 0.5, 1/2, px, full, auto, [..], (--var).
const VALUE = String.raw`(?:\d+(?:\.\d+)?|\d+\/\d+|px|full|auto|\[[^\]\s"'\x60]+\]|\([^)\s"'\x60]+\))`;
const VARIANTS = String.raw`(?:[\w\-\[\]&:*>.()@/=%]+:)?`;

const PHYSICAL = String.raw`(?:(?:ml|mr|pl|pr|scroll-m[lr]|scroll-p[lr]|left|right)-${VALUE}|border-[lr](?:-[\w\[\]\/().-]+)?|rounded-(?:tl|tr|bl|br|l|r)(?:-[\w\[\]()-]+)?|(?:text|float|clear)-(?:left|right))`;

/** Matches one physical utility token (with variants, ! and negative sign). */
export const PHYSICAL_CLASS_SOURCE = String.raw`(?<![\w\-/.\[])(${VARIANTS})(!?)(-?)(${PHYSICAL})(?![\w\-\[/])`;

export function physicalClassRegex() {
  return new RegExp(PHYSICAL_CLASS_SOURCE, "g");
}

/** Direction neutral centering utilities that stay physical. */
const ALLOWED = new Set(["left-1/2", "right-1/2", "left-[50%]", "right-[50%]"]);

const PREFIX_MAP = [
  ["scroll-ml-", "scroll-ms-"],
  ["scroll-mr-", "scroll-me-"],
  ["scroll-pl-", "scroll-ps-"],
  ["scroll-pr-", "scroll-pe-"],
  ["rounded-tl", "rounded-ss"],
  ["rounded-tr", "rounded-se"],
  ["rounded-bl", "rounded-es"],
  ["rounded-br", "rounded-ee"],
  ["rounded-l", "rounded-s"],
  ["rounded-r", "rounded-e"],
  ["border-l", "border-s"],
  ["border-r", "border-e"],
  ["text-left", "text-start"],
  ["text-right", "text-end"],
  ["float-left", "float-start"],
  ["float-right", "float-end"],
  ["clear-left", "clear-start"],
  ["clear-right", "clear-end"],
  ["left-", "start-"],
  ["right-", "end-"],
  ["ml-", "ms-"],
  ["mr-", "me-"],
  ["pl-", "ps-"],
  ["pr-", "pe-"],
];

/** Logical equivalent of a physical utility (without variants), or null. */
export function toLogical(utility) {
  if (ALLOWED.has(utility)) return null;
  for (const [from, to] of PREFIX_MAP) {
    if (utility.startsWith(from)) return to + utility.slice(from.length);
  }
  return null;
}

/**
 * ESLint rule name. A line that must stay physical (JS-computed `left`
 * positions, HTML `align=right`, drag handles) carries
 * `// eslint-disable-next-line local/no-physical-classes` (or -line); the
 * codemod and check-i18n honour the same comment.
 */
export const PHYSICAL_CLASS_RULE = "local/no-physical-classes";

function suppressedLines(text) {
  const lines = text.split("\n");
  const out = new Set();
  lines.forEach((line, i) => {
    if (!line.includes(PHYSICAL_CLASS_RULE)) return;
    if (line.includes("eslint-disable-next-line")) out.add(i + 1);
    else if (line.includes("eslint-disable-line")) out.add(i);
  });
  return out;
}

function lineAt(text, index) {
  let n = 0;
  for (let i = text.indexOf("\n"); i !== -1 && i < index;) {
    n++;
    i = text.indexOf("\n", i + 1);
  }
  return n;
}

/** Lists physical utilities in a source text: [{ index, line, token, logical }]. */
export function findPhysicalClasses(text) {
  const skip = suppressedLines(text);
  const out = [];
  for (const m of text.matchAll(physicalClassRegex())) {
    const logical = toLogical(m[4]);
    if (!logical) continue;
    const line = lineAt(text, m.index);
    if (skip.has(line)) continue;
    out.push({
      index: m.index,
      line: line + 1,
      token: m[0],
      logical: `${m[1]}${m[2]}${m[3]}${logical}`,
    });
  }
  return out;
}

/** Rewrites every physical utility in text to its logical equivalent. */
export function rewritePhysicalClasses(text) {
  const skip = suppressedLines(text);
  return text.replace(
    physicalClassRegex(),
    (whole, variants, bang, neg, utility, offset) => {
      const logical = toLogical(utility);
      if (!logical || skip.has(lineAt(text, offset))) return whole;
      return `${variants}${bang}${neg}${logical}`;
    },
  );
}

/** Paths (relative to frontend/) the rule does not apply to. */
export const PHYSICAL_CLASS_IGNORES = [
  "src/generated/",
  // Vendored component kit, kept as upstream ships it.
  "src/components/shadix-ui/",
];

export function isPhysicalClassIgnored(relPath) {
  const p = relPath.replaceAll("\\", "/");
  return PHYSICAL_CLASS_IGNORES.some((ig) => p.startsWith(ig));
}
