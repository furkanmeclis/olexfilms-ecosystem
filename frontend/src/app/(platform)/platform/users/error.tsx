"use client";

import { ErrorState } from "@/components/common/error-state";
import { useLocale } from "@/providers/locale-provider";

export default function UsersError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const { t } = useLocale();

  return (
    <ErrorState
      title={t("common.error_generic")}
      description={error.message || t("users.error_boundary")}
      onRetry={reset}
      retryLabel={t("common.retry")}
    />
  );
}
