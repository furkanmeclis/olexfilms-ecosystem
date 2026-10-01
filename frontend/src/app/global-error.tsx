"use client";

import en from "@/locales/en/common.json";
import tr from "@/locales/tr/common.json";
import { useReportError } from "@/lib/errtrack/use-report-error";

/**
 * Root error boundary: replaces the root layout (no providers), so texts come
 * straight from the common locale files.
 */
export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useReportError(error);
  const lang =
    typeof navigator !== "undefined" && navigator.language?.startsWith("tr")
      ? "tr"
      : "en";
  const messages = lang === "tr" ? tr : en;

  return (
    <html lang={lang}>
      <body
        style={{
          display: "flex",
          minHeight: "100vh",
          alignItems: "center",
          justifyContent: "center",
          fontFamily: "system-ui, sans-serif",
        }}
      >
        <main role="alert" style={{ textAlign: "center" }}>
          <h1 style={{ fontSize: "1.25rem", fontWeight: 600 }}>
            {messages.error_generic}
          </h1>
          <button
            type="button"
            onClick={reset}
            style={{ marginTop: "1rem", padding: "0.5rem 1rem" }}
          >
            {messages.retry}
          </button>
        </main>
      </body>
    </html>
  );
}
