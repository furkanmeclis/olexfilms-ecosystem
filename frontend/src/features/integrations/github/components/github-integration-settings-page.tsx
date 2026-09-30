"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { githubNavIcon } from "@/components/icons/github-icon";
import { PageHeader } from "@/components/layout/page-header";
import { permissions } from "@/config/permissions";
import { GitHubSettingsForm } from "@/features/integrations/github/components/github-settings-form";
import type { GitHubSettingsFormValues } from "@/features/integrations/github/schemas/github-settings-form";
import { githubIntegrationService } from "@/features/integrations/github/services/github-integration.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const QUERY_KEY = ["platform", "integrations", "github"] as const;

const GitHubNavIcon = githubNavIcon;

export function GitHubIntegrationSettingsPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const queryClient = useQueryClient();
  const canWrite = can(permissions.integrations.github.write);
  const callbackUrl =
    typeof window !== "undefined"
      ? `${window.location.origin}/api/auth/callback/github`
      : "/api/auth/callback/github";

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: QUERY_KEY,
    queryFn: () => githubIntegrationService.getSettings(),
  });

  const saveMutation = useMutation({
    mutationFn: (values: GitHubSettingsFormValues) =>
      githubIntegrationService.patchSettings({
        enabled: values.enabled,
        register_enabled: values.register_enabled,
        app_id: values.app_id,
        client_id: values.client_id,
        ...(values.client_secret?.trim()
          ? { client_secret: values.client_secret.trim() }
          : {}),
        ...(values.private_key?.trim()
          ? { private_key: values.private_key.trim() }
          : {}),
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: QUERY_KEY });
      appToast.success(t("integrations.github.toast.saved"));
    },
    onError: (error) => {
      if (isApiError(error)) {
        appToast.error(error.message);
        return;
      }
      appToast.error(t("integrations.github.toast.failed"));
    },
  });

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<GitHubNavIcon className="size-7" />}
        title={t("integrations.github.title")}
        description={t("integrations.github.description")}
      />

      {isLoading ? <Loading label={t("common.loading")} /> : null}
      {isError ? (
        <ErrorState
          title={t("integrations.github.error.title")}
          description={t("integrations.github.error.description")}
          retryLabel={t("common.retry")}
          onRetry={() => refetch()}
        />
      ) : null}

      {data ? (
        <GitHubSettingsForm
          settings={data}
          canWrite={canWrite}
          isSaving={saveMutation.isPending}
          callbackUrl={callbackUrl}
          onSubmit={async (values) => {
            await saveMutation.mutateAsync(values);
          }}
        />
      ) : null}
    </div>
  );
}
