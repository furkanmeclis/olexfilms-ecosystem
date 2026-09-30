/**
 * Decode JWT payload without verification (client already received the token
 * from our authenticated BFF). Used to read Centrifugo `sub` (user id).
 */
export function decodeJwtPayload(
  token: string,
): Record<string, unknown> | null {
  try {
    const parts = token.split(".");
    if (parts.length < 2) return null;
    const payload = parts[1];
    const normalized = payload.replace(/-/g, "+").replace(/_/g, "/");
    const padded = normalized.padEnd(
      normalized.length + ((4 - (normalized.length % 4)) % 4),
      "=",
    );
    const json =
      typeof atob === "function"
        ? atob(padded)
        : Buffer.from(padded, "base64").toString("utf8");
    return JSON.parse(json) as Record<string, unknown>;
  } catch {
    return null;
  }
}

/**
 * Centrifugo connection JWT `sub`.
 * App: UUID string; legacy deployments may still use numeric ids.
 */
export function readUserIdFromConnectionToken(token: string): string | null {
  const payload = decodeJwtPayload(token);
  if (!payload) return null;
  const sub = payload.sub;
  if (typeof sub === "string" && sub.length > 0) return sub;
  if (typeof sub === "number" && Number.isFinite(sub) && sub > 0) {
    return String(Math.trunc(sub));
  }
  return null;
}

/** @deprecated Prefer `readUserIdFromConnectionToken`. */
export function readNumericUserIdFromConnectionToken(
  token: string,
): string | null {
  return readUserIdFromConnectionToken(token);
}

export function parseRealtimeMessage(data: unknown): {
  type: string;
  data: Record<string, unknown>;
} | null {
  if (!data || typeof data !== "object") return null;
  const record = data as Record<string, unknown>;
  const event =
    typeof record.event === "string" && record.event.length > 0
      ? record.event
      : null;
  const typed =
    typeof record.type === "string" && record.type.length > 0
      ? record.type
      : null;
  // Prefer `event`: inbox publishes domain name in `event` and message content
  // type (text/image/…) in `type`. Notifications / older payloads use only `type`.
  const type = event ?? typed;
  if (!type) return null;
  if (record.data && typeof record.data === "object") {
    return { type, data: record.data as Record<string, unknown> };
  }
  const rest: Record<string, unknown> = { ...record };
  delete rest.event;
  // Keep `type` in data when it is the content type alongside `event`.
  if (!event) {
    delete rest.type;
  }
  return { type, data: rest };
}
