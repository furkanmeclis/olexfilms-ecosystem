import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { Sheet } from '@/features/services/sheet';
import { DetailSkeleton, ErrorState } from '@/features/services/ui';
import { Text } from '@/ui/text';

import { formatMoney } from './lib';

type StatementOrder = components['schemas']['CariStatementOrder'];

/**
 * TEC-1035: lines and barcodes of the order behind a cari row. The statement
 * of the cari (same data as the web statement) carries the order of each
 * order row; the sheet reads it by the row uuid.
 */
export function OrderDetailSheet({
  cariUuid,
  entryUuid,
  queryKey,
  onClose,
}: {
  cariUuid: string;
  entryUuid: string;
  queryKey: readonly unknown[];
  onClose: () => void;
}) {
  const { t, i18n } = useTranslation('accounting');
  const statement = useQuery({
    queryKey: [...queryKey, 'statement', cariUuid],
    queryFn: () => unwrap(api.GET('/v1/accounting/cari/{uuid}/statement', { params: { path: { uuid: cariUuid } } })),
  });
  const order: StatementOrder | null | undefined = statement.data?.lines.find((l) => l.uuid === entryUuid)?.order;

  return (
    <Sheet title={order ? order.order_no : t('order_detail.title')} saveTitle={t('order_detail.done')} onSave={onClose} onClose={onClose}>
      {statement.isPending ? (
        <DetailSkeleton />
      ) : statement.error ? (
        <ErrorState error={statement.error} onRetry={() => void statement.refetch()} />
      ) : !order ? (
        <Text variant="callout" tone="muted">
          {t('order_detail.empty')}
        </Text>
      ) : (
        order.items.map((it) => (
          <View key={it.product_uuid} className="gap-1 rounded-lg border border-border bg-card px-4 py-3">
            <Text variant="callout" className="font-semibold">
              {it.product_name}
            </Text>
            {it.product_sku ? (
              <Text variant="footnote" tone="muted">
                {it.product_sku}
              </Text>
            ) : null}
            <Text variant="footnote" className="tabular-nums">
              {it.meters != null ? t('order_detail.meters', { n: it.meters }) : t('order_detail.pieces', { n: it.quantity ?? 0 })}
              {' × '}
              {formatMoney(it.unit_price, order.currency, i18n.language)}
              {' = '}
              {formatMoney(it.line_total, order.currency, i18n.language)}
            </Text>
            <Text variant="caption" tone="muted" className="mt-1">
              {t('order_detail.barcodes')}
            </Text>
            {it.units.length === 0 ? (
              <Text variant="footnote" tone="muted">
                {t('order_detail.no_barcodes')}
              </Text>
            ) : (
              <View className="flex-row flex-wrap gap-1.5">
                {it.units.map((u) => (
                  <View key={u.uuid} className="rounded-md bg-secondary px-2 py-1">
                    <Text variant="footnote" className="font-mono">
                      {u.meters != null ? `${u.barcode} · ${t('order_detail.meters', { n: u.meters })}` : u.barcode}
                    </Text>
                  </View>
                ))}
              </View>
            )}
          </View>
        ))
      )}
    </Sheet>
  );
}
