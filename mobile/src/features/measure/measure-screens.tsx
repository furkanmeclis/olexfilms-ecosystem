import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack, useFocusEffect, useLocalSearchParams } from 'expo-router';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Image, Pressable, View } from 'react-native';
import { SvgXml } from 'react-native-svg';

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

import { GaugeHeaderBadge } from '@/features/ble-gauge/gauge-badge';
import { GaugePanel } from '@/features/ble-gauge/gauge-panel';
import { autoReconnect, isGaugeAvailable } from '@/features/ble-gauge/manager';
import { appendValue, formatInputValue, substrateOf } from '@/features/ble-gauge/readings';
import { SAPL_COLORS, saplLevel } from '@/features/ble-gauge/interpretation';
import { useGauge } from '@/features/ble-gauge/store';
import { useGaugeButtons, useGaugeSession, useGaugeUserKey } from '@/features/ble-gauge/use-gauge-session';
import { PlateScanButton } from '@/features/plate-scan/plate-scan-button';

import { bodyTypeOf, useBodyTypes } from './body-types';
import { draftReadingCount, useDrafts } from './drafts';
import {
  buildTargets,
  type GuideTarget,
  guideView,
  nextOpen,
  nextPartIndex,
  partProgress,
  type PointReading,
  targetKey,
} from './guided';
import { PARTS, parseValues } from './parts';
import type { PartMapData } from './point-map';
import { MeasurementMap } from './point-map-view';
import type { IconName } from '@/ui/icons';
import { PullRefresh } from '@/ui/pull-refresh';

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

/** Header shortcut to the gauge screen (SmartPTG builds only). */
function GaugeButton() {
  const { t } = useTranslation('measure');
  const href = useChildHref();
  return (
    <Pressable accessibilityRole="button" accessibilityLabel={t('ble.device_title')} hitSlop={8} onPress={() => router.push(href('device'))}>
      <View className="h-8 min-w-8 items-center justify-center">
        <GaugeHeaderBadge />
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

  const userKey = useGaugeUserKey();
  // Opening the list reconnects the last gauge in the background (6 tries, 10 s apart).
  useFocusEffect(
    useCallback(() => {
      autoReconnect(userKey);
    }, [userKey]),
  );
  const loadDrafts = useDrafts((s) => s.load);
  loadDrafts(userKey);
  const drafts = useDrafts((s) => s.drafts);
  const bodyTypes = useBodyTypes();
  const removeDraft = useDrafts((s) => s.remove);
  const confirmRemove = (id: string) =>
    Alert.alert(t('draft_delete_title'), undefined, [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t('draft_delete'), style: 'destructive', onPress: () => removeDraft(id) },
    ]);

  const header = (
    <View className="gap-2.5 pb-2 pt-2">
      {!isGaugeAvailable() ? <SoonCard icon="bluetooth" title={t('ble_title')} body={t('ble_body')} /> : null}
      {canWrite ? (
        <View className="px-4">
          <Button title={t('new')} icon={<Icon name="gauge" tone="primary-foreground" size={18} />} onPress={() => router.push(href('new'))} />
        </View>
      ) : null}
      {canWrite && drafts.length ? (
        <>
          <SectionHeader title={t('drafts')} />
          {drafts.map((d) => {
            const b = bodyTypeOf(bodyTypes.data, d.bodyType);
            return (
              <CardRow
                key={d.id}
                title={d.plate}
                subtitle={[d.vin || t('vin_missing'), b ? t(`body.${b.key}`, { defaultValue: b.name }) : null].filter(Boolean).join(' · ')}
                meta={`${f.dateTime(d.updatedAt)} · ${t('reading_count', { count: draftReadingCount(d) })}`}
                trailing={<StatusPill label={t('draft')} tone="neutral" />}
                onPress={() => router.push(`${href('new')}?draft=${d.id}` as never)}
                onLongPress={() => confirmRemove(d.id)}
              />
            );
          })}
        </>
      ) : null}
      {canRead ? <SectionHeader title={t('recent')} /> : null}
    </View>
  );

  return (
    <>
      <Stack.Screen
        options={{
          title: t('title'),
          headerRight: isGaugeAvailable() ? () => <GaugeButton /> : undefined,
        }}
      />
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

type GaugeRead = {
  part_type: string;
  value_um: number;
  frequency_hz: number;
  channel: 'fe' | 'al';
  material: 'fe' | 'al';
  trigger: 'steady' | 'button';
  fe_offset_hz: number | null;
  al_offset_hz: number | null;
  at: string;
};

/** Gauge block of the raw upload: curve hash, calibration and the raw readings (REVERSE_ENGINEERING §9.2). */
function gaugeRaw(reads: GaugeRead[]) {
  const g = useGauge.getState();
  return {
    serial: g.serial,
    model: g.model,
    device_type: g.deviceType,
    curve_sha256: g.curveSha256,
    calibration: g.calibration
      ? { fe_offset_hz: g.calibration.fe, al_offset_hz: g.calibration.al, target_um: g.calibration.targetUm, calibrated_at: g.calibration.calibratedAt }
      : null,
    readings: reads,
  };
}

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
      <Screen refreshControl={<PullRefresh busy={q.isRefetching} onRefresh={() => void q.refetch()} />}>
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
            {m.values.length ? (
              <MeasurementMap bodyType={m.body_type} values={m.values} />
            ) : rows.length ? (
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
/** Guided readings per part for the request's `parts` (part = NexPTG part type, values in point order). */
function guidedParts(targets: GuideTarget[], points: Record<string, PointReading>) {
  const out: { part: string; place: string; values: number[] }[] = [];
  for (const t of targets) {
    const r = points[targetKey(t)];
    if (!r) continue;
    let row = out.find((x) => x.part === t.part);
    if (!row) {
      row = { part: t.part, place: t.place, values: [] };
      out.push(row);
    }
    row.values.push(r.um);
  }
  return out;
}

/** Flat raw rows with the NexPTG position and SAPL level, so the detail map places every point. */
function guidedRows(targets: GuideTarget[], points: Record<string, PointReading>) {
  return targets.flatMap((t) => {
    const r = points[targetKey(t)];
    if (!r) return [];
    return [
      {
        place_id: t.place,
        part_type: t.part,
        value: r.um,
        position: t.position,
        interpretation: r.level,
        is_inside: false,
        timestamp: r.at,
        substrate_type: substrateOf(r.material),
      },
    ];
  });
}

/** Guided measuring panel: the place view (unzoomed, active part highlighted, next point ringed), progress and controls. */
function GuidePanel({
  map,
  targets,
  target,
  points,
  onPrev,
  onSkipPoint,
  onSkipPart,
  onUndo,
  onJump,
  canUndo,
}: {
  map: PartMapData;
  targets: GuideTarget[];
  target: GuideTarget;
  points: Record<string, PointReading>;
  onPrev: () => void;
  onSkipPoint: () => void;
  onSkipPart: () => void;
  onUndo: () => void;
  onJump: (index: number) => void;
  canUndo: boolean;
}) {
  const { t, i18n } = useTranslation('measure');
  const unit = usePreferences((s) => s.unit);
  const progress = partProgress(targets, points);
  const part = progress.find((p) => p.part === target.part);
  const done = Object.keys(points).length;
  const svg = guideView(map, target, points, {
    highlight: '#7DD3A8',
    ring: '#2563EB',
    levelColor: (l) => SAPL_COLORS[(Math.min(5, Math.max(0, l)) as 0 | 1 | 2 | 3 | 4 | 5)],
  });
  const current = points[targetKey(target)];
  const allDone = done === targets.length;
  return (
    <View className="gap-3">
      <View className="gap-3 rounded-2xl border border-border bg-card px-4 pb-4 pt-3">
        <View className="flex-row items-end justify-between gap-3">
          <View className="flex-1 gap-0.5">
            <Text variant="caption" tone="muted">
              {t(`place.${target.place}`)}
            </Text>
            <Text variant="title" numberOfLines={1}>
              {t(`parts.${target.part}`, { defaultValue: target.part })}
            </Text>
          </View>
          <View className="items-end">
            <Text variant="headline" tone="primary" className="tabular-nums">
              {t('guide.point', { n: target.position, total: part?.total ?? 0 })}
            </Text>
            <Text variant="footnote" tone="muted" className="tabular-nums">
              {t('guide.total', { done, total: targets.length })}
            </Text>
          </View>
        </View>
        {svg ? <SvgXml xml={svg} width="100%" height={undefined} style={{ aspectRatio: 2 }} /> : null}
        <View className="h-1.5 overflow-hidden rounded-full bg-secondary">
          <View className="h-full rounded-full bg-primary" style={{ width: `${targets.length ? (done / targets.length) * 100 : 0}%` }} />
        </View>
        <Text variant="callout" tone={allDone ? 'primary' : current ? 'default' : 'muted'} className="text-center">
          {allDone
            ? t('guide.all_done')
            : current
              ? t('guide.measured', { value: formatThickness(current.um, unit, i18n.language) })
              : t('guide.touch_point', { n: target.position })}
        </Text>
        <View className="flex-row gap-2">
          <Button className="flex-1" variant="secondary" title={t('guide.prev')} onPress={onPrev} />
          <Button className="flex-1" variant="secondary" title={t('guide.skip_point')} onPress={onSkipPoint} />
          <Button className="flex-1" variant="secondary" title={t('guide.skip_part')} onPress={onSkipPart} />
        </View>
        {canUndo ? <Button variant="ghost" title={t('guide.undo')} onPress={onUndo} /> : null}
      </View>

      <SectionHeader title={t('guide.parts')} />
      <View className="flex-row flex-wrap gap-2">
        {progress.map((p) => {
          const on = p.part === target.part;
          const full = p.done === p.total;
          return (
            <Pressable
              key={p.part}
              accessibilityRole="button"
              onPress={() => onJump(p.first)}
              className={`flex-row items-center gap-1.5 rounded-full border px-3 py-1.5 ${on ? 'border-primary bg-primary/10' : full ? 'border-success/40 bg-success/10' : 'border-border bg-card'}`}>
              <Text variant="footnote" tone={on ? 'primary' : 'default'} className="font-medium">
                {t(`parts.${p.part}`, { defaultValue: p.part })}
              </Text>
              <Text variant="footnote" tone="muted" className="tabular-nums">
                {`${p.done}/${p.total}`}
              </Text>
            </Pressable>
          );
        })}
      </View>
    </View>
  );
}

/** One part of the measuring form: tap selects it as the probe's target; values are read-only (gauge only). */
function PartRow({
  label,
  values,
  active,
  onSelect,
  onRemoveLast,
  removeLabel,
}: {
  label: string;
  values: string;
  active: boolean;
  onSelect: () => void;
  onRemoveLast: () => void;
  removeLabel: string;
}) {
  const has = values.trim().length > 0;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ selected: active }}
      onPress={onSelect}
      className={`flex-row items-center gap-3 rounded-lg border px-4 py-3 ${active ? 'border-primary bg-primary/10' : 'border-border bg-card'}`}>
      <View className="flex-1 gap-0.5">
        <Text variant="footnote" tone={active ? 'primary' : 'muted'}>
          {label}
        </Text>
        <Text variant="headline" tone={has ? 'default' : 'muted'} className="tabular-nums" numberOfLines={2}>
          {has ? values : '—'}
        </Text>
      </View>
      {has ? (
        <Pressable accessibilityRole="button" accessibilityLabel={removeLabel} hitSlop={8} onPress={onRemoveLast}>
          <Icon name="undo" tone="muted-foreground" size={20} />
        </Pressable>
      ) : null}
    </Pressable>
  );
}

const VIN_RE = /^[A-Z0-9]{11,17}$/u;

/** Body type grid of the vehicle step (backend list): side view per type, tap to select (again to clear). */
function BodyTypePicker({ value, onChange }: { value: string | null; onChange: (code: string | null) => void }) {
  const { t } = useTranslation('measure');
  const list = useBodyTypes();
  if (list.isPending) return <ListSkeleton rows={2} inset={false} />;
  if (list.isError) return <ErrorState error={list.error} onRetry={() => void list.refetch()} />;
  return (
    <View className="flex-row flex-wrap justify-between gap-y-2.5">
      {list.data.map((b) => {
        const on = value === b.code;
        return (
          <Pressable
            key={b.code}
            accessibilityRole="button"
            accessibilityState={{ selected: on }}
            onPress={() => onChange(on ? null : b.code)}
            className={`w-[48.5%] items-center gap-1 rounded-lg border px-2 pb-2 pt-1.5 ${on ? 'border-primary bg-primary/10' : 'border-border bg-card'}`}>
            <Image source={{ uri: b.thumbnail }} style={{ width: '100%', height: 64 }} resizeMode="contain" />
            <Text variant="footnote" tone={on ? 'primary' : 'default'} className="font-medium" numberOfLines={1}>
              {t(`body.${b.key}`, { defaultValue: b.name })}
            </Text>
            <Text variant="caption" tone="muted">
              {t('guide.points_count', { count: b.points })}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

/**
 * New measurement in two steps (user decision 11.11): 1) vehicle — plate
 * (required), VIN (optional) and body type, can be kept as a draft on the
 * phone; 2) measuring with the gauge. Sending needs the VIN. `?draft=<id>`
 * resumes a draft.
 */
export function NewMeasurementScreen() {
  const { t, i18n } = useTranslation('measure');
  const qc = useQueryClient();
  const unit = usePreferences((s) => s.unit);
  const userKey = useGaugeUserKey();
  const params = useLocalSearchParams<{ draft?: string }>();
  const loadDrafts = useDrafts((s) => s.load);
  loadDrafts(userKey);
  const saveDraft = useDrafts((s) => s.save);
  const removeDraft = useDrafts((s) => s.remove);
  const [initial] = useState(() => (params.draft ? (useDrafts.getState().drafts.find((d) => d.id === params.draft) ?? null) : null));

  const [step, setStep] = useState<'vehicle' | 'measure'>(initial ? 'measure' : 'vehicle');
  const [plate, setPlate] = useState(initial?.plate ?? '');
  const [vin, setVin] = useState(initial?.vin ?? '');
  const [bodyType, setBodyType] = useState<string | null>(initial?.bodyType ?? null);
  const bodyTypes = useBodyTypes();
  const [values, setValues] = useState<Record<string, string>>(initial?.values ?? {});
  // One key per measurement (kept in its draft): a retried send never stores it twice.
  const [clientId] = useState(() => initial?.id ?? newClientId());

  // SmartPTG: a steady reading is appended to the selected part.
  const gaugeOn = isGaugeAvailable();
  const gaugeConnected = useGauge((s) => s.status === 'connected');
  const [activePart, setActivePart] = useState<string | null>(null);

  // Guided mode: a picked body type walks its numbered points (guided.ts).
  const partMap = useQuery({
    queryKey: ['measurements', 'part-map', bodyType],
    queryFn: () => unwrap(api.GET('/v1/measurement-part-maps/{body_type}', { params: { path: { body_type: bodyType! } } })),
    enabled: !!bodyType,
    staleTime: Infinity,
    retry: false,
  });
  const mapData = partMap.data as PartMapData | undefined;
  const targets = useMemo(() => (mapData ? buildTargets(mapData) : []), [mapData]);
  const guided = !!bodyType && targets.length > 0;
  const [points, setPoints] = useState<Record<string, PointReading>>(initial?.points ?? {});
  const [cursor, setCursor] = useState(initial?.cursor ?? 0);
  const [history, setHistory] = useState<string[]>(initial?.history ?? []);
  const target = guided ? (targets[Math.min(cursor, targets.length - 1)] ?? null) : null;
  const [substrates, setSubstrates] = useState<Record<string, 'Fe' | 'Al'>>(initial?.substrates ?? {});
  const [gaugeDevice, setGaugeDevice] = useState<{ serial: string; model?: string } | null>(initial?.gaugeDevice ?? null);
  // Raw probe data of every gauge reading (TEC-939): frequency and channel let a corrected curve recompute it later.
  const [gaugeReads, setGaugeReads] = useState<GaugeRead[]>(initial?.gaugeReads ?? []);
  const decimal = new Intl.NumberFormat(i18n.language).format(1.5).replace(/\d/g, '') || '.';
  useGaugeSession((c) => {
    if (step !== 'measure') return;
    const g0 = useGauge.getState();
    if (guided && target) {
      const key = targetKey(target);
      const reading: PointReading = { um: c.um, level: saplLevel(c.um), material: c.material, at: new Date().toISOString() };
      const nextPoints = { ...points, [key]: reading };
      setPoints(nextPoints);
      setHistory((h) => [...h.filter((k) => k !== key), key]);
      const next = nextOpen(targets, nextPoints, targets.indexOf(target));
      if (next >= 0) setCursor(next);
      if (g0.serial != null) setGaugeDevice({ serial: String(g0.serial), model: g0.model ?? undefined });
      setGaugeReads((list) => [
        ...list,
        {
          part_type: target.part,
          value_um: c.um,
          frequency_hz: c.frequency,
          channel: c.channel,
          material: c.material,
          trigger: c.trigger,
          fe_offset_hz: g0.calibration?.fe ?? null,
          al_offset_hz: g0.calibration?.al ?? null,
          at: reading.at,
        },
      ]);
      return;
    }
    const part = activePart ?? PARTS[0]!.key;
    if (!activePart) setActivePart(part);
    setValues((v) => ({ ...v, [part]: appendValue(v[part] ?? '', formatInputValue(c.um, unit, decimal)) }));
    const substrate = substrateOf(c.material);
    if (substrate) setSubstrates((m) => ({ ...m, [part]: substrate }));
    const g = useGauge.getState();
    if (g.serial != null) setGaugeDevice({ serial: String(g.serial), model: g.model ?? undefined });
    setGaugeReads((list) => [
      ...list,
      {
        part_type: part,
        value_um: c.um,
        frequency_hz: c.frequency,
        channel: c.channel,
        material: c.material,
        trigger: c.trigger,
        fe_offset_hz: g.calibration?.fe ?? null,
        al_offset_hz: g.calibration?.al ?? null,
        at: new Date().toISOString(),
      },
    ]);
  });

  const parts = PARTS.map((p) => ({ ...p, values: parseValues(values[p.key] ?? '').map((v) => toMicrons(v, unit)) })).filter(
    (p) => p.values.length > 0,
  );
  // Values come only from the gauge (no manual entry).
  const pointCount = Object.keys(points).length;
  const fromGauge = gaugeDevice != null && (guided ? pointCount > 0 : parts.some((p) => substrates[p.key]));
  const plateClean = plate.trim().toUpperCase();
  const vinClean = vin.trim().toUpperCase();
  const vinError = vinClean && !VIN_RE.test(vinClean) ? t('vin_invalid') : undefined;
  const vinReady = VIN_RE.test(vinClean);

  const removeLast = (part: string) => {
    setValues((v) => {
      const list = (v[part] ?? '').trim().split(/\s+/u).filter(Boolean);
      list.pop();
      return { ...v, [part]: list.join(' ') };
    });
    setGaugeReads((list) => {
      const i = list.map((r) => r.part_type).lastIndexOf(part);
      return i < 0 ? list : [...list.slice(0, i), ...list.slice(i + 1)];
    });
  };

  // The gauge's button in the air: short → next part, held → remove the last value of the selected part.
  // Guided: undo the last reading (back to its point) / skip the current point.
  const undoLast = () => {
    const key = history[history.length - 1];
    if (!key) return;
    setHistory((h) => h.slice(0, -1));
    setPoints((p) => {
      const next = { ...p };
      delete next[key];
      return next;
    });
    const i = targets.findIndex((t) => targetKey(t) === key);
    if (i >= 0) setCursor(i);
    setGaugeReads((list) => {
      const part = key.split('#')[0];
      const i2 = list.map((r) => r.part_type).lastIndexOf(part!);
      return i2 < 0 ? list : [...list.slice(0, i2), ...list.slice(i2 + 1)];
    });
  };
  const skipPoint = () => setCursor((c) => (targets.length ? (c + 1) % targets.length : 0));
  const skipPart = () => {
    const i = nextPartIndex(targets, cursor);
    setCursor(i >= 0 ? i : 0);
  };
  const prevPoint = () => setCursor((c) => (c > 0 ? c - 1 : Math.max(0, targets.length - 1)));

  useGaugeButtons((e) => {
    if (step !== 'measure') return;
    if (guided) {
      if (e === 'short') skipPoint();
      else undoLast();
      return;
    }
    const current = activePart ?? PARTS[0]!.key;
    if (e === 'short') {
      const i = PARTS.findIndex((p) => p.key === current);
      setActivePart(PARTS[(i + 1) % PARTS.length]!.key);
    } else {
      removeLast(current);
    }
  });

  // Point readings belong to one body type: changing it after measuring starts over (asked first).
  const changeBodyType = (code: string | null) => {
    if (code === bodyType) return;
    if (!Object.keys(points).length) {
      setBodyType(code);
      setCursor(0);
      return;
    }
    Alert.alert(t('body_change_title'), t('body_change_body'), [
      { text: t('common:cancel'), style: 'cancel' },
      {
        text: t('body_change_confirm'),
        style: 'destructive',
        onPress: () => {
          setBodyType(code);
          setPoints({});
          setHistory([]);
          setCursor(0);
          setGaugeReads([]);
        },
      },
    ]);
  };

  const keepDraft = () => {
    saveDraft({
      id: clientId,
      plate: plateClean,
      vin: vinClean,
      bodyType,
      values,
      substrates,
      gaugeDevice,
      gaugeReads,
      points,
      cursor,
      history,
      updatedAt: new Date().toISOString(),
    });
    Alert.alert(t('draft_saved'));
    router.back();
  };

  const save = useMutation({
    mutationFn: () => {
      const now = new Date().toISOString();
      return unwrap(
        api.POST('/v1/mobile/measurements', {
          params: { header: { 'X-Mobile-Api-Version': MOBILE_API_VERSION, 'Idempotency-Key': clientId } },
          body: {
            client_measurement_id: clientId,
            plate: plateClean || undefined,
            vin: vinClean,
            body_type: bodyType ?? undefined,
            measured_at: now,
            unit: 'um',
            device: gaugeDevice ?? undefined,
            parts: guided ? guidedParts(targets, points) : parts.map((p) => ({ part: p.key, place: p.place, values: p.values })),
            raw: {
              source: 'smartptg',
              body_type: bodyType ?? undefined,
              gauge: gaugeRaw(gaugeReads),
              app_version: APP_VERSION,
              date: now,
              measurements: guided
                ? guidedRows(targets, points)
                : parts.flatMap((p) =>
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
    onSuccess: () => {
      removeDraft(clientId);
      void qc.invalidateQueries({ queryKey: ['measurements', 'list'] });
      Alert.alert(t('saved'));
      router.back();
    },
    onError: (e) => Alert.alert(errorMessage(isApiError(e) ? e.code : undefined)),
  });

  const unitLabel = unit === 'mil' ? 'mil' : 'µm';
  const body = bodyTypeOf(bodyTypes.data, bodyType);

  if (step === 'vehicle') {
    return (
      <>
        <Stack.Screen options={{ title: t('new'), headerLargeTitle: false }} />
        <Screen>
          <Text variant="footnote" tone="muted">
            {t('step_vehicle_hint')}
          </Text>
          <Input
            label={t('plate_required')}
            placeholder={t('placeholder_plate', { ns: 'common' })}
            value={plate}
            onChangeText={setPlate}
            autoCapitalize="characters"
            autoCorrect={false}
            trailing={<PlateScanButton onResult={setPlate} />}
          />
          <Input
            label={t('vin')}
            placeholder={t('placeholder_vin', { ns: 'common' })}
            value={vin}
            onChangeText={setVin}
            autoCapitalize="characters"
            autoCorrect={false}
            maxLength={17}
            error={vinError}
            hint={t('vin_hint_send')}
          />
          <SectionHeader title={t('body_type')} />
          <Text variant="footnote" tone="muted" className="-mt-2">
            {t('body_type_hint')}
          </Text>
          <BodyTypePicker value={bodyType} onChange={changeBodyType} />
          <Button
            size="lg"
            title={t('start_measuring')}
            icon={<Icon name="gauge" tone="primary-foreground" size={18} />}
            disabled={!plateClean || !!vinError}
            onPress={() => setStep('measure')}
          />
          <Button title={t('save_draft')} variant="ghost" disabled={!plateClean || !!vinError} onPress={keepDraft} />
        </Screen>
      </>
    );
  }

  return (
    <>
      <Stack.Screen options={{ title: plateClean || t('new'), headerLargeTitle: false }} />
      <Screen>
        <Pressable accessibilityRole="button" onPress={() => setStep('vehicle')} className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-3">
          {body ? <Image source={{ uri: body.thumbnail }} style={{ width: 72, height: 36 }} resizeMode="contain" /> : <Icon name="vehicle" tone="muted-foreground" />}
          <View className="flex-1 gap-0.5">
            <Text variant="headline" numberOfLines={1}>
              {plateClean}
            </Text>
            <Text variant="footnote" tone={vinReady ? 'muted' : 'danger'} numberOfLines={1}>
              {[vinReady ? vinClean : t('vin_missing'), body ? t(`body.${body.key}`, { defaultValue: body.name }) : null].filter(Boolean).join(' · ')}
            </Text>
          </View>
          <Icon name="edit" tone="primary" size={18} />
        </Pressable>

        {gaugeOn ? (
          <GaugePanel targetLabel={activePart ? t(`parts.${activePart}`) : gaugeConnected ? t(`parts.${PARTS[0]!.key}`) : null} />
        ) : null}

        {bodyType && partMap.isPending ? (
          <ListSkeleton rows={2} inset={false} />
        ) : guided && target && mapData ? (
          <GuidePanel
            map={mapData}
            targets={targets}
            target={target}
            points={points}
            onPrev={prevPoint}
            onSkipPoint={skipPoint}
            onSkipPart={skipPart}
            onUndo={undoLast}
            onJump={(index) => setCursor(index)}
            canUndo={history.length > 0}
          />
        ) : (
          <>
          <SectionHeader title={t('values_in', { unit: unitLabel })} />
          <Text variant="footnote" tone="muted" className="-mt-2">
            {t('values_hint_gauge')}
          </Text>
          <View className="gap-2">
            {PARTS.map((p) => (
              <PartRow
                key={p.key}
                label={t(`parts.${p.key}`)}
                values={values[p.key] ?? ''}
                active={activePart === p.key}
                onSelect={() => setActivePart(p.key)}
                onRemoveLast={() => removeLast(p.key)}
                removeLabel={t('remove_last')}
              />
            ))}
          </View>
          </>
        )}

        {!vinReady ? (
          <Text variant="footnote" tone="danger">
            {t('vin_required_to_send')}
          </Text>
        ) : null}
        <Button
          size="lg"
          title={t('save')}
          loading={save.isPending}
          disabled={(guided ? pointCount === 0 : parts.length === 0) || !fromGauge || !vinReady}
          onPress={() => save.mutate()}
        />
        <Button title={t('save_draft')} variant="ghost" disabled={save.isPending} onPress={keepDraft} />
      </Screen>
    </>
  );
}
