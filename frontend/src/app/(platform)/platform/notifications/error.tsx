"use client";

import { ErrorState } from "@/components/common/error-state";
import { useReportError } from "@/lib/errtrack/use-report-error";
import { useLocale } from "@/providers/locale-provider";

export default function NotificationsError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const { t } = useLocale();
  useReportError(error, "notification");

  return (
    <ErrorState
      title={t("common.error_generic")}
      description={error.message || t("notifications.error_boundary")}
      onRetry={reset}
      retryLabel={t("common.retry")}
    />
  );
}
