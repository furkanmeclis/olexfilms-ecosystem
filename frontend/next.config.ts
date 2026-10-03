import { withSentryConfig } from "@sentry/nextjs/config";
import type { NextConfig } from "next";

/**
 * Baseline security headers for every route. A full script CSP is not set
 * here (Next inline bootstrap scripts would need nonces); `frame-ancestors`
 * and `base-uri` restrictions are safe to enforce globally. There is no
 * `connect-src` either: the browser error-tracking SDK posts to the
 * same-origin /api/monitoring tunnel, so a future `connect-src 'self'`
 * already covers it (never the DSN host).
 */
const securityHeaders = [
  {
    key: "Content-Security-Policy",
    value: "frame-ancestors 'self'; base-uri 'self'",
  },
  { key: "X-Frame-Options", value: "SAMEORIGIN" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Permissions-Policy",
    value: "camera=(self), microphone=(self), geolocation=(), payment=()",
  },
  ...(process.env.NODE_ENV === "production"
    ? [
        {
          key: "Strict-Transport-Security",
          value: "max-age=31536000; includeSubDomains",
        },
      ]
    : []),
];

const nextConfig: NextConfig = {
  output: "standalone",
  async headers() {
    return [{ source: "/:path*", headers: securityHeaders }];
  },
  /**
   * TEC-249: the old hub served short links at `/_/{token}`; migrated tokens
   * keep their value (TEC-263), so links already sent by SMS / WhatsApp go
   * to the new resolver route `/s/{token}`.
   */
  async redirects() {
    return [
      { source: "/_/:token", destination: "/s/:token", permanent: false },
    ];
  },
};

/**
 * Error tracking (TEC-82): the technowide receiver has no source map
 * support, so nothing is uploaded and the Sentry CLI never runs; no build
 * telemetry. The SDK stays a no-op without a DSN.
 */
export default withSentryConfig(nextConfig, {
  silent: true,
  telemetry: false,
  sourcemaps: { disable: true },
  release: { create: false, finalize: false },
  widenClientFileUpload: false,
});
