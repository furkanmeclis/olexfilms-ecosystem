import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RefreshControl, ScrollView, View } from 'react-native';

import { Chips, ErrorState, ListSkeleton, useChildHref, useFormat } from '@/features/admin/kit';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { type Period, PERIODS, useReport, useReportCatalog } from './api';
import { ReportView } from './report-view';

/** Reports the caller may open; each opens the generic report screen. */
export function ReportsScreen() {
  const { t } = useTranslation('admin');
  const href = useChildHref();
  const catalog = useReportCatalog();
  const items = catalog.data?.items ?? [];

  return (
    <>
      <Stack.Screen options={{ title: t('reports.title') }} />
      <Screen refreshControl={<RefreshControl refreshing={catalog.isRefetching} onRefresh={() => void catalog.refetch()} />}>
        {catalog.isPending ? (
          <ListSkeleton inset={false} />
        ) : catalog.isError && !items.length ? (
          <ErrorState error={catalog.error} onRetry={() => void catalog.refetch()} />
        ) : !items.length ? (
          <EmptyState icon="chart" title={t('reports.empty')} body={t('reports.empty_body')} />
        ) : (
          <List>
            {items.map((r, i) => (
              <ListRow
                key={r.key}
                title={r.title}
                subtitle={t(`reports.kind.${r.kind}`)}
                onPress={() => router.push(href(encodeURIComponent(r.key)))}
                last={i === items.length - 1}
              />
            ))}
          </List>
        )}
      </Screen>
    </>
  );
}

/** One report with a period selector when the report takes a period. */
export function ReportScreen() {
  const { t } = useTranslation('admin');
  const f = useFormat();
  const { key: raw } = useLocalSearchParams<{ key: string }>();
  const key = decodeURIComponent(raw ?? '');
  const catalog = useReportCatalog();
  const entry = catalog.data?.items.find((i) => i.key === key);
  const periods = (entry?.periods ?? []).filter((p): p is Period => (PERIODS as string[]).includes(p));
  const [period, setPeriod] = useState<Period | undefined>(undefined);
  const active = entry?.periodic ? (period ?? ((entry.default_period as Period | null) ?? '30d')) : undefined;
  const report = useReport(key, { period: active, limit: entry?.limited ? 20 : undefined, enabled: !!entry });

  return (
    <>
      <Stack.Screen options={{ title: entry?.title ?? report.data?.title ?? t('reports.title'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-3 pb-10"
        refreshControl={<RefreshControl refreshing={report.isRefetching} onRefresh={() => void report.refetch()} />}>
        {entry?.periodic && periods.length > 0 ? (
          <Chips
            options={periods.map((p) => ({ key: p, label: t(`reports.period.${p}`) }))}
            value={active ?? '30d'}
            onChange={setPeriod}
          />
        ) : null}
        <View className="gap-3 px-4">
          {catalog.isPending || (report.isPending && !!entry) ? (
            <ListSkeleton rows={4} inset={false} />
          ) : !entry ? (
            catalog.isError ? (
              <ErrorState error={catalog.error} onRetry={() => void catalog.refetch()} />
            ) : (
              <EmptyState icon="chart" title={t('reports.unavailable')} />
            )
          ) : report.isError && !report.data ? (
            <ErrorState error={report.error} onRetry={() => void report.refetch()} />
          ) : report.data ? (
            <>
              {report.data.range_from && report.data.range_to ? (
                <Text variant="footnote" tone="muted">
                  {f.date(report.data.range_from)} – {f.date(report.data.range_to)}
                </Text>
              ) : null}
              <ReportView report={report.data} />
              <Text variant="footnote" tone="muted" className="text-center">
                {t('reports.generated', { at: f.dateTime(report.data.generated_at) })}
              </Text>
            </>
          ) : null}
        </View>
      </ScrollView>
    </>
  );
}
