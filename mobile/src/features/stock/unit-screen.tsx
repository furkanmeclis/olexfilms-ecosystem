import { Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { Barcode } from '@/ui/icons';
import { EmptyState } from '@/ui/empty-state';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { formatDate, formatNumber } from '@/features/warehouse/format';
import { Card, ErrorState, Field, ListSkeleton } from '@/features/warehouse/parts';
import { unitStatusTone } from '@/features/warehouse/scan-result-card';

import { useUnitHistory } from './api';

/** One barcode: product, state, holder and its movement history. */
export function UnitScreen() {
  const { t } = useTranslation('warehouse');
  const { barcode } = useLocalSearchParams<{ barcode: string }>();
  const q = useUnitHistory(barcode);
  const h = q.data;

  return (
    <>
      <Stack.Screen options={{ title: barcode, headerLargeTitle: false }} />
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {q.isLoading ? (
          <ListSkeleton rows={4} />
        ) : !h ? (
          isApiError(q.error) && q.error.status === 404 ? (
            <EmptyState icon={Barcode} title={t('stock.unit.not_found')} body={t('stock.unit.not_found_hint')} />
          ) : (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          )
        ) : (
          <>
            <Card>
              <View className="flex-row items-start gap-2 pb-1">
                <Text variant="headline" className="flex-1">
                  {h.unit.product.name}
                </Text>
                <StatusPill
                  tone={unitStatusTone(h.current?.status ?? h.unit.status)}
                  label={t(`stock.status.${h.current?.status ?? h.unit.status}`)}
                />
              </View>
              <Field label={t('fields.barcode')} value={h.unit.barcode} mono />
              <Field label={t('fields.sku')} value={h.unit.product.sku} mono />
              <Field label={t('stock.unit.kind')} value={t(`stock.unit_kind.${h.unit.unit_kind}`)} />
              {h.unit.remaining_meters ? (
                <Field
                  label={t('fields.meters')}
                  value={`${formatNumber(h.unit.remaining_meters, 2)} / ${formatNumber(h.unit.initial_meters, 2)} m`}
                />
              ) : null}
              {h.current ? (
                <>
                  <Field label={t('stock.unit.holder')} value={h.current.holder.name} />
                  <Field
                    label={t('stock.unit.owner')}
                    value={
                      h.current.owner.masked
                        ? t('stock.history.masked')
                        : (h.current.owner.location?.code ?? h.current.owner.organization?.name ?? t('scan.unplaced'))
                    }
                  />
                </>
              ) : null}
            </Card>

            {h.holdings.length > 1 ? (
              <>
                <SectionHeader title={t('stock.unit.holdings')} />
                <Card>
                  {h.holdings.map((x, i) => (
                    <Field
                      key={i}
                      label={x.owner.location?.code ?? x.holder.name}
                      value={`× ${formatNumber(x.quantity)}`}
                    />
                  ))}
                </Card>
              </>
            ) : null}

            <SectionHeader title={t('stock.history.title', { total: h.movements.total })} />
            {h.movements.items.length === 0 ? (
              <Text variant="callout" tone="muted" className="px-4">
                {t('stock.history.empty')}
              </Text>
            ) : (
              <View className="gap-2">
                {h.movements.items.map((m) => (
                  <View key={m.uuid} className="gap-0.5 rounded-lg border border-border bg-card px-4 py-3">
                    <View className="flex-row items-center gap-2">
                      <Text variant="callout" className="flex-1 font-semibold">
                        {t(`stock.movement.${m.type}`)}
                      </Text>
                      <Text variant="footnote" tone="muted">
                        {formatDate(m.created_at)}
                      </Text>
                    </View>
                    <Text variant="footnote" tone="muted" numberOfLines={2}>
                      {[
                        m.organization_masked ? t('stock.history.masked') : m.organization?.name,
                        m.to_owner?.location?.code,
                        m.document ? t(`stock.history.document.${m.document.type}`) : null,
                        m.actor ? t('stock.history.by', { name: m.actor.name }) : null,
                      ]
                        .filter(Boolean)
                        .join(' · ')}
                    </Text>
                  </View>
                ))}
              </View>
            )}
          </>
        )}
      </Screen>
    </>
  );
}
