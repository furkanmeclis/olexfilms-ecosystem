import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { Boxes, MapPin, Package } from '@/ui/icons';
import { Icon } from '@/ui/icon';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import type { ScanResult } from './api';
import { formatNumber } from './format';
import { Field } from './parts';

export function unitStatusTone(status: string): StatusTone {
  if (status === 'void' || status === 'used') return 'problem';
  if (status === 'in_transit' || status === 'reserved' || status === 'printed') return 'progress';
  return 'done';
}

/** Location path as "Warehouse › Room › A1 › S2". */
export function locationPath(loc: NonNullable<ScanResult['location']>) {
  return loc.path.length ? loc.path.map((p) => p.code || p.name).join(' › ') : loc.full_code;
}

/** What a scanned code is: location, unit (with product and place) or product. */
export function ScanResultCard({ result }: { result: ScanResult }) {
  const { t } = useTranslation('warehouse');
  const { unit, product, location } = result;

  if (result.status !== 'ok') {
    return (
      <View className="gap-1">
        <StatusPill tone="problem" label={t(`scan_status.${result.status}`, { warehouse: '', actual: '', expected: '' })} />
        <Text variant="title" selectable className="font-mono">
          {result.code}
        </Text>
      </View>
    );
  }

  const icon = result.type === 'location' ? MapPin : result.type === 'unit' ? Boxes : Package;
  const heading =
    result.type === 'location' && location
      ? location.name || location.full_code
      : product
        ? product.name
        : result.code;

  return (
    <View className="gap-2">
      <View className="flex-row items-center gap-3">
        <View className="size-10 items-center justify-center rounded-md bg-secondary">
          <Icon as={icon} tone="primary" size={20} />
        </View>
        <View className="flex-1 gap-0.5">
          <Text variant="caption" tone="muted">
            {t(`scan.type.${result.type || 'unit'}`)}
            {result.matched_by ? ` · ${t(`scan.matched_by.${result.matched_by}`)}` : ''}
          </Text>
          <Text variant="headline" numberOfLines={2}>
            {heading}
          </Text>
        </View>
        {unit ? <StatusPill tone={unitStatusTone(unit.status)} label={t(`stock.status.${unit.status}`)} /> : null}
      </View>

      <View>
        {unit ? <Field label={t('fields.barcode')} value={unit.barcode} mono /> : null}
        {product ? <Field label={t('fields.sku')} value={product.sku} mono /> : null}
        {unit ? <Field label={t('fields.type')} value={t(`stock.unit_kind.${unit.unit_kind}`)} /> : null}
        {unit?.quantity_on_hand !== null && unit?.quantity_on_hand !== undefined ? (
          <Field label={t('fields.quantity')} value={formatNumber(unit.quantity_on_hand)} />
        ) : null}
        {unit?.remaining_meters ? (
          <Field
            label={t('fields.meters')}
            value={`${formatNumber(unit.remaining_meters, 2)} / ${formatNumber(unit.initial_meters, 2)} m`}
          />
        ) : null}
        {unit ? (
          <Field
            label={t('fields.location')}
            value={unit.location ? locationPath(unit.location) : t('scan.unplaced')}
          />
        ) : null}
        {unit?.holder ? <Field label={t('fields.holder')} value={unit.holder.name} /> : null}
        {location ? (
          <>
            <Field label={t('fields.code')} value={location.full_code} mono />
            <Field label={t('scan.path')} value={locationPath(location)} />
            {location.type ? <Field label={t('fields.type')} value={t(`type.${location.type}`)} /> : null}
          </>
        ) : null}
      </View>
    </View>
  );
}
