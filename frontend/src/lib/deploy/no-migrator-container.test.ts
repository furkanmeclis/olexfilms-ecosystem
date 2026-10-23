import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { describe, expect, it } from "vitest";

/**
 * TEC-282 / K27: the migrator container is removed after the final cutover.
 * Neither compose file declares it, the env template carries no LEGACY_* /
 * MIGRATOR_* keys, gen-env-server.sh does not emit them, and prod runs the
 * 12 always-on containers of design §6 (profile services aside).
 */
const repoRoot = path.resolve(__dirname, "../../../..");
const read = (file: string) => readFileSync(path.join(repoRoot, file), "utf8");

// Top-level service names with whether each sits behind a compose profile.
function services(compose: string): { name: string; profiled: boolean }[] {
  const body = compose.split(/^services:\s*$/m)[1]?.split(/^\S/m)[0] ?? "";
  return body
    .split(/^(?= {2}[a-z][a-z0-9_-]*:\s*$)/m)
    .map((block) => {
      const name = /^ {2}([a-z][a-z0-9_-]*):/.exec(block)?.[1];
      return name ? { name, profiled: /^ {4}profiles:/m.test(block) } : null;
    })
    .filter((s): s is { name: string; profiled: boolean } => s !== null);
}

const LEGACY_KEY = /^(LEGACY_|MIGRATOR_)/m;

describe("migrator removal (TEC-282)", () => {
  it("no compose file declares a migrator service", () => {
    for (const file of ["compose.prod.yml", "compose.local.yml"]) {
      const names = services(read(file)).map((s) => s.name);
      expect(names.length, file).toBeGreaterThan(0);
      expect(names, file).not.toContain("migrator");
    }
  });

  it("prod runs the 12 always-on containers of design §6", () => {
    const always = services(read("compose.prod.yml")).filter(
      (s) => !s.profiled,
    );
    expect(always.map((s) => s.name)).toEqual([
      "postgres",
      "redis",
      "seaweedfs",
      "meilisearch",
      "centrifugo",
      "gotenberg",
      "wuzapi",
      "migrate",
      "backend",
      "worker-core",
      "worker-docs",
      "frontend",
    ]);
  });

  it("the env template has no legacy migrator keys", () => {
    expect(read(".env.example")).not.toMatch(LEGACY_KEY);
  });

  it("gen-env-server.sh does not emit legacy migrator keys", () => {
    const dir = mkdtempSync(path.join(tmpdir(), "tec282-"));
    try {
      const out = path.join(dir, "server.env");
      execFileSync(
        "bash",
        [
          path.join(repoRoot, "scripts/gen-env-server.sh"),
          "--app",
          "ci.example",
          "--realtime",
          "wss.ci.example",
          "--out",
          out,
        ],
        { stdio: "ignore" },
      );
      const env = readFileSync(out, "utf8");
      expect(env).toMatch(/^GOTENBERG_URL=/m);
      expect(env).not.toMatch(LEGACY_KEY);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
