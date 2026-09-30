export const authConfig = {
  /** Access cookie TTL fallback when upstream omits expires_in (seconds) */
  accessMaxAgeSec: 15 * 60,
  /** Refresh cookie TTL (seconds) — aligns with backend default 7d */
  refreshMaxAgeSec: 7 * 24 * 60 * 60,
  refreshSkewMs: 30_000,
} as const;
