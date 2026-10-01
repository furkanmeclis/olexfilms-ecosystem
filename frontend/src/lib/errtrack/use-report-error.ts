"use client";

import * as Sentry from "@sentry/nextjs";
import { useEffect } from "react";

import type { Module } from "./modules";

/**
 * Reports an error boundary's error once (no-op without a DSN). `module`
 * pins the K17 module tag; otherwise it is derived from the URL.
 */
export function useReportError(
  error: Error & { digest?: string },
  module?: Module,
) {
  useEffect(() => {
    Sentry.captureException(error, {
      tags: {
        ...(module ? { module } : {}),
        ...(error.digest ? { digest: error.digest } : {}),
      },
    });
  }, [error, module]);
}
