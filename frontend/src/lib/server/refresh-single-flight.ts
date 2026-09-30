import { createHash } from "node:crypto";

/**
 * Refresh tokens are single-use (the API revokes on rotation). The panel fires
 * several requests at once (polling, focus refetch), and after the access token
 * expires each BFF handler would refresh with the same token: one wins, the
 * rest get 401 and the user was logged out. Every caller holding the same
 * refresh token now shares one upstream rotation, and a successful result is
 * reused for a short window so late requests — or a cookie briefly rewritten
 * with the old pair by a concurrent Auth.js response — get the same new tokens.
 */
export type RefreshResult = {
  status: number;
  body: ArrayBuffer;
};

type Entry = {
  promise: Promise<RefreshResult>;
  expiresAt: number;
};

/** How long a successful rotation is handed to callers still on the old token. */
export const REFRESH_REUSE_MS = 120_000;

const MAX_ENTRIES = 1_000;

const entries = new Map<string, Entry>();

function keyFor(refreshToken: string) {
  return createHash("sha256").update(refreshToken).digest("hex");
}

function prune(now: number) {
  for (const [key, entry] of entries) {
    if (entry.expiresAt <= now) entries.delete(key);
  }
  // Hard cap so a burst of distinct tokens cannot grow the map unbounded.
  while (entries.size > MAX_ENTRIES) {
    const oldest = entries.keys().next().value;
    if (oldest === undefined) break;
    entries.delete(oldest);
  }
}

/**
 * Never hand out a cached pair longer than its access token lives (minus a
 * margin), so a reused result is always usable.
 */
function reuseWindowMs(result: RefreshResult): number {
  try {
    const json = JSON.parse(new TextDecoder().decode(result.body)) as {
      data?: { expires_in?: unknown };
    };
    const expiresIn = json.data?.expires_in;
    if (typeof expiresIn === "number" && expiresIn > 0) {
      return Math.max(0, Math.min(REFRESH_REUSE_MS, (expiresIn - 5) * 1000));
    }
  } catch {
    // Unparseable body: fall back to the default window.
  }
  return REFRESH_REUSE_MS;
}

export function sharedRefresh(
  refreshToken: string,
  rotate: (refreshToken: string) => Promise<RefreshResult>,
  now: () => number = Date.now,
): Promise<RefreshResult> {
  prune(now());
  const key = keyFor(refreshToken);
  const existing = entries.get(key);
  if (existing) return existing.promise;

  const entry: Entry = {
    // In flight: keep it until it settles.
    expiresAt: Number.POSITIVE_INFINITY,
    promise: rotate(refreshToken).then(
      (result) => {
        if (result.status < 400) {
          entry.expiresAt = now() + reuseWindowMs(result);
        } else {
          // Failures are not cached: a later retry must reach the API.
          entries.delete(key);
        }
        return result;
      },
      (error: unknown) => {
        entries.delete(key);
        throw error;
      },
    ),
  };
  entries.set(key, entry);
  return entry.promise;
}

/** Test hook. */
export function resetSharedRefresh() {
  entries.clear();
}
