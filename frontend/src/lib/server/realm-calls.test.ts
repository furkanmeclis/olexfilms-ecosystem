import fs from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

/**
 * TEC-90 lock: every session cookie helper and BFF entry point is called
 * with an explicit realm. A realm-less call (impersonation, org switch,
 * refresh paths, ...) would read or write the wrong session cookie. The
 * TypeScript signatures require the argument; this test also catches `any`
 * casts and string-built calls.
 */
const SRC = path.resolve(__dirname, "../..");

function sourceFiles(dir: string, out: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === "generated" || entry.name === "node_modules") continue;
      sourceFiles(full, out);
    } else if (
      /\.(ts|tsx)$/.test(entry.name) &&
      !/\.test\.tsx?$/.test(entry.name)
    ) {
      out.push(full);
    }
  }
  return out;
}

const REALM = `"(panel|portal)"|realm\\b`;
const CHECKS: { name: string; call: RegExp; ok: RegExp }[] = [
  {
    name: "getApiTokens",
    call: /getApiTokens\(([^)]*)\)/g,
    ok: new RegExp(`^\\s*(${REALM})\\s*$`),
  },
  {
    name: "clearAuthSessionCookie",
    call: /clearAuthSessionCookie\(([^)]*)\)/g,
    ok: new RegExp(`^\\s*(${REALM})\\s*$`),
  },
  {
    name: "persistApiTokens",
    call: /persistApiTokens\(\s*([^,{)]*)/g,
    ok: new RegExp(`^\\s*(${REALM})\\s*$`),
  },
  {
    name: "proxyToUpstream",
    call: /proxyToUpstream\(\s*([^,)]*)/g,
    ok: new RegExp(`^\\s*(${REALM})\\s*$`),
  },
  {
    name: "sessionCookieName",
    call: /sessionCookieName\(([^)]*)\)/g,
    ok: new RegExp(`^\\s*(${REALM})\\s*$`),
  },
  {
    name: "authCookies",
    call: /authCookies\(([^)]*)\)/g,
    ok: new RegExp(`^\\s*(${REALM})\\s*$`),
  },
];

describe("realm-scoped auth helpers", () => {
  it("never calls a session helper without a realm", () => {
    const offenders: string[] = [];
    for (const file of sourceFiles(SRC)) {
      const text = fs.readFileSync(file, "utf8");
      for (const check of CHECKS) {
        for (const m of text.matchAll(check.call)) {
          const before = text.slice(Math.max(0, (m.index ?? 0) - 16), m.index);
          // Skip the definitions themselves.
          if (/function\s*$|async function\s*$/.test(before)) continue;
          if (!check.ok.test(m[1] ?? "")) {
            offenders.push(`${path.relative(SRC, file)}: ${m[0]}`);
          }
        }
      }
    }
    expect(offenders).toEqual([]);
  });

  it("portal UI never uses the panel next-auth/react client", () => {
    const portalDirs = [
      path.join(SRC, "features/portal"),
      path.join(SRC, "app/portal"),
    ];
    const offenders: string[] = [];
    for (const dir of portalDirs) {
      if (!fs.existsSync(dir)) continue;
      for (const file of sourceFiles(dir)) {
        const text = fs.readFileSync(file, "utf8");
        if (
          text.includes("next-auth/react") ||
          text.includes('"/api/v1/') ||
          text.includes("`/api/v1/")
        ) {
          offenders.push(path.relative(SRC, file));
        }
      }
    }
    expect(offenders).toEqual([]);
  });
});
