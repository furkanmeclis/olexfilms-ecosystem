import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import type { paths } from '@/core/api/schema';
import { APP_VERSION, MOBILE_API_VERSION } from '@/core/config';
import { errorMessage } from '@/core/i18n';
import { usePreferences } from '@/core/preferences';
import { useCan } from '@/core/spaces';
import { formatThickness, toMicrons } from '@/core/units';
import {
  CardRow,
  ErrorState,
  InfoRow,
  ListSkeleton,
  PagedList,
  PAGE_SIZE,
  useChildHref,
  useFormat,
  usePagedQuery,
} from '@/features/admin/kit';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { List, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { GaugePanel } from '@/features/ble-gauge/gauge-panel';
import { isGaugeAvailable } from '@/features/ble-gauge/manager';
import { appendValue, formatInputValue, substrateOf } from '@/features/ble-gauge/readings';
import { useGauge } from '@/features/ble-gauge/store';
import { useGaugeSession } from '@/features/ble-gauge/use-gauge-session';

import { PARTS, parseValues, PLATE_RECOGNITION_ENABLED } from './parts';
import type { IconName } from '@/ui/icons';

type Measurement =
  paths['/v1/measurements']['get']['responses'][200]['content']['application/json']['data']['items'][number];

/** Placeholder card for a capability that needs the development build. */
function SoonCard({ icon, title, body }: { icon: IconName; title: string; body: string }) {
  const { t } = useTranslation('measure');
  return (
    <View className="mx-4 flex-row items-center gap-3 rounded-lg border border-dashed border-border bg-card px-4 py-3 opacity-80">
      <View className="size-9 items-center justify-center rounded-md bg-secondary">
        <Icon name={icon} tone="muted-foreground" size={20} />
      </View>
      <View className="flex-1 gap-0.5">
        <Text variant="callout" className="font-semibold">
          {title}
        </Text>
        <Text variant="footnote" tone="muted">
          {body}
        </Text>
      </View>
      <Text variant="caption" tone="muted">
        {t('soon')}
      </Text>
    </View>
  );
}

function NewButton() {
  const { t } = useTranslation('measure');
  const href = useChildHref();
  return (
    <Pressable accessibilityRole="button" accessibilityLabel={t('new')} hitSlop={8} onPress={() => router.push(href('new'))}>
      <View className="size-8 items-center justify-center">
        <Icon name="add" tone="primary" />
      </View>
    </Pressable>
  );
}

/** Measuring tab: recent measurements of the organization, manual entry, device placeholders. */
export function MeasureScreen() {
  const { t } = useTranslation('measure');
  const f = useFormat();
  const href = useChildHref();
  const canRead = useCan('measurements.read');
  const canWrite = useCan('measurements.write');

  const list = usePagedQuery<Measurement>(
    ['measurements', 'list'],
    (offset) => unwrap(api.GET('/v1/measurements', { params: { query: { limit: PAGE_SIZE, offset } } })),
    canRead,
  );

  const header = (
    <View className="gap-2.5 pb-2 pt-2">
      {!isGaugeAvailable() ? <SoonCard icon="bluetooth" title={t('ble_title')} body={t('ble_body')} /> : null}
      {!PLATE_RECOGNITION_ENABLED ? <SoonCard icon="camera" title={t('plate_title')} body={t('plate_body')} /> : null}
      {canWrite ? (
        <View className="px-4">
          <Button title={t('manual_entry')} icon={<Icon name="add" tone="primary-foreground" size={18} />} onPress={() => router.push(href('new'))} />
        </View>
      ) : null}
      {canRead ? <SectionHeader title={t('recent')} /> : null}
    </View>
  );

  return (
    <>
      <Stack.Screen options={{ title: t('title'), headerRight: canWrite ? () => <NewButton /> : undefined }} />
      {canRead ? (
        <PagedList
          query={list}
          keyOf={(m) => m.uuid}
          header={header}
          empty={{ icon: 'gauge', title: t('empty'), body: canWrite ? t('empty_body') : undefined }}
          renderItem={({ item }) => (
            <CardRow
              title={item.plate || item.vin || t('no_vehicle')}
              subtitle={[item.plate && item.vin ? item.vin : null, item.service ? t('service_no', { no: item.service.service_no }) : null]
                .filter(Boolean)
                .join(' · ') || undefined}
              meta={`${f.dateTime(item.measured_at ?? item.created_at)}${item.device_serial ? ` · ${item.device_serial}` : ''}`}
              trailing={
                <StatusPill label={t(`status.${item.status}`)} tone={item.status === 'accepted' ? 'done' : 'progress'} />
              }
              onPress={() => router.push(href(item.uuid))}
            />
          )}
        />
      ) : (
        <Screen className="px-0">{header}</Screen>
      )}
    </>
  );
}

type Reading = { part: string; values: number[] };

function newClientId() {
  return `m-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

/** Readings grouped by part; falls back to the uploaded parts when the record is not normalized yet. */
function readings(data: {
  values: { part_type: string; value_um: string | null }[];
  raw: Record<string, unknown>;
}): Reading[] {
  const groups = new Map<string, number[]>();
  for (const v of data.values) {
    if (v.value_um === null) continue;
    const list = groups.get(v.part_type) ?? [];
    list.push(Number(v.value_um));
    groups.set(v.part_type, list);
  }
  if (groups.size === 0 && Array.isArray(data.raw.parts)) {
    for (const p of data.raw.parts as { part?: string; values?: number[] }[]) {
      if (p.part && Array.isArray(p.values)) groups.set(p.part, p.values.map(Number));
    }
  }
  return [...groups.entries()].map(([part, values]) => ({ part, values }));
}

export function MeasurementDetailScreen() {
  const { t } = useTranslation('measure');
  const f = useFormat();
  const unit = usePreferences((s) => s.unit);
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const q = useQuery({
    queryKey: ['measurements', 'detail', uuid],
    queryFn: () => unwrap(api.GET('/v1/measurements/{uuid}', { params: { path: { uuid: uuid! } } })),
    enabled: !!uuid,
  });
  const m = q.data;
  const rows = m ? readings(m) : [];
  const th = (um: number) => formatThickness(um, unit, f.locale);

  return (
    <>
      <Stack.Screen options={{ title: m?.plate || m?.vin || t('title'), headerLargeTitle: false }} />
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {q.isPending ? (
          <ListSkeleton rows={4} inset={false} />
        ) : !m ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : (
          <>
            <List>
              <InfoRow label={t('plate')} value={m.plate} />
              <InfoRow label={t('vin')} value={m.vin} />
              <InfoRow label={t('status_label')} value={t(`status.${m.status}`)} />
              <InfoRow label={t('measured_at')} value={f.dateTime(m.measured_at ?? m.created_at)} />
              <InfoRow label={t('device')} value={m.device?.label || m.device?.model || m.device_serial} />
              <InfoRow label={t('service')} value={m.service?.service_no} last />
            </List>

            <SectionHeader title={t('readings')} />
            {rows.length ? (
              <List>
                {rows.map((r, i) => {
                  const min = Math.min(...r.values);
                  const max = Math.max(...r.values);
                  return (
                    <View key={r.part} className={i < rows.length - 1 ? 'gap-0.5 border-b border-border px-4 py-3' : 'gap-0.5 px-4 py-3'}>
                      <View className="flex-row justify-between gap-3">
                        <Text variant="body">{t(`parts.${r.part}`, { defaultValue: r.part })}</Text>
                        <Text variant="body" className="font-semibold tabular-nums">
                          {min === max ? th(min) : `${th(min)} – ${th(max)}`}
                        </Text>
                      </View>
                      <Text variant="footnote" tone="muted">
                        {t('reading_count', { count: r.values.length })}
                      </Text>
                    </View>
                  );
                })}
              </List>
            ) : (
              <Text variant="callout" tone="muted" className="px-4">
                {t('no_readings')}
              </Text>
            )}
          </>
        )}
      </Screen>
    </>
  );
}

/**
 * Manual entry: values per body part in the user's unit, sent in microns to
 * POST /v1/mobile/measurements. The raw body uses the flat reading shape the
 * backend parser reads (place_id / part_type / value).
 */
export function NewMeasurementScreen() {
  const { t, i18n } = useTranslation('measure');
  // Example readings in the user's number format (the parser accepts both , and .).
  const sample = (n: number[]) => n.map((x) => new Intl.NumberFormat(i18n.language).format(x)).join(' ');
  const qc = useQueryClient();
  const unit = usePreferences((s) => s.unit);
  const [plate, setPlate] = useState('');
  const [vin, setVin] = useState('');
  const [values, setValues] = useState<Record<string, string>>({});
  // One key per form: a retried tap never stores the measurement twice.
  const [clientId] = useState(newClientId);

  // SmartPTG: a steady reading is appended to the selected part, like typing it.
  const gaugeOn = isGaugeAvailable();
  const gaugeConnected = useGauge((s) => s.status === 'connected');
  const [activePart, setActivePart] = useState<string | null>(null);
  const [substrates, setSubstrates] = useState<Record<string, 'Fe' | 'Al'>>({});
  const [gaugeDevice, setGaugeDevice] = useState<{ serial: string; model?: string } | null>(null);
  const decimal = new Intl.NumberFormat(i18n.language).format(1.5).replace(/\d/g, '') || '.';
  useGaugeSession((c) => {
    const part = activePart ?? PARTS[0]!.key;
    if (!activePart) setActivePart(part);
    setValues((v) => ({ ...v, [part]: appendValue(v[part] ?? '', formatInputValue(c.um, unit, decimal)) }));
    const substrate = substrateOf(c.material);
    if (substrate) setSubstrates((m) => ({ ...m, [part]: substrate }));
    const g = useGauge.getState();
    if (g.serial != null) setGaugeDevice({ serial: String(g.serial), model: g.model ?? undefined });
  });

  const parts = PARTS.map((p) => ({ ...p, values: parseValues(values[p.key] ?? '').map((v) => toMicrons(v, unit)) })).filter(
    (p) => p.values.length > 0,
  );
  const vinClean = vin.trim().toUpperCase();
  const vinError = vinClean && !/^[A-Z0-9]{11,17}$/.test(vinClean) ? t('vin_invalid') : undefined;

  const save = useMutation({
    mutationFn: () => {
      const now = new Date().toISOString();
      return unwrap(
        api.POST('/v1/mobile/measurements', {
          params: { header: { 'X-Mobile-Api-Version': MOBILE_API_VERSION, 'Idempotency-Key': clientId } },
          body: {
            client_measurement_id: clientId,
            plate: plate.trim() || undefined,
            vin: vinClean || undefined,
            measured_at: now,
            unit: 'um',
            device: gaugeDevice ?? undefined,
            parts: parts.map((p) => ({ part: p.key, place: p.place, values: p.values })),
            raw: {
              source: gaugeDevice ? 'smartptg' : 'manual',
              app_version: APP_VERSION,
              date: now,
              measurements: parts.flatMap((p) =>
                p.values.map((value, position) => ({
                  place_id: p.place,
                  part_type: p.key,
                  value,
                  position,
                  timestamp: now,
                  substrate_type: substrates[p.key],
                })),
              ),
            },
          },
        }),
      );
    },
    onSuccess: (res) => {
      void qc.invalidateQueries({ queryKey: ['measurements', 'list'] });
      Alert.alert(t(res.status === 'accepted' ? 'saved' : 'saved_vin_pending'));
      router.back();
    },
    onError: (e) => Alert.alert(errorMessage(isApiError(e) ? e.code : undefined)),
  });

  const unitLabel = unit === 'mil' ? 'mil' : 'µm';

  return (
    <>
      <Stack.Screen options={{ title: t('new'), headerLargeTitle: false }} />
      <Screen>
        <Input label={t('plate')} placeholder={t('placeholder_plate', { ns: 'common' })} value={plate} onChangeText={setPlate} autoCapitalize="characters" autoCorrect={false} />
        <Input
          label={t('vin')}
          placeholder={t('placeholder_vin', { ns: 'common' })}
          value={vin}
          onChangeText={setVin}
          autoCapitalize="characters"
          autoCorrect={false}
          maxLength={17}
          error={vinError}
          hint={t('vin_hint')}
        />

        {gaugeOn ? (
          <GaugePanel targetLabel={activePart ? t(`parts.${activePart}`) : gaugeConnected ? t(`parts.${PARTS[0]!.key}`) : null} />
        ) : null}

        <SectionHeader title={t('values_in', { unit: unitLabel })} />
        <Text variant="footnote" tone="muted" className="-mt-2">
          {t('values_hint')}
        </Text>
        {PARTS.map((p) => (
          <Input
            key={p.key}
            label={t(`parts.${p.key}`)}
            value={values[p.key] ?? ''}
            onChangeText={(text) => setValues((v) => ({ ...v, [p.key]: text }))}
            onFocus={() => setActivePart(p.key)}
            // With the gauge connected a tap only selects the part; the probe types.
            showSoftInputOnFocus={!gaugeConnected}
            keyboardType="numbers-and-punctuation"
            placeholder={unit === 'mil' ? sample([4.5, 4.8]) : sample([115, 120])}
          />
        ))}

        <Button
          size="lg"
          title={t('save')}
          loading={save.isPending}
          disabled={parts.length === 0 || !!vinError}
          onPress={() => save.mutate()}
        />
      </Screen>
    </>
  );
}
