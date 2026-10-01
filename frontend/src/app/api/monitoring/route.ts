import { MAX_ENVELOPE_BYTES, resolveTunnelTarget } from "@/lib/errtrack/tunnel";
import { isCrossSiteMutation } from "@/lib/server/bff-proxy";

/**
 * Sentry browser SDK tunnel (TEC-82). Separate from the /api/v1 BFF: it
 * forwards envelopes only to the configured NEXT_PUBLIC_SENTRY_DSN and only
 * for same-origin browsers.
 */
export async function POST(request: Request) {
  const dsn = process.env.NEXT_PUBLIC_SENTRY_DSN;
  if (!dsn) return new Response(null, { status: 404 });
  if (isCrossSiteMutation(request)) return new Response(null, { status: 403 });

  const length = Number(request.headers.get("content-length") ?? "0");
  if (length > MAX_ENVELOPE_BYTES) return new Response(null, { status: 413 });
  const body = await request.text();
  if (body.length > MAX_ENVELOPE_BYTES) {
    return new Response(null, { status: 413 });
  }

  const target = resolveTunnelTarget(body, dsn);
  if (!target) return new Response(null, { status: 400 });

  try {
    const upstream = await fetch(target, {
      method: "POST",
      headers: { "Content-Type": "application/x-sentry-envelope" },
      body,
      signal: AbortSignal.timeout(5_000),
    });
    return new Response(null, { status: upstream.status });
  } catch {
    return new Response(null, { status: 502 });
  }
}
