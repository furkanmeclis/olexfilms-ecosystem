import * as Sentry from "@sentry/nextjs";

import { browserOptions } from "@/lib/errtrack/options";

// Browser error tracking: only when NEXT_PUBLIC_SENTRY_DSN is baked in at
// build time. Envelopes go through the same-origin /api/monitoring tunnel.
if (process.env.NEXT_PUBLIC_SENTRY_DSN) {
  Sentry.init(browserOptions());
}

// Navigation hook the SDK asks for (breadcrumbs only: tracing is off).
export const onRouterTransitionStart = Sentry.captureRouterTransitionStart;
