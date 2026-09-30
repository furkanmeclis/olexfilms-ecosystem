"use client";

import { ErrorState } from "@/components/common/error-state";
import { useLocale } from "@/providers/locale-provider";

export default function OrganizationsError({
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
      description={error.message || t("organizations.error_boundary")}
      onRetry={reset}
      retryLabel={t("common.retry")}
    />
  );
}
