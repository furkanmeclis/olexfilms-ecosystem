import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

/**
 * TEC-251 / design §9: Umami analytics and the trade-fair ("fuar") screen of
 * the old hub are removed for good. This guard scans the source tree so
 * neither creeps back in. docs/ is skipped (the design doc names them as
 * removed) and so is this file.
 */
const repoRoot = path.resolve(__dirname, "../../../..");
const frontendRoot = path.join(repoRoot, "frontend");
const self = path.resolve(__filename);

const SCAN = [
  "frontend/src",
  "frontend/public",
  "frontend/scripts",
  "frontend/e2e",
  "backend/cmd",
  "backend/internal",
  "backend/pkg",
  "backend/migrations",
  "backend/deploy",
  "backend/docs",
  "deploy",
  "scripts",
  ".github",
];
const ROOT_FILES = [
  ".env.example",
  ".env.server.example",
  "compose.local.yml",
  "compose.prod.yml",
  "Makefile",
  "frontend/next.config.ts",
  "frontend/package.json",
  "backend/go.mod",
];
const SKIP_DIRS = new Set(["node_modules", ".next", ".git", "test-results"]);
const TEXT_EXT =
  /\.(ts|tsx|js|mjs|cjs|jsx|json|go|sql|ya?ml|sh|css|html|svg|md|txt|toml|env|example)$|(^|\/)(Makefile|Dockerfile)$/;
const FORBIDDEN = /umami|fuar/i;

function walk(dir: string, out: string[]) {
  for (const name of readdirSync(dir)) {
    if (SKIP_DIRS.has(name)) continue;
    const full = path.join(dir, name);
    const st = statSync(full);
    if (st.isDirectory()) walk(full, out);
    else if (TEXT_EXT.test(full) && full !== self) out.push(full);
  }
}

function sourceFiles(): string[] {
  const files: string[] = [];
  for (const rel of SCAN) {
    const dir = path.join(repoRoot, rel);
    if (existsSync(dir)) walk(dir, files);
  }
  for (const rel of ROOT_FILES) {
    const file = path.join(repoRoot, rel);
    if (existsSync(file)) files.push(file);
  }
  return files;
}

describe("Umami / fuar remnants (TEC-251, design §9)", () => {
  it("scans the frontend source", () => {
    const files = sourceFiles();
    expect(
      files.some((f) => f.startsWith(path.join(frontendRoot, "src"))),
    ).toBe(true);
    expect(files.length).toBeGreaterThan(100);
  });

  it("finds no umami or fuar in source", () => {
    const hits: string[] = [];
    for (const file of sourceFiles()) {
      const lines = readFileSync(file, "utf8").split("\n");
      lines.forEach((line, i) => {
        if (FORBIDDEN.test(line)) {
          hits.push(`${path.relative(repoRoot, file)}:${i + 1}`);
        }
      });
    }
    expect(hits).toEqual([]);
  });
});
