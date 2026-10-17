import { useMutation, useQuery } from '@tanstack/react-query';
import { CameraView, useCameraPermissions } from 'expo-camera';
import * as Haptics from 'expo-haptics';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, FlatList, Pressable, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { Button } from '@/ui/button';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import { type ServiceStockUnit, useDebounced } from './lib';
import { useServiceWrite } from './service-detail-screen';
import { Chips, errorText, ListBody, SearchField } from './ui';

type Kind = 'full' | 'partial';

/**
 * Adds a stock unit to the service: search the organization's stock (name,
 * SKU, barcode) or scan the barcode, then pick quantity / meters and parts.
 */
export function AddItemScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t } = useTranslation('dealer');
  const onWrite = useServiceWrite(uuid);
  const [q, setQ] = useState('');
  const [scanning, setScanning] = useState(false);
  const [picked, setPicked] = useState<ServiceStockUnit | null>(null);
  const query = useDebounced(q.trim());

  const units = useQuery({
    queryKey: ['services', 'stock-units', uuid, query],
    queryFn: () =>
      unwrap(
        api.GET('/v1/services/{uuid}/stock-units', {
          params: { path: { uuid }, query: { q: query || undefined, limit: 50 } },
        }),
      ),
  });

  const add = useMutation({
    mutationFn: (body: {
      barcode: string;
      kind: Kind;
      quantity?: number;
      meters?: number;
      applied_parts?: string[];
      notes?: string;
    }) => unwrap(api.POST('/v1/services/{uuid}/items', { params: { path: { uuid } }, body })),
    onSuccess: (s) => {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      onWrite(s);
      router.back();
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });

  const items = units.data?.items ?? [];

  return (
    <>
      <Stack.Screen options={{ title: t('item_add'), headerLargeTitle: false }} />
      {picked ? (
        <UnitForm unit={picked} busy={add.isPending} onCancel={() => setPicked(null)} onSubmit={(b) => add.mutate(b)} />
      ) : (
        <FlatList
          data={items}
          keyExtractor={(u) => u.uuid}
          contentInsetAdjustmentBehavior="automatic"
          keyboardShouldPersistTaps="handled"
          keyboardDismissMode="on-drag"
          className="flex-1 bg-background"
          contentContainerClassName="gap-2 px-4 pb-10"
          ListHeaderComponent={
            <View className="gap-3 pb-1 pt-2">
              <View className="flex-row items-center gap-2">
                <View className="flex-1">
                  <SearchField value={q} onChange={setQ} placeholder={t('item_search')} />
                </View>
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('scan_barcode')}
                  onPress={() => setScanning((v) => !v)}
                  className={cn('size-11 items-center justify-center rounded-lg', scanning ? 'bg-primary' : 'bg-secondary')}>
                  <Icon name="camera" tone={scanning ? 'primary-foreground' : 'primary'} size={20} />
                </Pressable>
              </View>
              {scanning ? (
                <BarcodeScanner
                  onCode={(code) => {
                    setScanning(false);
                    setQ(code);
                  }}
                />
              ) : null}
            </View>
          }
          ListEmptyComponent={
            <ListBody
              isPending={units.isPending}
              error={units.error}
              count={items.length}
              onRetry={() => void units.refetch()}
              empty={{ icon: 'packageIn', title: t('stock_empty'), body: t('stock_empty_body') }}
            />
          }
          renderItem={({ item: u }) => (
            <Pressable
              accessibilityRole="button"
              onPress={() => setPicked(u)}
              className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
              <Icon name="barcode" tone="muted-foreground" size={20} />
              <View className="flex-1 gap-0.5">
                <Text variant="body" numberOfLines={2}>
                  {u.product.name}
                </Text>
                <Text variant="footnote" tone="muted" numberOfLines={1}>
                  {[
                    u.barcode,
                    u.product.sku,
                    u.unit_kind === 'fixed'
                      ? t('on_hand', { count: u.quantity_on_hand })
                      : u.remaining_meters
                        ? t('meters_left', { value: u.remaining_meters })
                        : null,
                  ]
                    .filter(Boolean)
                    .join(' · ')}
                </Text>
              </View>
            </Pressable>
          )}
        />
      )}
    </>
  );
}

const SCAN_TYPES = ['qr', 'datamatrix', 'code128', 'code39', 'ean13', 'ean8'] as const;

function BarcodeScanner({ onCode }: { onCode: (code: string) => void }) {
  const { t } = useTranslation('dealer');
  const [permission, request] = useCameraPermissions();
  const done = useRef(false);
  if (!permission) return null;
  if (!permission.granted) {
    return <Button title={t('camera_allow')} variant="secondary" onPress={() => void request()} />;
  }
  return (
    <View className="h-56 overflow-hidden rounded-lg bg-black">
      <CameraView
        style={{ flex: 1 }}
        barcodeScannerSettings={{ barcodeTypes: [...SCAN_TYPES] }}
        onBarcodeScanned={({ data }) => {
          if (done.current) return;
          done.current = true;
          void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
          onCode(data.trim());
        }}
      />
    </View>
  );
}

function UnitForm({
  unit,
  busy,
  onCancel,
  onSubmit,
}: {
  unit: ServiceStockUnit;
  busy: boolean;
  onCancel: () => void;
  onSubmit: (b: { barcode: string; kind: Kind; quantity?: number; meters?: number; applied_parts?: string[]; notes?: string }) => void;
}) {
  const { t } = useTranslation('dealer');
  const fixed = unit.unit_kind === 'fixed';
  const roll = !fixed && unit.remaining_meters !== null;
  const [kind, setKind] = useState<Kind>('full');
  const [quantity, setQuantity] = useState('1');
  const [meters, setMeters] = useState('');
  const [notes, setNotes] = useState('');
  const [parts, setParts] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);

  const submit = () => {
    setError(null);
    const body: Parameters<typeof onSubmit>[0] = { barcode: unit.barcode, kind: fixed ? 'full' : kind };
    if (fixed) {
      const n = Number.parseInt(quantity, 10);
      if (!Number.isFinite(n) || n < 1 || n > unit.quantity_on_hand) return setError(t('quantity_invalid'));
      body.quantity = n;
    } else if (kind === 'partial') {
      const m = Number.parseFloat(meters.replace(',', '.'));
      const max = unit.remaining_meters ? Number.parseFloat(unit.remaining_meters) : Infinity;
      if (!Number.isFinite(m) || m <= 0 || m > max) return setError(t('meters_invalid'));
      body.meters = Math.round(m * 100) / 100;
    }
    if (parts.length) body.applied_parts = parts;
    if (notes.trim()) body.notes = notes.trim();
    onSubmit(body);
  };

  return (
    <ScrollView
      contentInsetAdjustmentBehavior="automatic"
      keyboardShouldPersistTaps="handled"
      className="flex-1 bg-background"
      contentContainerClassName="gap-4 px-4 pb-12 pt-2">
      <View className="gap-1 rounded-lg border border-border bg-card p-4">
        <Text variant="headline">{unit.product.name}</Text>
        <Text variant="footnote" tone="muted">
          {unit.barcode} · {unit.product.sku}
        </Text>
      </View>

      {fixed ? (
        <Input
          label={t('quantity')}
          hint={t('on_hand', { count: unit.quantity_on_hand })}
          keyboardType="number-pad"
          value={quantity}
          onChangeText={setQuantity}
        />
      ) : roll ? (
        <View className="gap-3">
          <Chips
            options={[
              { value: 'full' as Kind, label: t('kind_full') },
              { value: 'partial' as Kind, label: t('kind_partial') },
            ]}
            value={kind}
            onChange={setKind}
          />
          {kind === 'partial' ? (
            <Input
              label={t('meters')}
              hint={t('meters_left', { value: unit.remaining_meters ?? '' })}
              keyboardType="decimal-pad"
              value={meters}
              onChangeText={setMeters}
            />
          ) : null}
        </View>
      ) : null}

      {unit.product.available_parts.length > 0 ? (
        <View className="gap-2">
          <Text variant="footnote" tone="muted" className="font-medium">
            {t('applied_parts')}
          </Text>
          <View className="flex-row flex-wrap gap-2">
            {unit.product.available_parts.map((p) => {
              const on = parts.includes(p);
              return (
                <Pressable
                  key={p}
                  accessibilityRole="checkbox"
                  accessibilityState={{ checked: on }}
                  onPress={() => setParts((cur) => (on ? cur.filter((x) => x !== p) : [...cur, p]))}
                  className={cn(
                    'min-h-9 justify-center rounded-full border px-3',
                    on ? 'border-primary bg-primary' : 'border-border bg-card',
                  )}>
                  <Text variant="footnote" className={cn(on && 'text-primary-foreground')}>
                    {t(`part.${p}`, { defaultValue: p.replace(/_/g, ' ') })}
                  </Text>
                </Pressable>
              );
            })}
          </View>
        </View>
      ) : null}

      <Input label={t('notes')} value={notes} onChangeText={setNotes} multiline className="min-h-20 py-3" />

      {error ? (
        <Text variant="footnote" tone="danger">
          {error}
        </Text>
      ) : null}

      <Button title={t('item_add')} size="lg" loading={busy} onPress={submit} />
      <Button title={t('common:back')} variant="ghost" onPress={onCancel} disabled={busy} />
    </ScrollView>
  );
}
