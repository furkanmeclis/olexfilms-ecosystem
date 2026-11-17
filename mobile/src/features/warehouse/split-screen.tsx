import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { resolveScan } from './api';
import { ContinuousScanner, type ScanOutcome } from './continuous-scanner';
import { whError } from './format';
import { useLabelPrint } from './label-print';
import { parseMeters } from './split-meters';
import { Card, Field } from './parts';
import { uuidv4 } from './uuid';

type Split = components['schemas']['StockSplit'];
type Roll = { barcode: string; product: string; remaining: string };

/**
 * Standalone roll cut (TEC-1230): scan a roll on hand, enter the meters,
 * cut them off as a new unit (POST /v1/stock/splits) and print the labels.
 * The backend applies the product's minimum cut; a roll left under it is a
 * remnant ("ragle") and is shown as such.
 */
export function SplitScreen() {
  const { t } = useTranslation('warehouse');
  const [scanning, setScanning] = useState(false);
  const [roll, setRoll] = useState<Roll | null>(null);
  const [meters, setMeters] = useState('');
  const [key, setKey] = useState(uuidv4);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<Split | null>(null);
  const labels = useLabelPrint();

  const onCode = async (code: string): Promise<ScanOutcome> => {
    try {
      const r = await resolveScan(code, { expect: 'unit' });
      const u = r.unit;
      if (r.status !== 'ok' || !u) return { ok: false, title: t('split.not_found') };
      if (u.remaining_meters === null) return { ok: false, title: t('split.not_roll') };
      setRoll({ barcode: u.barcode, product: r.product?.name ?? u.barcode, remaining: u.remaining_meters });
      setResult(null);
      setError(null);
      setKey(uuidv4());
      return { ok: true, title: r.product?.name ?? u.barcode, detail: `${u.remaining_meters} m` };
    } catch (e) {
      return { ok: false, title: whError(e) };
    }
  };

  const cut = async () => {
    const m = parseMeters(meters);
    if (!roll || !m) {
      setError(t('split.meters_invalid'));
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const out = await unwrap(api.POST('/v1/stock/splits', { body: { barcode: roll.barcode, meters: m, idempotency_key: key } }));
      setResult(out);
      setRoll({ ...roll, remaining: out.source.remaining_meters });
      setMeters('');
      setKey(uuidv4());
    } catch (e) {
      setError(whError(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Stack.Screen options={{ title: t('split.title'), headerLargeTitle: false }} />
      <Screen>
        <Button size="lg" title={roll ? t('split.scan_other') : t('split.scan_roll')} icon={<Icon name="scan" tone="primary-foreground" />} onPress={() => setScanning(true)} />
        {roll ? (
          <Card>
            <Text variant="headline" numberOfLines={2}>
              {roll.product}
            </Text>
            <Field label={t('split.roll')} value={roll.barcode} mono />
            <Field label={t('split.remaining')} value={`${roll.remaining} m`} />
          </Card>
        ) : (
          <Text variant="callout" tone="muted" className="px-4">
            {t('split.intro')}
          </Text>
        )}
        {roll ? (
          <View className="gap-2">
            <Input
              label={t('split.meters')}
              value={meters}
              onChangeText={setMeters}
              keyboardType="decimal-pad"
              placeholder="0.00"
              error={error ?? undefined}
            />
            <Button size="lg" title={t('split.cut')} loading={busy} disabled={!meters.trim()} onPress={() => void cut()} />
          </View>
        ) : null}
        {result ? (
          <Card>
            <View className="flex-row items-center gap-2 pb-1">
              <Icon name="check" tone="primary" size={18} />
              <Text variant="headline" className="flex-1">
                {t('split.done', { meters: result.meters })}
              </Text>
            </View>
            <Field label={t('split.new_unit')} value={result.new_unit.barcode} mono />
            <Field label={t('split.new_meters')} value={`${result.new_unit.remaining_meters} m`} />
            <Field label={t('split.left_on_roll')} value={`${result.source.remaining_meters} m`} />
            {result.source.remnant ? <StatusPill tone="progress" label={t('split.remnant')} /> : null}
            {result.source.remnant ? (
              <Text variant="footnote" tone="muted">
                {t('split.remnant_hint')}
              </Text>
            ) : null}
            <Button
              variant="secondary"
              title={t('split.print')}
              icon={<Icon name="share" tone="primary" />}
              loading={labels.busy}
              onPress={() =>
                labels.open({
                  path: '/v1/stock/labels/units.pdf',
                  query: { barcode: [result.new_unit.barcode, result.source.barcode] },
                  name: `split-${result.new_unit.barcode}`,
                  // Split units are already in stock: nothing to mark printed.
                  barcodes: [],
                })
              }
            />
          </Card>
        ) : null}
      </Screen>
      <ContinuousScanner
        visible={scanning}
        single
        onClose={() => setScanning(false)}
        title={t('split.scan_roll')}
        hint={t('split.scan_hint')}
        onCode={onCode}
      />
      {labels.sheet}
    </>
  );
}
