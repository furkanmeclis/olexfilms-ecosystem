import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Alert, Pressable, ScrollView, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { ErrorState, ListSkeleton, useChildHref, useFormat } from '@/features/admin/kit';
import { PeriodBar } from '@/features/analytics-kit/period-bar';
import { DEFAULT_PERIOD, type PeriodState, type Preset, PRESETS, toQuery } from '@/features/analytics-kit/period';
import { previousRange } from '@/features/analytics-kit/envelope';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { List, ListRow } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';
import { PullRefresh } from '@/ui/pull-refresh';

import { supportOf, useReport, useReportCatalog } from './api';
import { canShareExport, type ExportFormat, shareReportExport } from './export';
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
      <Screen refreshControl={<PullRefresh busy={catalog.isRefetching} onRefresh={() => void catalog.refetch()} />}>
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
                icon="chart"
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

function ExportButton({ onPick, busy }: { onPick: (f: ExportFormat) => void; busy: boolean }) {
  const { t } = useTranslation('analytics');
  if (busy) return <ActivityIndicator />;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={t('export')}
      hitSlop={8}
      className="size-8 items-center justify-center"
      onPress={() =>
        Alert.alert(t('export'), t('export_body'), [
          { text: t('export_xlsx'), onPress: () => onPick('xlsx') },
          { text: t('export_csv'), onPress: () => onPick('csv') },
          { text: t('common:cancel'), style: 'cancel' },
        ])
      }>
      <Icon name="share" tone="primary" />
    </Pressable>
  );
}

/**
 * One report (TEC-1213): the period bar (presets, custom range, bucket size,
 * comparison) when the report takes a period, KPI tiles with deltas, the
 * series compared in one chart, distributions and rankings, the table with
 * sort, search and totals, and XLSX / CSV export to the share sheet.
 */
export function ReportScreen() {
  const { t } = useTranslation('analytics');
  const f = useFormat();
  const { key: raw } = useLocalSearchParams<{ key: string }>();
  const key = decodeURIComponent(raw ?? '');
  const catalog = useReportCatalog();
  const entry = catalog.data?.items.find((i) => i.key === key);
  const support = supportOf(entry);
  const initial = entry?.default_period && (PRESETS as readonly string[]).includes(entry.default_period) ? (entry.default_period as Preset) : DEFAULT_PERIOD.preset;
  const [state, setState] = useState<PeriodState | null>(null);
  const period = state ?? { ...DEFAULT_PERIOD, preset: initial };
  const limit = entry?.limited ? 50 : undefined;
  const report = useReport(key, { range: toQuery(period, support), limit, enabled: !!entry });
  const [exporting, setExporting] = useState(false);
  const prev = report.data ? previousRange(report.data as unknown as Record<string, unknown>) : null;

  const doExport = async (format: ExportFormat) => {
    setExporting(true);
    try {
      await shareReportExport(key, { ...toQuery(period, support), ...(limit ? { limit } : {}) }, format, entry?.title ?? report.data?.title ?? key);
    } catch (e) {
      Alert.alert(t('export_failed'), errorMessage(isApiError(e) ? e.code : undefined, t('common:error_generic')));
    } finally {
      setExporting(false);
    }
  };

  return (
    <>
      <Stack.Screen
        options={{
          title: entry?.title ?? report.data?.title ?? t('admin:reports.title'),
          headerLargeTitle: false,
          headerRight: entry && canShareExport() ? () => <ExportButton busy={exporting} onPick={(fmt) => void doExport(fmt)} /> : undefined,
        }}
      />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-3 pb-10"
        refreshControl={<PullRefresh busy={report.isRefetching} onRefresh={() => void report.refetch()} />}>
        {entry ? <PeriodBar value={period} onChange={setState} support={support} /> : null}
        <View className="gap-3 px-4">
          {catalog.isPending || (report.isPending && !!entry) ? (
            <ListSkeleton rows={4} inset={false} />
          ) : !entry ? (
            catalog.isError ? (
              <ErrorState error={catalog.error} onRetry={() => void catalog.refetch()} />
            ) : (
              <EmptyState icon="chart" title={t('admin:reports.unavailable')} />
            )
          ) : report.isError && !report.data ? (
            <ErrorState error={report.error} onRetry={() => void report.refetch()} />
          ) : report.data ? (
            <>
              {report.data.range_from && report.data.range_to ? (
                <Text variant="footnote" tone="muted">
                  {f.date(report.data.range_from)} – {f.date(report.data.range_to)}
                  {prev ? ` · ${t('compared_with', { range: `${f.date(prev.from)} – ${f.date(prev.to)}` })}` : ''}
                </Text>
              ) : null}
              <ReportView report={report.data} />
              <Text variant="footnote" tone="muted" className="text-center">
                {t('admin:reports.generated', { at: f.dateTime(report.data.generated_at) })}
              </Text>
            </>
          ) : null}
        </View>
      </ScrollView>
    </>
  );
}
