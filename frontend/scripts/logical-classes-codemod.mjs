#!/usr/bin/env node
/**
 * Codemod: physical Tailwind utilities -> logical ones (TEC-137).
 *
 *   node scripts/logical-classes-codemod.mjs           # rewrite src/
 *   node scripts/logical-classes-codemod.mjs --check   # list, exit 1 if any
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  findPhysicalClasses,
  isPhysicalClassIgnored,
  rewritePhysicalClasses,
} from "./physical-classes.mjs";

const FRONTEND = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const CHECK = process.argv.includes("--check");

function walk(dir) {
  const out = [];
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, ent.name);
    const rel = path.relative(FRONTEND, p);
    if (isPhysicalClassIgnored(`${rel}/`)) continue;
    if (ent.isDirectory()) out.push(...walk(p));
    else if (/\.(tsx?|css)$/.test(ent.name) && !ent.name.endsWith(".d.ts")) {
      out.push(p);
    }
  }
  return out;
}

let total = 0;
for (const file of walk(path.join(FRONTEND, "src"))) {
  const src = fs.readFileSync(file, "utf8");
  const hits = findPhysicalClasses(src);
  if (!hits.length) continue;
  total += hits.length;
  const rel = path.relative(FRONTEND, file);
  for (const h of hits) {
    console.log(`${rel}:${h.line}  ${h.token} -> ${h.logical}`);
  }
  if (!CHECK) fs.writeFileSync(file, rewritePhysicalClasses(src));
}
console.log(
  `${total} physical utilit${total === 1 ? "y" : "ies"}${CHECK ? " found" : " rewritten"}.`,
);
process.exit(CHECK && total ? 1 : 0);
