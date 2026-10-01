import * as Sentry from "@sentry/nextjs";

import { serverOptions } from "@/lib/errtrack/options";

/**
 * Server + edge error tracking (TEC-82, K17). Without SENTRY_DSN (or
 * NEXT_PUBLIC_SENTRY_DSN) the SDK is initialised disabled: a no-op.
 */
export async function register() {
  if (process.env.NEXT_RUNTIME === "nodejs") {
    Sentry.init(serverOptions("nodejs"));
  }
  if (process.env.NEXT_RUNTIME === "edge") {
    Sentry.init(serverOptions("edge"));
  }
}

/** Errors from server components, route handlers, server actions, proxy. */
export const onRequestError = Sentry.captureRequestError;
