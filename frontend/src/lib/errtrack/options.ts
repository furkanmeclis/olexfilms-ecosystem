import type { BrowserOptions, ErrorEvent, NodeOptions } from "@sentry/nextjs";

import { moduleFromPath, parseModule } from "./modules";
import { scrubBreadcrumb, scrubEvent } from "./scrub";

/** Same-origin tunnel (app/api/monitoring/route.ts): no CSP / ad-block issues. */
export const TUNNEL_PATH = "/api/monitoring";

type Runtime = "nodejs" | "edge" | "browser";

type BaseInput = {
  dsn: string | undefined;
  environment: string | undefined;
  release: string | undefined;
  runtime: Runtime;
};

function pathOf(url: string | undefined): string | undefined {
  if (!url) return undefined;
  try {
    return new URL(url, "http://localhost").pathname;
  } catch {
    return undefined;
  }
}

/** Ensures a fixed `module` tag (K17) and the runtime tag on every event. */
export function tagEvent(event: ErrorEvent, runtime: Runtime): ErrorEvent {
  const tags = { ...(event.tags ?? {}) };
  const explicit = tags.module;
  if (typeof explicit === "string" && explicit) {
    tags.module = parseModule(explicit);
  } else {
    const path =
      pathOf(event.request?.url) ??
      (typeof window !== "undefined" ? window.location.pathname : undefined) ??
      event.transaction;
    tags.module = moduleFromPath(path);
  }
  tags.runtime = runtime;
  event.tags = tags;
  return event;
}

export function beforeSendFor(runtime: Runtime) {
  return (event: ErrorEvent): ErrorEvent => {
    // Tag first (needs the path), then scrub (drops query, PII).
    return scrubEvent(tagEvent(event, runtime));
  };
}

/** Common SDK options; `enabled` is false without a DSN (SDK is a no-op). */
export function baseOptions({ dsn, environment, release, runtime }: BaseInput) {
  const trimmed = dsn?.trim() || undefined;
  return {
    dsn: trimmed,
    enabled: Boolean(trimmed),
    environment: environment?.trim() || process.env.NODE_ENV,
    release: release?.trim() || undefined,
    sendDefaultPii: false,
    // The receiver drops transactions and sessions.
    tracesSampleRate: 0,
    beforeSend: beforeSendFor(runtime),
    beforeBreadcrumb: scrubBreadcrumb,
  } satisfies NodeOptions & BrowserOptions;
}

export function serverOptions(runtime: "nodejs" | "edge") {
  return baseOptions({
    dsn: process.env.SENTRY_DSN || process.env.NEXT_PUBLIC_SENTRY_DSN,
    environment:
      process.env.SENTRY_ENVIRONMENT ||
      process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT,
    release:
      process.env.SENTRY_RELEASE || process.env.NEXT_PUBLIC_SENTRY_RELEASE,
    runtime,
  });
}

export function browserOptions(): BrowserOptions {
  return {
    ...baseOptions({
      dsn: process.env.NEXT_PUBLIC_SENTRY_DSN,
      environment: process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT,
      release: process.env.NEXT_PUBLIC_SENTRY_RELEASE,
      runtime: "browser",
    }),
    tunnel: TUNNEL_PATH,
    // No session replay / tracing integrations: errors only.
    sendClientReports: false,
  };
}
