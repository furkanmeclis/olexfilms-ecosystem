"use client";

import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { PeriodSelect } from "@/features/ai-admin/components/period-select";
import { QuotaBar } from "@/features/ai-admin/components/quota-bar";
import { aiUserName, isUnlimitedQuota } from "@/features/ai-admin/lib/quota";
import type {
  AIQuotaUsage,
  AIUsageSummary,
} from "@/features/ai-admin/services/ai-admin.service";
import { useLocale } from "@/providers/locale-provider";

const TOP_USERS = 5;

/** Monthly summary of the organization's AI usage (TEC-391). */
export function UsageSummaryCard({
  summary,
  isLoading,
  period,
  onPeriodChange,
}: {
  summary: AIUsageSummary | undefined;
  isLoading?: boolean;
  period: string;
  onPeriodChange: (period: string) => void;
}) {
  const { t, format } = useLocale();

  return (
    <Card data-testid="ai-usage-summary">
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          {t("ai_admin.summary.title")}
          {summary && !summary.enabled ? (
            <Badge variant="warning">{t("ai_admin.status.disabled")}</Badge>
          ) : null}
        </CardTitle>
        <CardDescription>{t("ai_admin.summary.description")}</CardDescription>
        <CardAction>
          <PeriodSelect value={period} onChange={onPeriodChange} />
        </CardAction>
      </CardHeader>
      <CardContent>
        {isLoading || !summary ? (
          <div className="grid gap-4 md:grid-cols-3">
            <Skeleton className="h-24" />
            <Skeleton className="h-24" />
            <Skeleton className="h-24" />
          </div>
        ) : (
          <div className="space-y-6">
            <div className="grid gap-4 md:grid-cols-3">
              <PoolTile
                label={t("ai_admin.summary.org_pool")}
                usage={summary.quota}
              />
              {summary.system_pool ? (
                <PoolTile
                  label={t("ai_admin.summary.system_pool")}
                  usage={summary.system_pool}
                />
              ) : null}
              <div className="space-y-1 rounded-md border p-4">
                <p className="text-muted-foreground text-xs">
                  {t("ai_admin.summary.requests")}
                </p>
                <p className="text-2xl font-semibold tabular-nums">
                  {format.number(summary.totals.requests)}
                </p>
                <p className="text-muted-foreground text-xs tabular-nums">
                  {t("ai_admin.summary.in_out", {
                    input: format.number(summary.totals.input_tokens),
                    output: format.number(summary.totals.output_tokens),
                  })}
                </p>
              </div>
            </div>

            <div className="grid gap-6 md:grid-cols-2">
              <Breakdown
                title={t("ai_admin.summary.by_channel")}
                rows={summary.by_channel.map((row) => ({
                  key: `${row.channel}-${row.pool}`,
                  label: `${t(`ai_admin.channel.${row.channel}`)} · ${t(
                    `ai_admin.pool.${row.pool}`,
                  )}`,
                  tokens: row.tokens,
                  requests: row.requests,
                }))}
              />
              <Breakdown
                title={t("ai_admin.summary.by_user")}
                rows={summary.by_user.slice(0, TOP_USERS).map((row, i) => ({
                  key: row.user?.uuid ?? `none-${i}`,
                  label: row.user
                    ? aiUserName(row.user)
                    : t("ai_admin.no_user"),
                  tokens: row.tokens,
                  requests: row.requests,
                }))}
              />
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function PoolTile({ label, usage }: { label: string; usage: AIQuotaUsage }) {
  const { t, format } = useLocale();
  const unlimited = isUnlimitedQuota(usage.limit);
  return (
    <div className="space-y-2 rounded-md border p-4">
      <p className="text-muted-foreground text-xs">{label}</p>
      <p className="text-2xl font-semibold tabular-nums">
        {format.number(usage.used)}
        <span className="text-muted-foreground ms-1 text-sm font-normal">
          /{" "}
          {unlimited
            ? t("ai_admin.quota.unlimited")
            : format.number(usage.limit)}
        </span>
      </p>
      <QuotaBar percent={unlimited ? null : usage.percent} />
      {usage.remaining != null ? (
        <p className="text-muted-foreground text-xs tabular-nums">
          {t("ai_admin.summary.remaining", {
            value: format.number(usage.remaining),
          })}
        </p>
      ) : null}
    </div>
  );
}

function Breakdown({
  title,
  rows,
}: {
  title: string;
  rows: { key: string; label: string; tokens: number; requests: number }[];
}) {
  const { t, format } = useLocale();
  return (
    <section className="space-y-2">
      <h3 className="text-sm font-medium">{title}</h3>
      {rows.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("ai_admin.summary.no_usage")}
        </p>
      ) : (
        <ul className="divide-y rounded-md border text-sm">
          {rows.map((row) => (
            <li
              key={row.key}
              className="flex items-center justify-between gap-4 px-3 py-2"
            >
              <span className="min-w-0 truncate">{row.label}</span>
              <span className="text-muted-foreground shrink-0 tabular-nums">
                {t("ai_admin.summary.tokens_requests", {
                  tokens: format.number(row.tokens),
                  requests: format.number(row.requests),
                })}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
