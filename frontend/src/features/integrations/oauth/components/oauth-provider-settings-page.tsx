"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import type { PermissionSlug } from "@/config/permissions";
import { OAuthProviderSettingsForm } from "@/features/integrations/oauth/components/oauth-provider-settings-form";
import { oauthProviderIcons } from "@/features/integrations/oauth/oauth-provider-icons";
import type { OAuthProviderFormValues } from "@/features/integrations/oauth/schemas/oauth-provider-form";
import {
  oauthProviderService,
  type OAuthProviderSlug,
} from "@/features/integrations/oauth/services/oauth-provider.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type OAuthProviderSettingsPageProps = {
  provider: OAuthProviderSlug;
  writePermission: PermissionSlug;
};

export function OAuthProviderSettingsPage({
  provider,
  writePermission,
}: OAuthProviderSettingsPageProps) {
  const Icon = oauthProviderIcons[provider];
  const { t } = useLocale();
  const { can } = usePermission();
  const queryClient = useQueryClient();
  const canWrite = can(writePermission);
  const ns = `integrations.${provider}`;
  const queryKey = ["platform", "integrations", provider] as const;
  const callbackUrl =
    typeof window !== "undefined"
      ? `${window.location.origin}/api/auth/callback/${provider}`
      : `/api/auth/callback/${provider}`;

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey,
    queryFn: () => oauthProviderService.getSettings(provider),
  });

  const saveMutation = useMutation({
    mutationFn: (values: OAuthProviderFormValues) =>
      oauthProviderService.patchSettings(provider, {
        login_enabled: values.login_enabled,
        register_enabled: values.register_enabled,
        client_id: values.client_id,
        ...(values.client_secret?.trim()
          ? { client_secret: values.client_secret.trim() }
          : {}),
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey });
      appToast.success(t(`${ns}.toast.saved`));
    },
    onError: (error) => {
      if (isApiError(error)) {
        appToast.error(error.message);
        return;
      }
      appToast.error(t(`${ns}.toast.failed`));
    },
  });

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Icon className="size-7" />}
        title={t(`${ns}.title`)}
        description={t(`${ns}.description`)}
      />

      {isLoading ? <Loading label={t("common.loading")} /> : null}
      {isError ? (
        <ErrorState
          title={t(`${ns}.error.title`)}
          description={t(`${ns}.error.description`)}
          retryLabel={t("common.retry")}
          onRetry={() => refetch()}
        />
      ) : null}

      {data ? (
        <OAuthProviderSettingsForm
          provider={provider}
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
