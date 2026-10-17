import { useQueryClient } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { countApi, resolveScan, type StockCountInput, useRooms, wk } from './api';
import { useCurrentWarehouse } from './context';
import { ContinuousScanner, type ScanOutcome } from './continuous-scanner';
import { alertError, replace, whError } from './format';
import { Chips } from './parts';
import { WarehouseField } from './warehouse-picker';

type Method = StockCountInput['method'];
type Visibility = StockCountInput['visibility'];
type Scope = StockCountInput['scope_type'];

const METHODS: Method[] = ['location_first', 'unit_first', 'product_qty', 'initial_placement'];
const SCOPES: Scope[] = ['warehouse', 'room', 'location', 'product'];

/** Opens a stock count; the location or product scope is picked by scanning its label. */
export function CountNewScreen() {
  const { t } = useTranslation('warehouse');
  const qc = useQueryClient();
  const { warehouse } = useCurrentWarehouse();
  const [wh, setWh] = useState<string | undefined>(undefined);
  const [method, setMethod] = useState<Method>('location_first');
  const [visibility, setVisibility] = useState<Visibility>('blind');
  const [scope, setScope] = useState<Scope>('warehouse');
  const [room, setRoom] = useState<string>('');
  const [location, setLocation] = useState<{ uuid: string; code: string } | null>(null);
  const [product, setProduct] = useState<{ uuid: string; name: string } | null>(null);
  const [note, setNote] = useState('');
  const [scanning, setScanning] = useState(false);
  const [busy, setBusy] = useState(false);
  const [tried, setTried] = useState(false);

  const whId = wh ?? warehouse?.uuid;
  const rooms = useRooms(scope === 'room' ? whId : undefined);
  const scopes = method === 'initial_placement' ? SCOPES.filter((s) => s !== 'product') : SCOPES;
  const effectiveScope = scopes.includes(scope) ? scope : 'warehouse';

  const missing =
    !whId
      ? t('validation.warehouse')
      : effectiveScope === 'room' && !room
        ? t('validation.room')
        : effectiveScope === 'location' && !location
          ? t('validation.location')
          : effectiveScope === 'product' && !product
            ? t('validation.product')
            : null;

  const onScan = async (code: string): Promise<ScanOutcome> => {
    try {
      const r = await resolveScan(code, {
        expect: effectiveScope === 'location' ? 'location' : 'any',
        warehouse: effectiveScope === 'location' ? whId : undefined,
      });
      if (r.status !== 'ok') {
        return { ok: false, title: t(`scan_status.${r.status}`, { warehouse: '', actual: t(`scan_status.type.${r.type || 'any'}`), expected: t('scan_status.type.location') }) };
      }
      if (effectiveScope === 'location') {
        if (!r.location) return { ok: false, title: t('order_place.not_location') };
        setLocation({ uuid: r.location.uuid, code: r.location.full_code });
        return { ok: true, title: r.location.full_code };
      }
      if (!r.product) return { ok: false, title: t('validation.product') };
      setProduct({ uuid: r.product.uuid, name: r.product.name });
      return { ok: true, title: r.product.name };
    } catch (e) {
      return { ok: false, title: whError(e) };
    }
  };

  const submit = async () => {
    setTried(true);
    if (missing || !whId) return;
    setBusy(true);
    try {
      const x = await countApi.create({
        warehouse_uuid: whId,
        method,
        visibility,
        scope_type: effectiveScope,
        room_uuid: effectiveScope === 'room' ? room : undefined,
        location_uuid: effectiveScope === 'location' ? location?.uuid : undefined,
        product_uuid: effectiveScope === 'product' ? product?.uuid : undefined,
        note: note.trim() || null,
      });
      qc.setQueryData(wk.count(x.uuid), x);
      void qc.invalidateQueries({ queryKey: ['warehouse', 'counts'] });
      void qc.invalidateQueries({ queryKey: wk.summary() });
      replace(`/depo/jobs/count/${x.uuid}`);
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Stack.Screen options={{ title: t('counts.new_title'), headerLargeTitle: false }} />
      <Screen>
        <WarehouseField
          label={t('fields.warehouse')}
          value={whId}
          onChange={(w) => {
            setWh(w.uuid);
            setRoom('');
            setLocation(null);
          }}
        />

        <View className="gap-2">
          <Text variant="footnote" tone="muted" className="font-medium">
            {t('fields.method')}
          </Text>
          <Chips value={method} onChange={setMethod} options={METHODS.map((m) => ({ value: m, label: t(`count_method.${m}`) }))} />
          <Text variant="footnote" tone="muted">
            {t(`count_method_hint.${method}`)}
          </Text>
        </View>

        <View className="gap-2">
          <Text variant="footnote" tone="muted" className="font-medium">
            {t('fields.visibility')}
          </Text>
          <Chips
            value={visibility}
            onChange={setVisibility}
            options={(['blind', 'guided'] as const).map((v) => ({ value: v, label: t(`count_visibility.${v}`) }))}
          />
          <Text variant="footnote" tone="muted">
            {t(`count_visibility_hint.${visibility}`)}
          </Text>
        </View>

        <View className="gap-2">
          <Text variant="footnote" tone="muted" className="font-medium">
            {t('fields.scope')}
          </Text>
          <Chips value={effectiveScope} onChange={setScope} options={scopes.map((s) => ({ value: s, label: t(`count_scope.${s}`) }))} />
        </View>

        {effectiveScope === 'room' ? (
          <View className="gap-2">
            <Text variant="footnote" tone="muted" className="font-medium">
              {t('fields.room')}
            </Text>
            {rooms.isLoading ? (
              <Text variant="footnote" tone="muted">
                {t('list.loading')}
              </Text>
            ) : (rooms.data?.length ?? 0) === 0 ? (
              <Text variant="footnote" tone="muted">
                {t('locations.no_rooms')}
              </Text>
            ) : (
              <Chips value={room} onChange={setRoom} options={(rooms.data ?? []).map((r) => ({ value: r.uuid, label: r.name || r.code }))} />
            )}
            {tried && !room ? (
              <Text variant="footnote" tone="danger">
                {t('validation.room')}
              </Text>
            ) : null}
          </View>
        ) : null}

        {effectiveScope === 'location' || effectiveScope === 'product' ? (
          <View className="gap-2">
            <Text variant="footnote" tone="muted" className="font-medium">
              {effectiveScope === 'location' ? t('fields.location') : t('fields.product')}
            </Text>
            <Button
              variant="outline"
              icon={<Icon name="scan" tone="primary" size={18} />}
              title={
                effectiveScope === 'location'
                  ? (location?.code ?? t('entry.scan_location'))
                  : (product?.name ?? t('m.scan_product'))
              }
              onPress={() => setScanning(true)}
            />
            {tried && missing ? (
              <Text variant="footnote" tone="danger">
                {missing}
              </Text>
            ) : null}
          </View>
        ) : null}

        <Input label={t('fields.note')} value={note} onChangeText={setNote} multiline maxLength={500} className="min-h-20 py-3" />
        <Button size="lg" title={t('counts.create')} loading={busy} onPress={() => void submit()} />
      </Screen>

      <ContinuousScanner
        visible={scanning}
        single
        onClose={() => setScanning(false)}
        title={effectiveScope === 'location' ? t('entry.scan_location') : t('m.scan_product')}
        hint={effectiveScope === 'location' ? t('entry.scan_location_hint') : undefined}
        onCode={onScan}
      />
    </>
  );
}
