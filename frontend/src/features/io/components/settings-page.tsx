"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { SettingsForm } from "@/features/io/components/settings-form";
import { ioKeys } from "@/features/io/hooks/query-keys";
import type { PatchSettingsRequest } from "@/features/io/services/settings.service";
import { settingsService } from "@/features/io/services/settings.service";
import type { ExportJobScope } from "@/features/io/types";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type SettingsPageProps = {
  scope?: ExportJobScope;
  slug?: string;
};

export function SettingsPage({ scope = "platform", slug }: SettingsPageProps) {
  const { t } = useLocale();
  const { can } = usePermission();
  const tenant = scope === "tenant";
  const canWrite = can(
    tenant ? permissions.settings.tenantWrite : permissions.settings.write,
  );
  const queryClient = useQueryClient();
  const settingsKey = ioKeys.settings.all(scope);

  const settingsQuery = useQuery({
    queryKey: settingsKey,
    queryFn: () => settingsService.get(scope),
  });

  const save = useAppMutation({
    mutationFn: (body: PatchSettingsRequest) =>
      settingsService.patch(body, scope),
    onSuccess: (data) => {
      void queryClient.setQueryData(settingsKey, data);
      appToast.success(t("settings.toast.saved"));
    },
    onError: () => {
      appToast.error(t("settings.toast.save_failed"));
    },
  });

  const uploadLogo = useAppMutation({
    mutationFn: (file: File) => settingsService.uploadLogo(file, scope),
    onSuccess: (data) => {
      void queryClient.setQueryData(settingsKey, data);
      appToast.success(t("settings.toast.logo_saved"));
    },
  });

  const removeLogo = useAppMutation({
    mutationFn: () => settingsService.deleteLogo(scope),
    onSuccess: (data) => {
      void queryClient.setQueryData(settingsKey, data);
      appToast.success(t("settings.toast.logo_removed"));
    },
  });

  const homeHref =
    tenant && slug ? routes.tenant.home(slug) : routes.platform.home;
  const settingsHref =
    tenant && slug
      ? routes.tenant.settings.root(slug)
      : routes.platform.settings.root;

  return (
    <EntityPage
      title={t("settings.title")}
      description={
        tenant ? t("settings.tenant_description") : t("settings.description")
      }
      permission={
        tenant ? permissions.settings.tenantRead : permissions.settings.read
      }
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("settings.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: homeHref },
        { label: t("layout.section_io"), href: settingsHref },
        { label: t("settings.title") },
      ]}
    >
      {settingsQuery.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : settingsQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("settings.forbidden")}
          onRetry={() => void settingsQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : settingsQuery.data ? (
        <SettingsForm
          key={JSON.stringify(settingsQuery.data)}
          settings={settingsQuery.data}
          canWrite={canWrite}
          isSaving={save.isPending}
          isLogoUploading={uploadLogo.isPending}
          isLogoRemoving={removeLogo.isPending}
          showLocation={tenant}
          onSubmit={async (payload) => {
            await save.mutateAsync(payload);
          }}
          onUploadLogo={(file) => uploadLogo.mutate(file)}
          onRemoveLogo={() => removeLogo.mutate()}
        />
      ) : null}
    </EntityPage>
  );
}
