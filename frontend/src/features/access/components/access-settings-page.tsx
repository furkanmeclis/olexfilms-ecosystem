"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { permissions } from "@/config/permissions";
import { AccessSettingsForm } from "@/features/access/components/access-settings-form";
import type { AccessFormValues } from "@/features/access/schemas/access-form";
import { accessService } from "@/features/access/services/access.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const QUERY_KEY = ["platform", "access", "settings"] as const;

export function AccessSettingsPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const queryClient = useQueryClient();
  const canWrite = can(permissions.access.write);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: QUERY_KEY,
    queryFn: () => accessService.getPolicy(),
  });

  const saveMutation = useMutation({
    mutationFn: (values: AccessFormValues) => accessService.patchPolicy(values),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: QUERY_KEY });
      appToast.success(t("access.toast.saved"));
    },
    onError: (error) => {
      if (isApiError(error)) {
        appToast.error(error.message);
        return;
      }
      appToast.error(t("access.toast.failed"));
    },
  });

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("access.title")}
        description={t("access.description")}
      />

      {isLoading ? <Loading label={t("common.loading")} /> : null}
      {isError ? (
        <ErrorState
          title={t("access.error.title")}
          description={t("access.error.description")}
          retryLabel={t("common.retry")}
          onRetry={() => refetch()}
        />
      ) : null}

      {data ? (
        <AccessSettingsForm
          policy={data}
          canWrite={canWrite}
          isSaving={saveMutation.isPending}
          onSubmit={async (values) => {
            await saveMutation.mutateAsync(values);
          }}
        />
      ) : null}
    </div>
  );
}
