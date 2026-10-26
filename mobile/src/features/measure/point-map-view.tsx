import { useQuery } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';
import { SvgXml } from 'react-native-svg';

import { api, unwrap } from '@/core/api/client';
import { usePreferences } from '@/core/preferences';
import { formatThickness } from '@/core/units';
import { useFormat } from '@/features/admin/kit';
import { Icon } from '@/ui/icon';
import { List } from '@/ui/list';
import { Text } from '@/ui/text';

import {
  composeView,
  levelColor,
  partPoints,
  PLACES,
  type MapValue,
  type PartMapData,
  type PointValue,
} from './point-map';

function PointChip({ point }: { point: PointValue }) {
  const unit = usePreferences((s) => s.unit);
  const f = useFormat();
  return (
    <View className="flex-row items-center gap-1.5 rounded-full border border-border px-1.5 py-0.5">
      <View
        className="size-5 items-center justify-center rounded-full"
        style={{ backgroundColor: levelColor(point.level) }}>
        <Text variant="caption" className="text-[10px] font-bold text-gray-900">
          {point.position}
        </Text>
      </View>
      <Text variant="footnote" className="font-semibold tabular-nums">
        {point.um === null ? '—' : formatThickness(point.um, unit, f.locale)}
      </Text>
    </View>
  );
}

/**
 * The NexPTG part map of a measurement (TEC-1010): the four views with the
 * numbered points in their level color and, per part, the thickness of every
 * numbered point (outside and inside) in the user's unit.
 */
export function MeasurementMap({ bodyType, values }: { bodyType: string | null; values: MapValue[] }) {
  const { t } = useTranslation('measure');
  const map = useQuery({
    queryKey: ['measurements', 'part-map', bodyType],
    queryFn: () =>
      unwrap(
        api.GET('/v1/measurement-part-maps/{body_type}', {
          params: { path: { body_type: bodyType! } },
        }),
      ),
    enabled: !!bodyType,
    staleTime: Infinity,
    retry: false,
  });
  const data = map.data as PartMapData | undefined;
  const views = useMemo(
    () =>
      data
        ? PLACES.flatMap((place) => {
            const svg = composeView(data, place, values);
            return svg ? [{ place, svg }] : [];
          })
        : [],
    [data, values],
  );
  const parts = useMemo(() => partPoints(data, values), [data, values]);

  return (
    <View className="gap-3">
      {views.length ? (
        <View className="flex-row flex-wrap gap-2">
          {views.map((v) => (
            <View key={v.place} className="w-[48%] overflow-hidden rounded-lg border border-border bg-white p-1">
              <SvgXml xml={v.svg} width="100%" height={undefined} style={{ aspectRatio: 2 }} />
              <Text variant="caption" tone="muted" className="text-center">
                {t(`places.${v.place}`)}
              </Text>
            </View>
          ))}
        </View>
      ) : bodyType && map.isPending ? null : (
        <Text variant="footnote" tone="muted">
          {t('map_unavailable')}
        </Text>
      )}
      {parts.length ? (
        <List>
          {parts.map((p, i) => (
            <View
              key={p.part}
              className={i < parts.length - 1 ? 'gap-1.5 border-b border-border px-4 py-3' : 'gap-1.5 px-4 py-3'}>
              <Text variant="body">{t(`parts.${p.part}`, { defaultValue: p.part })}</Text>
              {(
                [
                  ['outside', p.outside],
                  ['inside', p.inside],
                ] as const
              ).map(([side, list]) =>
                list.length ? (
                  <View key={side} className="gap-1">
                    {p.inside.length ? (
                      <Text variant="caption" tone="muted">
                        {t(`points_${side}`)}
                      </Text>
                    ) : null}
                    <View className="flex-row flex-wrap gap-1.5" style={{ direction: 'ltr' }}>
                      {list.map((pt) => (
                        <PointChip key={pt.position} point={pt} />
                      ))}
                    </View>
                  </View>
                ) : null,
              )}
            </View>
          ))}
        </List>
      ) : null}
    </View>
  );
}

/** A linked measurement of a service, opened on demand. */
function PhaseMeasurement({
  phase,
  uuid,
  measuredAt,
}: {
  phase: 'before' | 'after';
  uuid: string;
  measuredAt: string;
}) {
  const { t } = useTranslation('measure');
  const f = useFormat();
  const [open, setOpen] = useState(false);
  const q = useQuery({
    queryKey: ['measurements', 'detail', uuid],
    queryFn: () => unwrap(api.GET('/v1/measurements/{uuid}', { params: { path: { uuid } } })),
    enabled: open,
  });
  return (
    <View className="gap-2">
      <Pressable
        accessibilityRole="button"
        onPress={() => setOpen((o) => !o)}
        className="flex-row items-center justify-between gap-3 px-4 py-3">
        <View className="flex-1 gap-0.5">
          <Text variant="body">{t(`phase.${phase}`)}</Text>
          <Text variant="footnote" tone="muted">
            {f.dateTime(measuredAt)}
          </Text>
        </View>
        <Icon name={open ? 'chevronUp' : 'chevronDown'} size={16} />
      </Pressable>
      {open ? (
        <View className="px-3 pb-3">
          {q.data ? (
            <MeasurementMap bodyType={q.data.body_type} values={q.data.values} />
          ) : (
            <Text variant="footnote" tone="muted">
              {q.isError ? t('map_unavailable') : '…'}
            </Text>
          )}
        </View>
      ) : null}
    </View>
  );
}

/**
 * Before / after measurements of a service with their part maps (TEC-1010).
 * Needs measurements.link (the links) and measurements.read (the detail).
 */
export function ServiceMeasurementMaps({ serviceUuid }: { serviceUuid: string }) {
  const q = useQuery({
    queryKey: ['services', 'measurements', serviceUuid],
    queryFn: () =>
      unwrap(
        api.GET('/v1/services/{uuid}/measurements', {
          params: { path: { uuid: serviceUuid } },
        }),
      ),
  });
  const links = q.data?.links ?? [];
  if (!links.length) return null;
  return (
    <List>
      {links.map((l, i) => (
        <View key={l.phase} className={i < links.length - 1 ? 'border-b border-border' : undefined}>
          <PhaseMeasurement
            phase={l.phase}
            uuid={l.measurement.uuid}
            measuredAt={l.measurement.measured_at ?? l.linked_at}
          />
        </View>
      ))}
    </List>
  );
}
