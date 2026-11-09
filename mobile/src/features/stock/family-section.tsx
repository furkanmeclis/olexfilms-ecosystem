import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { SectionHeader } from '@/ui/list';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { formatNumber, go } from '@/features/warehouse/format';

import { useUnitFamily } from './api';
import { type FamilyStatus, familyStatusTone, looksLikeUnitCode } from './family';

/** One member of a split family: barcode, meters, status, holder, order. */
export type FamilyRow = {
  barcode: string;
  depth: number;
  meters: string | null;
  status: FamilyStatus;
  holder?: string | null;
  orderNo?: string | null;
};

export function FamilyRowView({ row, onPress }: { row: FamilyRow; onPress?: () => void }) {
  const { t } = useTranslation('warehouse');
  return (
    <Pressable
      accessibilityRole="button"
      disabled={!onPress}
      onPress={onPress}
      style={{ marginStart: Math.min(row.depth, 4) * 12 }}
      className="gap-1 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
      <View className="flex-row items-center gap-2">
        <Text variant="callout" selectable className="flex-1 font-mono font-semibold" numberOfLines={1}>
          {row.barcode}
        </Text>
        {row.meters ? <Text variant="callout">{`${formatNumber(row.meters, 2)} m`}</Text> : null}
      </View>
      <View className="flex-row flex-wrap items-center gap-2">
        <StatusPill tone={familyStatusTone(row.status)} label={t(`stock.family_status.${row.status}`)} />
        {row.depth === 0 ? (
          <Text variant="footnote" tone="muted">
            {t('stock.family.roll')}
          </Text>
        ) : null}
        <Text variant="footnote" tone="muted" numberOfLines={1} className="flex-1">
          {[row.holder, row.orderNo ? t('stock.family.order', { no: row.orderNo }) : null].filter(Boolean).join(' · ')}
        </Text>
      </View>
    </Pressable>
  );
}

/**
 * Stock search by a main code (TEC-1079): the roll and every part cut from
 * it (`-3M`, `-3M-2`, ...), wherever each is now, above the search results.
 * Hidden while the search is not a barcode or the family is empty.
 */
export function FamilySection({ q, base }: { q: string; base: string }) {
  const { t } = useTranslation('warehouse');
  const org = useSession((s) => s.me?.active_organization_uuid ?? '');
  const code = looksLikeUnitCode(q) ? q.trim() : '';
  const family = useUnitFamily(org, code);
  const items = code ? (family.data?.items ?? []) : [];
  if (!items.length) return null;
  return (
    <View className="gap-2">
      <SectionHeader title={t('stock.family.title')} />
      {items.map((u) => (
        <FamilyRowView
          key={u.uuid}
          row={{
            barcode: u.barcode,
            depth: u.family_depth ?? 0,
            // The roll shows what is left on it, a part what was cut as it.
            meters: (u.family_depth ?? 0) === 0 ? u.remaining_meters : (u.split_meters ?? u.remaining_meters),
            status: u.family_status ?? 'other',
            holder: u.holder?.name,
            orderNo: u.order?.order_no,
          }}
          onPress={() => go(`${base}/unit/${encodeURIComponent(u.barcode)}`)}
        />
      ))}
    </View>
  );
}
