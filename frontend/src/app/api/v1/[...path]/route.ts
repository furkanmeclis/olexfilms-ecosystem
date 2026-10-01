import { bffRouteHandler } from "@/lib/server/bff-route";

/**
 * Long-lived SSE streams (AI chat and confirmed-action continuations) need a
 * raised function timeout on serverless hosts. The Go AI stream handlers
 * extend their own write deadline to 15 minutes (heartbeat every 15s), so
 * allow the same here; self-hosted `next start` ignores this value.
 */
export const maxDuration = 900;

/** Panel (staff) BFF: reads and writes the panel session cookie only. */
const handle = bffRouteHandler("panel");

export const GET = handle;
export const POST = handle;
export const PUT = handle;
export const PATCH = handle;
export const DELETE = handle;
export const HEAD = handle;
export const OPTIONS = handle;
