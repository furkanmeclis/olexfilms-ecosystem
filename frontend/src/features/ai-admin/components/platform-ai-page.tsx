"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { AISettingsForm } from "@/features/ai-admin/components/ai-settings-form";
import { OrgQuotasTable } from "@/features/ai-admin/components/org-quotas-table";
import { UsageTable } from "@/features/ai-admin/components/usage-table";
import {
  useAIOrgQuotas,
  useAISettings,
  useUpdateAISettings,
} from "@/features/ai-admin/hooks/use-ai-admin";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const PLATFORM_AI_TABS = ["settings", "orgs", "usage"] as const;
type PlatformAITab = (typeof PLATFORM_AI_TABS)[number];

/** Organizations offered in the usage report filter (sorted by name). */
const ORG_DIRECTORY_PARAMS = { limit: 100, offset: 0, sort: "name" };

/**
 * /platform/ai (TEC-391, super_admin): AI settings, organization quota
 * table and the usage report of every organization; `?tab=` keeps the tab.
 */
export function PlatformAIPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const canManage = can(permissions.aiAdmin.settingsManage);

  const rawTab = searchParams.get("tab");
  const tab: PlatformAITab = PLATFORM_AI_TABS.includes(rawTab as PlatformAITab)
    ? (rawTab as PlatformAITab)
    : "settings";
  const setTab = useCallback(
    (next: string) => {
      const params = new URLSearchParams(searchParams.toString());
      if (next === "settings") params.delete("tab");
      else params.set("tab", next);
      const qs = params.toString();
      router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
    },
    [pathname, router, searchParams],
  );

  const settings = useAISettings(canManage);
  const update = useUpdateAISettings();
  const orgDirectory = useAIOrgQuotas(
    ORG_DIRECTORY_PARAMS,
    canManage && tab === "usage",
  );
  const organizationOptions = useMemo(
    () =>
      (orgDirectory.data?.items ?? []).map((row) => ({
        value: row.organization.uuid,
        label: row.organization.name,
      })),
    [orgDirectory.data?.items],
  );
  const modelOptions = useMemo(
    () =>
      (settings.data?.allowed_models ?? []).map((model) => ({
        value: model,
        label: model,
      })),
    [settings.data?.allowed_models],
  );

  return (
    <EntityPage
      title={t("ai_admin.title")}
      description={t("ai_admin.description")}
      permission={permissions.aiAdmin.settingsManage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("ai_admin.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("ai_admin.title") },
      ]}
    >
      <Tabs value={tab} onValueChange={setTab} className="space-y-4">
        <TabsList>
          {PLATFORM_AI_TABS.map((value) => (
            <TabsTrigger key={value} value={value}>
              {t(`ai_admin.tabs.${value}`)}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="settings">
          {settings.isLoading ? <Loading /> : null}
          {settings.isError ? (
            <ErrorState
              title={t("ai_admin.settings.failed")}
              onRetry={() => void settings.refetch()}
            />
          ) : null}
          {settings.data ? (
            <AISettingsForm
              key={settings.data.updated_at}
              settings={settings.data}
              isSaving={update.isPending}
              onSubmit={(body) =>
                update.mutateAsync(body).then(
                  () => appToast.success(t("ai_admin.toast.settings_saved")),
                  (error: unknown) => {
                    appToast.error(
                      isApiError(error)
                        ? error.message
                        : t("ai_admin.toast.save_failed"),
                    );
                    throw error;
                  },
                )
              }
            />
          ) : null}
        </TabsContent>
        <TabsContent value="orgs">
          {tab === "orgs" ? (
            <OrgQuotasTable
              defaultQuota={settings.data?.default_monthly_token_quota}
            />
          ) : null}
        </TabsContent>
        <TabsContent value="usage">
          {tab === "usage" ? (
            <UsageTable
              scope="platform"
              organizationOptions={organizationOptions}
              modelOptions={modelOptions}
            />
          ) : null}
        </TabsContent>
      </Tabs>
    </EntityPage>
  );
}
