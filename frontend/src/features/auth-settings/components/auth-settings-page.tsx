"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Settings2 } from "lucide-react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { permissions } from "@/config/permissions";
import { AuthSettingsForm } from "@/features/auth-settings/components/auth-settings-form";
import type { AuthSettingsFormValues } from "@/features/auth-settings/schemas/auth-settings-form";
import { authSettingsService } from "@/features/auth-settings/services/auth-settings.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const QUERY_KEY = ["platform", "auth", "settings"] as const;

export function AuthSettingsPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const queryClient = useQueryClient();
  const canWrite = can(permissions.authSettings.write);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: QUERY_KEY,
    queryFn: () => authSettingsService.getSettings(),
  });

  const saveMutation = useMutation({
    mutationFn: (values: AuthSettingsFormValues) =>
      authSettingsService.patchSettings({
        registration_enabled: values.registration_enabled,
        password_login_enabled: values.password_login_enabled,
        password_register_enabled: values.password_register_enabled,
        passkey_login_enabled: values.passkey_login_enabled,
        ...(values.default_role_uuid
          ? { default_role_uuid: values.default_role_uuid }
          : { clear_default_role: true }),
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: QUERY_KEY });
      appToast.success(t("auth.settings.toast.saved"));
    },
    onError: (error) => {
      if (isApiError(error)) {
        appToast.error(error.message);
        return;
      }
      appToast.error(t("auth.settings.toast.failed"));
    },
  });

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Settings2 className="size-7" />}
        title={t("auth.settings.title")}
        description={t("auth.settings.description")}
      />

      {isLoading ? <Loading label={t("common.loading")} /> : null}
      {isError ? (
        <ErrorState
          title={t("auth.settings.error.title")}
          description={t("auth.settings.error.description")}
          retryLabel={t("common.retry")}
          onRetry={() => refetch()}
        />
      ) : null}

      {data ? (
        <AuthSettingsForm
          settings={data}
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
