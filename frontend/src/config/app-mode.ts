/**
 * Legacy app mode env — no longer affects routing or tenant visibility.
 * Kept for backward-compatible `/api/app-config` responses.
 */
export type AppMode = "platform";

export function getAppMode(): AppMode {
  return "platform";
}
