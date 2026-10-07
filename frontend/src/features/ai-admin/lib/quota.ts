import type { AIOrgType } from "@/features/ai-admin/services/ai-admin.service";

/** Organization types of the quota table (`org_type` filter). */
export const AI_ORG_TYPES: readonly AIOrgType[] = [
  "center",
  "distributor",
  "dealer",
];

/** Usage ledger enums (backend model.Usage*). */
export const AI_USAGE_CHANNELS = [
  "panel",
  "portal",
  "whatsapp",
  "mcp",
  "triage",
] as const;
export const AI_USAGE_PURPOSES = ["chat", "title", "triage", "locale"] as const;
export const AI_USAGE_POOLS = ["org", "system"] as const;

/** Knowledge text limit of the backend (bytes) and extra instructions (chars). */
export const AI_KNOWLEDGE_MAX_BYTES = 20 * 1024;
export const AI_INSTRUCTIONS_MAX_CHARS = 20_000;

/** A monthly token quota of 0 means unlimited (TEC-389). */
export function isUnlimitedQuota(quota: number | null | undefined): boolean {
  return quota === 0;
}

/**
 * Typed quota text → whole non-negative token count. Signs, decimals and
 * any other character are dropped, so a negative value cannot be entered;
 * an empty field is null.
 */
export function parseQuotaInput(raw: string): number | null {
  const digits = raw.replace(/[^0-9]/g, "");
  if (digits === "") return null;
  const n = Number(digits);
  return Number.isSafeInteger(n) ? n : Number.MAX_SAFE_INTEGER;
}

/** Quota usage bar tone: 80% warns, 100% is exhausted. */
export function quotaTone(
  percent: number | null | undefined,
): "default" | "warning" | "danger" {
  if (percent == null) return "default";
  if (percent >= 100) return "danger";
  if (percent >= 80) return "warning";
  return "default";
}

/** Current UTC month as YYYY-MM (the backend period key). */
export function currentPeriod(now: Date = new Date()): string {
  return now.toISOString().slice(0, 7);
}

/** The last `count` periods, newest first. */
export function recentPeriods(count = 12, now: Date = new Date()): string[] {
  const out: string[] = [];
  const year = now.getUTCFullYear();
  const month = now.getUTCMonth();
  for (let i = 0; i < count; i++) {
    const d = new Date(Date.UTC(year, month - i, 1));
    out.push(d.toISOString().slice(0, 7));
  }
  return out;
}

/** UTF-8 byte length (the knowledge text limit is in bytes). */
export function byteLength(text: string): number {
  return new TextEncoder().encode(text).length;
}

export function aiUserName(
  user: { name: string; surname: string } | null | undefined,
): string {
  if (!user) return "";
  return `${user.name} ${user.surname}`.trim();
}
