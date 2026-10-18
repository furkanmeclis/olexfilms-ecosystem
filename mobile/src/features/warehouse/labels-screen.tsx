import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { CardRow, PagedList, PAGE_SIZE, useFormat, usePagedQuery } from '@/features/admin/kit';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { Input } from '@/ui/input';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { resolveScan, useRooms } from './api';
import { useCurrentWarehouse } from './context';
import { ContinuousScanner, type ScanOutcome } from './continuous-scanner';
import { alertError, whError } from './format';
import { IconButton, Segmented } from './parts';
import { canSharePdf, sharePanelPdf } from './pdf-share';
import { WarehouseField } from './warehouse-picker';

type Batch = components['schemas']['BarcodeBatch'];
type Mode = 'locations' | 'units' | 'batches';

/** Server limit of one label sheet. */
const MAX_UNITS = 1000;

/** Runs one PDF share at a time and reports failures. */
function usePdfShare() {
  const [busy, setBusy] = useState<string | null>(null);
  const run = async (key: string, fn: () => Promise<void>) => {
    if (busy) return;
    setBusy(key);
    try {
      await fn();
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(null);
    }
  };
  return { busy, run };
}

function LocationLabels() {
  const { t } = useTranslation('warehouse');
  const { warehouse } = useCurrentWarehouse();
  const [warehouseUuid, setWarehouseUuid] = useState<string | undefined>(undefined);
  const selected = warehouseUuid ?? warehouse?.uuid;
  const rooms = useRooms(selected);
  const pdf = usePdfShare();

  const share = (room?: { uuid: string; code: string }) =>
    void pdf.run(room?.uuid ?? 'all', () =>
      sharePanelPdf(
        '/v1/warehouse/labels/locations.pdf',
        room ? { room: room.uuid } : { warehouse: selected },
        `locations-${room?.code ?? 'all'}`,
      ),
    );

  return (
    <View className="gap-2.5">
      <Text variant="callout" tone="muted">
        {t('labels.locations_description')}
      </Text>
      <WarehouseField label={t('fields.warehouse')} value={selected} onChange={(w) => setWarehouseUuid(w.uuid)} />
      {selected ? (
        <>
          <Button
            title={t('labels.print_warehouse')}
            icon="qr"
            size="lg"
            loading={pdf.busy === 'all'}
            disabled={!!pdf.busy}
            onPress={() => share()}
          />
          {rooms.data?.length ? (
            <>
              <SectionHeader title={t('labels.by_room')} />
              <List>
                {rooms.data.map((r, i) => (
                  <ListRow
                    key={r.uuid}
                    icon="location"
                    title={r.name}
                    subtitle={r.code}
                    trailing={
                      <IconButton icon="share" label={t('labels.print')} busy={pdf.busy === r.uuid} onPress={() => share(r)} />
                    }
                    last={i === rooms.data.length - 1}
                  />
                ))}
              </List>
            </>
          ) : null}
        </>
      ) : null}
    </View>
  );
}

function UnitLabels() {
  const { t } = useTranslation('warehouse');
  const [codes, setCodes] = useState<string[]>([]);
  const [manual, setManual] = useState('');
  const [scanning, setScanning] = useState(false);
  const pdf = usePdfShare();

  const add = (code: string) => setCodes((c) => (c.includes(code) || c.length >= MAX_UNITS ? c : [...c, code]));

  const onScan = async (code: string): Promise<ScanOutcome> => {
    try {
      const res = await resolveScan(code, { expect: 'unit' });
      if (res.status !== 'ok' || !res.unit) return { ok: false, title: t(`scan_status.${res.status}`, { defaultValue: t('scan.failed') }) };
      add(res.unit.barcode);
      return { ok: true, title: res.unit.barcode, detail: res.product?.name };
    } catch (e) {
      return { ok: false, title: whError(e) };
    }
  };

  return (
    <View className="gap-2.5">
      <Text variant="callout" tone="muted">
        {t('labels.units_description')}
      </Text>
      <Button title={t('labels.scan_units')} icon="scan" variant="secondary" onPress={() => setScanning(true)} />
      <Input
        label={t('reclass.barcode')}
        icon="barcode"
        value={manual}
        onChangeText={setManual}
        placeholder={t('reclass.barcode_placeholder')}
        autoCapitalize="characters"
        autoCorrect={false}
        returnKeyType="done"
        onSubmitEditing={() => {
          if (manual.trim()) add(manual.trim());
          setManual('');
        }}
      />
      {codes.length ? (
        <>
          <SectionHeader title={t('labels.selected', { count: codes.length })} />
          <List>
            {codes.map((c, i) => (
              <ListRow
                key={c}
                icon="barcode"
                title={c}
                trailing={
                  <IconButton icon="close" label={t('labels.remove')} onPress={() => setCodes((x) => x.filter((y) => y !== c))} />
                }
                last={i === codes.length - 1}
              />
            ))}
          </List>
          <Button
            title={t('labels.print_units', { count: codes.length })}
            icon="share"
            size="lg"
            loading={pdf.busy === 'units'}
            onPress={() =>
              void pdf.run('units', () => sharePanelPdf('/v1/stock/labels/units.pdf', { barcode: codes }, `units-${codes.length}`))
            }
          />
          <Button title={t('labels.clear')} variant="ghost" onPress={() => setCodes([])} />
        </>
      ) : null}
      <ContinuousScanner
        visible={scanning}
        onClose={() => setScanning(false)}
        title={t('labels.scan_units')}
        hint={t('labels.scan_units_hint')}
        onCode={onScan}
      />
    </View>
  );
}

function BatchLabels({ header }: { header: React.ReactElement }) {
  const { t } = useTranslation('warehouse');
  const f = useFormat();
  const pdf = usePdfShare();
  const list = usePagedQuery<Batch>(['warehouse', 'label-batches'], (offset) =>
    unwrap(api.GET('/v1/stock/barcodes', { params: { query: { limit: PAGE_SIZE, offset } } })),
  );
  return (
    <PagedList
      query={list}
      keyOf={(b) => b.uuid}
      header={header}
      empty={{ icon: 'barcode', title: t('labels.batches_empty'), body: list.isError ? whError(list.error) : undefined }}
      renderItem={({ item }) => (
        <CardRow
          title={`${item.first_barcode} – ${item.last_barcode}`}
          subtitle={`${item.product.name} · ${t('labels.batch_quantity', { count: item.quantity })}`}
          meta={
            item.last_printed_at
              ? t('labels.printed', { count: item.print_count, date: f.date(item.last_printed_at) })
              : t('labels.never_printed')
          }
          trailing={
            <IconButton
              icon="share"
              label={t('labels.print')}
              busy={pdf.busy === item.uuid}
              tone="primary"
              onPress={() =>
                void pdf.run(item.uuid, () =>
                  sharePanelPdf(`/v1/stock/barcodes/${item.uuid}/labels.pdf`, {}, `batch-${item.first_barcode}`),
                )
              }
            />
          }
        />
      )}
    />
  );
}

/**
 * Label sheets (location QR, unit barcodes, reserved batches) rendered by the
 * server and handed to the system share sheet, where AirPrint, Files and
 * other apps take them. No printer SDK on the device.
 */
export function LabelsScreen() {
  const { t } = useTranslation('warehouse');
  const [mode, setMode] = useState<Mode>('locations');

  const switcher = (
    <View className={mode === 'batches' ? 'mx-4 mb-1' : undefined}>
      <Segmented
        value={mode}
        onChange={setMode}
        options={(['locations', 'units', 'batches'] as Mode[]).map((m) => ({ value: m, label: t(`labels.mode.${m}`) }))}
      />
    </View>
  );

  return (
    <>
      <Stack.Screen options={{ title: t('labels.title'), headerLargeTitle: false }} />
      {!canSharePdf ? (
        <Screen>
          <EmptyState icon="info" title={t('labels.ios_only_title')} body={t('labels.ios_only_body')} />
        </Screen>
      ) : mode === 'batches' ? (
        <BatchLabels header={switcher} />
      ) : (
        <Screen keyboardShouldPersistTaps="handled">
          {switcher}
          {mode === 'locations' ? <LocationLabels /> : <UnitLabels />}
        </Screen>
      )}
    </>
  );
}
