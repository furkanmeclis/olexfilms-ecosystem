/**
 * Browser SDK tunnel: the SDK posts envelopes to /api/monitoring (same
 * origin) and the server forwards them to the configured DSN's envelope
 * endpoint. Only the configured DSN is accepted, so this is not an open proxy.
 */

export const MAX_ENVELOPE_BYTES = 1_000_000;

type ParsedDsn = {
  origin: string;
  pathPrefix: string;
  projectId: string;
  publicKey: string;
};

export function parseDsn(dsn: string | undefined): ParsedDsn | null {
  if (!dsn) return null;
  try {
    const url = new URL(dsn.trim());
    const parts = url.pathname.split("/").filter(Boolean);
    const projectId = parts.pop();
    if (!projectId || !url.username) return null;
    return {
      origin: url.origin,
      pathPrefix: parts.length ? `/${parts.join("/")}` : "",
      projectId,
      publicKey: url.username,
    };
  } catch {
    return null;
  }
}

/** Envelope endpoint URL of a DSN: {origin}{prefix}/api/{id}/envelope/. */
export function envelopeUrl(dsn: ParsedDsn): string {
  return `${dsn.origin}${dsn.pathPrefix}/api/${dsn.projectId}/envelope/?sentry_key=${encodeURIComponent(dsn.publicKey)}`;
}

/**
 * Returns the upstream URL for an envelope when its header DSN matches the
 * allowed DSN (same origin, project and key); null otherwise.
 */
export function resolveTunnelTarget(
  envelope: string,
  allowedDsn: string | undefined,
): string | null {
  const allowed = parseDsn(allowedDsn);
  if (!allowed) return null;
  const newline = envelope.indexOf("\n");
  const headerLine = newline >= 0 ? envelope.slice(0, newline) : envelope;
  let header: { dsn?: unknown };
  try {
    header = JSON.parse(headerLine) as { dsn?: unknown };
  } catch {
    return null;
  }
  const got = parseDsn(typeof header.dsn === "string" ? header.dsn : undefined);
  if (
    !got ||
    got.origin !== allowed.origin ||
    got.projectId !== allowed.projectId ||
    got.publicKey !== allowed.publicKey ||
    got.pathPrefix !== allowed.pathPrefix
  ) {
    return null;
  }
  return envelopeUrl(allowed);
}
