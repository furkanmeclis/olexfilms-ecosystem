import { router, usePathname } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { useCan } from '@/core/spaces';
import { ListSkeleton, useFormat } from '@/features/admin/kit';
import { Button } from '@/ui/button';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { transferTone, useOrderReturns } from './api';
import { orderReturnCreate, type OrderReturnsTarget } from './returns';

/** The transfer screens live in the workplace menu only. */
const TRANSFERS = '/bayi/menu/transfers';

/**
 * TEC-1220 (web OrderReturnsCard, TEC-1085): the returns linked to an order on the
 * order page, and "Create return": the seller (transfers.approve) opens it in the
 * buyer's name, the buyer (transfers.request) its own return to the seller; both
 * preselect the order. Hidden without a transfer permission or when the transfers
 * module is off (the list answers an error). Rows and the button open the transfer
 * screens in the workplace; in the warehouse and admin spaces, which have none,
 * the list is read only. No amounts: the section never shows a price.
 */
export function OrderReturnsSection({ order }: { order: OrderReturnsTarget }) {
  const { t } = useTranslation('transfers');
  const f = useFormat();
  const request = useCan('transfers.request');
  const approve = useCan('transfers.approve');
  const workplace = usePathname().startsWith('/bayi');
  const canRead = request || approve;
  const returns = useOrderReturns(order.uuid, canRead);
  if (!canRead || returns.isError) return null;

  const create = workplace ? orderReturnCreate(order, { request, approve }) : null;
  const items = returns.data ?? [];

  return (
    <View>
      <SectionHeader title={t('order_returns.title')} />
      <View className="gap-2">
        {create ? (
          <Button
            title={t('order_returns.create')}
            icon="undo"
            variant="outline"
            onPress={() =>
              router.push({
                pathname: `${TRANSFERS}/new` as never,
                params: { kind: 'return', order: create.order, ...(create.from ? { from: create.from } : {}) },
              })
            }
          />
        ) : null}
        {returns.isPending ? (
          <ListSkeleton rows={1} inset={false} />
        ) : !items.length ? (
          <Text variant="callout" tone="muted" className="px-4">
            {t('order_returns.empty')}
          </Text>
        ) : (
          <List>
            {items.map((r, i, arr) => (
              <ListRow
                key={r.uuid}
                icon="undo"
                title={r.transfer_no}
                subtitle={`${t('items', { count: r.item_count })} · ${f.dateTime(r.created_at)}`}
                trailing={<StatusPill label={t(`status.${r.status}`)} tone={transferTone(r.status)} />}
                onPress={workplace ? () => router.push(`${TRANSFERS}/${r.uuid}` as never) : undefined}
                last={i === arr.length - 1}
              />
            ))}
          </List>
        )}
      </View>
    </View>
  );
}
