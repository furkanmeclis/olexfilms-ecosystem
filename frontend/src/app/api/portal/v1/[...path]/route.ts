import { bffRouteHandler } from "@/lib/server/bff-route";

/**
 * Customer / fleet portal BFF (TEC-90): reads and writes the portal session
 * cookie only and forwards an allowlist of upstream paths (see
 * `isRealmPathAllowed`); everything else answers 404.
 */
const handle = bffRouteHandler("portal");

export const GET = handle;
export const POST = handle;
export const PUT = handle;
export const PATCH = handle;
export const DELETE = handle;
export const HEAD = handle;
export const OPTIONS = handle;
