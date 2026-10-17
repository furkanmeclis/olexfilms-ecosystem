import { useQueryClient } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { MapPin, ScanLine } from '@/ui/icons';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { moveUnits, resolveScan } from './api';
import { useCurrentWarehouse } from './context';
import { ContinuousScanner, type ScanOutcome } from './continuous-scanner';
import { whError } from './format';
import { Card, Segmented } from './parts';
import { QtyStepper } from './qty-stepper';
import { createRetryKey } from './uuid';

type Moved = { barcode: string; name: string; to: string };

/**
 * Bin to bin move inside one warehouse: scan the target bin QR, then every
 * serial unit scanned moves onto it at once (POST /v1/warehouse/moves).
 */
export function MoveScreen() {
  const { t } = useTranslation('warehouse');
  const qc = useQueryClient();
  const { warehouse } = useCurrentWarehouse();
  const [scanning, setScanning] = useState(false);
  const [target, setTarget] = useState<{ uuid: string; code: string } | null>(null);
  const [source, setSource] = useState<{ uuid: string; code: string } | null>(null);
  const [locMode, setLocMode] = useState<'target' | 'source'>('target');
  const [qty, setQty] = useState(1);
  const [moved, setMoved] = useState<Moved[]>([]);
  // A fixed-barcode move carries request_uuid; a retry of the same move reuses it.
  const [retry] = useState(createRetryKey);

  const onCode = async (code: string): Promise<ScanOutcome> => {
    try {
      const r = await resolveScan(code, { warehouse: warehouse?.uuid });
      if (r.status !== 'ok') {
        return { ok: false, title: t(`scan_status.${r.status}`, { warehouse: r.location?.path[0]?.name ?? '', actual: '', expected: '' }) };
      }
      if (r.type === 'location' && r.location) {
        const loc = { uuid: r.location.uuid, code: r.location.full_code };
        if (locMode === 'source') {
          setSource(loc);
          setLocMode('target');
          return { ok: true, title: t('m.source_set', { location: loc.code }) };
        }
        setTarget(loc);
        return { ok: true, title: t('m.target_set', { location: loc.code }) };
      }
      if (r.type !== 'unit' || !r.unit) return { ok: false, title: t('m.scan_unit_not_product') };
      if (!target) return { ok: false, title: t('order_place.scan_location_first') };
      let res;
      if (r.unit.unit_kind === 'fixed') {
        const src = source ?? (r.unit.location ? { uuid: r.unit.location.uuid, code: r.unit.location.full_code } : null);
        if (!src) return { ok: false, title: t('fixed.source_scan') };
        const sig = `${r.unit.barcode}:${src.uuid}:${target.uuid}:${qty}`;
        res = await moveUnits({
          location_uuid: target.uuid,
          fixed: [{ barcode: r.unit.barcode, quantity: qty, source_location_uuid: src.uuid, request_uuid: retry.for(sig) }],
        });
        retry.done();
        setQty(1);
      } else {
        res = await moveUnits({ barcodes: [r.unit.barcode], location_uuid: target.uuid });
      }
      const to = res.location.full_code ?? res.location.code;
      setMoved((m) => [{ barcode: r.unit!.barcode, name: r.product?.name ?? '', to }, ...m].slice(0, 50));
      void qc.invalidateQueries({ queryKey: ['stock'] });
      return { ok: true, title: r.product?.name ?? r.unit.barcode, detail: `→ ${to}` };
    } catch (e) {
      return { ok: false, title: whError(e) };
    }
  };

  return (
    <>
      <Stack.Screen options={{ title: t('move.title'), headerLargeTitle: false }} />
      <Screen>
        <Text variant="callout" tone="muted">
          {t('m.move_hint')}
        </Text>
        <Card>
          <View className="flex-row items-center gap-2">
            <Icon as={MapPin} tone="primary" size={18} />
            <Text variant="headline" className="flex-1">
              {target ? t('m.target_bin', { location: target.code }) : t('order_place.no_target')}
            </Text>
          </View>
        </Card>
        <Button size="lg" title={t('m.scan_units')} icon={<Icon as={ScanLine} tone="primary-foreground" />} onPress={() => setScanning(true)} />
        {moved.length ? (
          <>
            <SectionHeader title={t('move.done', { count: moved.length, location: target?.code ?? '' })} />
            <View className="gap-2">
              {moved.map((m, i) => (
                <View key={`${m.barcode}-${i}`} className="gap-0.5 rounded-lg border border-border bg-card px-4 py-2.5">
                  <Text variant="callout" className="font-semibold" numberOfLines={1}>
                    {m.name || m.barcode}
                  </Text>
                  <Text variant="footnote" tone="muted" className="font-mono">
                    {m.barcode} → {m.to}
                  </Text>
                </View>
              ))}
            </View>
          </>
        ) : null}
      </Screen>
      <ContinuousScanner
        visible={scanning}
        onClose={() => setScanning(false)}
        title={t('move.title')}
        hint={t('m.move_hint')}
        onCode={onCode}
        footer={
          <>
            <View className="gap-2 rounded-lg bg-card p-2">
              <Text variant="footnote" tone="muted" className="ps-1 font-medium">
                {t('m.location_scan_sets')}
              </Text>
              <Segmented
                value={locMode}
                onChange={setLocMode}
                options={[
                  { value: 'target', label: target ? t('m.target_bin', { location: target.code }) : t('transfer.target') },
                  { value: 'source', label: source ? t('m.source_set', { location: source.code }) : t('fixed.source_label') },
                ]}
              />
            </View>
            <QtyStepper label={t('m.fixed_qty')} value={qty} onChange={setQty} />
          </>
        }
      />
    </>
  );
}
