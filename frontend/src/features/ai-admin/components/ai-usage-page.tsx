"use client";

import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { UsageSummaryCard } from "@/features/ai-admin/components/usage-summary-card";
import { UsageTable } from "@/features/ai-admin/components/usage-table";
import { useAIUsageSummary } from "@/features/ai-admin/hooks/use-ai-admin";
import { aiUserName, currentPeriod } from "@/features/ai-admin/lib/quota";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Organization panel "AI kullanımı" (TEC-391): summary + usage ledger. */
export function AIUsagePage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.aiAdmin.usageRead);
  const [period, setPeriod] = useState(() => currentPeriod());
  const summary = useAIUsageSummary(period, canRead);

  const userOptions = useMemo(
    () =>
      (summary.data?.by_user ?? []).flatMap((row) =>
        row.user ? [{ value: row.user.uuid, label: aiUserName(row.user) }] : [],
      ),
    [summary.data?.by_user],
  );

  return (
    <EntityPage
      title={t("ai_admin.usage_title")}
      description={t("ai_admin.usage_description")}
      permission={permissions.aiAdmin.usageRead}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("ai_admin.usage_forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("ai_admin.usage_title") },
      ]}
    >
      <div className="space-y-6">
        {summary.isError ? (
          <ErrorState
            title={t("ai_admin.summary.failed")}
            onRetry={() => void summary.refetch()}
          />
        ) : (
          <UsageSummaryCard
            summary={summary.data}
            isLoading={summary.isLoading}
            period={period}
            onPeriodChange={setPeriod}
          />
        )}
        <UsageTable scope="tenant" slug={slug} userOptions={userOptions} />
      </div>
    </EntityPage>
  );
}
