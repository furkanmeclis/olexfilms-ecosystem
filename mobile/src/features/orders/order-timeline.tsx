import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import type { components } from '@/core/api/schema';
import { useFormat } from '@/features/admin/kit';
import { Icon } from '@/ui/icon';
import { SectionHeader } from '@/ui/list';
import { Text } from '@/ui/text';

import { actionKey, newestFirst, orderAmounts, stepParts } from './order-summary';

type S = components['schemas'];
type TimelineEntry = S['OrderTimelineEntry'];
type LastActivity = S['OrderLastActivity'];

/** Translated label of a timeline or last activity action (TEC-1060 actions). */
export function useActionLabel() {
  const { t } = useTranslation('orders');
  return (action: string) => {
    const key = actionKey(action);
    if (!key) return action;
    // Statuses reuse the order status names of the admin catalog.
    return key.startsWith('status.') ? t(`admin:orders.${key}`) : t(key);
  };
}

/** "Melike Murat · Preparing": the user who took the order's latest step (TEC-1054 last_activity). */
export function useActivityText() {
  const { t } = useTranslation('orders');
  const label = useActionLabel();
  return (a: LastActivity | null | undefined) =>
    a ? t('card.activity', { name: a.user?.name ?? t('timeline.system'), action: label(a.action) }) : null;
}

/** "3 pcs · 12.5 m": what the order holds (TEC-1054 totals). */
export function useAmountText() {
  const { t } = useTranslation('orders');
  const f = useFormat();
  return (o: { total_quantity?: number | null; total_meters?: string | null }) => {
    const { pieces, meters } = orderAmounts(o);
    const parts = [
      pieces != null ? t('card.pieces', { n: f.number(pieces) }) : null,
      meters != null ? t('card.meters', { n: f.number(meters, 2) }) : null,
    ].filter(Boolean);
    return parts.length ? parts.join(' · ') : null;
  };
}

/** Order movement history (TEC-1059 / 5.4.1): who did what and when, newest first. */
export function OrderTimeline({ timeline }: { timeline: TimelineEntry[] | undefined }) {
  const { t } = useTranslation('orders');
  const f = useFormat();
  const label = useActionLabel();
  const rows = newestFirst(timeline);
  if (!rows.length) return null;
  return (
    <>
      <SectionHeader title={t('timeline.title')} />      <View className="gap-0 rounded-lg border border-border bg-card">
        {rows.map((s, i) => {
          const amount =
            s.meters != null
              ? t('card.meters', { n: f.number(s.meters, 2) })
              : s.quantity != null
                ? t('card.pieces', { n: f.number(s.quantity) })
                : null;
          const detail = [s.product, s.barcode, amount, s.lines != null ? t('timeline.lines', { n: s.lines }) : null]
            .filter(Boolean)
            .join(' · ');
          return (
            <View
              key={`${s.at}-${s.action}-${i}`}
              className={i < rows.length - 1 ? 'flex-row gap-3 border-b border-border px-4 py-3' : 'flex-row gap-3 px-4 py-3'}>
              <View className="pt-0.5">
                <Icon name="clock" tone="muted-foreground" size={16} />
              </View>
              <View className="flex-1 gap-0.5">
                <Text variant="callout" className="font-semibold" numberOfLines={1}>
                  {s.user?.name ?? t('timeline.system')}
                </Text>
                <Text variant="callout">{label(s.action)}</Text>
                {detail ? (
                  <Text variant="footnote" tone="muted" numberOfLines={2}>
                    {detail}
                  </Text>
                ) : null}
                {/* TEC-1057 / TEC-1059: the lines an edit changed and the plotter marks, one per row. */}
                {stepParts(s).map((p, k) => (
                  <Text key={`${p.key}-${k}`} variant="footnote" tone="muted" numberOfLines={2}>
                    {t(p.key, p.params)}
                  </Text>
                ))}
                {s.reason ? (
                  <Text variant="footnote" tone="muted" numberOfLines={3}>
                    {s.reason}
                  </Text>
                ) : null}
              </View>
              <Text variant="footnote" tone="muted" className="tabular-nums">
                {f.dateTime(s.at)}
              </Text>
            </View>
          );
        })}
      </View>
    </>
  );
}
