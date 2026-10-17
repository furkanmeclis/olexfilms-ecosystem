import { useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, type AlertButton, RefreshControl, View } from 'react-native';

import { useCan } from '@/core/spaces';
import { ListTree, Plus } from '@/ui/icons';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { CodeNameSheet } from './code-name-sheet';
import { alertError, confirmAction, go } from './format';
import { Card, ErrorState, Field, ListSkeleton } from './parts';
import {
  allowedChildTypes,
  type LocationType,
  moved,
  setupApi,
  sk,
  useRoomLocations,
  useSetupRoom,
  type WarehouseLocation,
} from './setup-api';
import { OrderRow, ToggleRow } from './setup-screen';

type Node = WarehouseLocation & { depth: number };

/** Flattens the room's locations into tree order (parents before children, siblings by sort order). */
function flatten(items: WarehouseLocation[]): Node[] {
  const byParent = new Map<string | null, WarehouseLocation[]>();
  const ids = new Set(items.map((i) => i.uuid));
  for (const i of items) {
    const key = i.parent_uuid && ids.has(i.parent_uuid) ? i.parent_uuid : null;
    byParent.set(key, [...(byParent.get(key) ?? []), i]);
  }
  for (const list of byParent.values()) list.sort((a, b) => a.sort_order - b.sort_order || a.code.localeCompare(b.code));
  const out: Node[] = [];
  const walk = (parent: string | null, depth: number) => {
    for (const n of byParent.get(parent) ?? []) {
      out.push({ ...n, depth });
      walk(n.uuid, depth + 1);
    }
  };
  walk(null, 0);
  return out;
}

type Sheet =
  | { kind: 'room' }
  | { kind: 'new'; parent: Node | null; types: LocationType[] }
  | { kind: 'edit'; node: Node };

/** One room: its details and the aisle › shelf › bin tree, editable on the phone. */
export function RoomScreen() {
  const { t } = useTranslation('warehouse');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const write = useCan('warehouse.write');
  const qc = useQueryClient();
  const room = useSetupRoom(uuid);
  const locs = useRoomLocations(uuid);
  const [sheet, setSheet] = useState<Sheet | null>(null);
  const nodes = flatten(locs.data ?? []);

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ['warehouse', 'setup'] });
    void qc.invalidateQueries({ queryKey: ['warehouse', 'rooms'] });
  };

  const siblings = (n: Node) =>
    nodes.filter((x) => (x.parent_uuid ?? null) === (n.parent_uuid ?? null) && x.depth === n.depth).map((x) => x.uuid);

  const act = async (fn: () => Promise<unknown>) => {
    try {
      await fn();
      refresh();
    } catch (e) {
      alertError(e);
    }
  };

  const menu = (n: Node) => {
    const child = allowedChildTypes(n.type);
    const buttons: AlertButton[] = [
      { text: t('m.location_stock'), onPress: () => go(`/depo/stock/location/${n.uuid}?name=${encodeURIComponent(n.full_code)}`) },
    ];
    if (write) {
      if (child.length) buttons.push({ text: t('m.add_child', { type: t(`type.${child[0]}`) }), onPress: () => setSheet({ kind: 'new', parent: n, types: child }) });
      if (child.length) buttons.push({ text: t('bulk.open_under', { code: n.code }), onPress: () => go(`/depo/setup/generate?room=${uuid}&parent=${n.uuid}&ptype=${n.type}&pcode=${encodeURIComponent(n.full_code)}`) });
      buttons.push({ text: t('form.edit_location'), onPress: () => setSheet({ kind: 'edit', node: n }) });
      buttons.push({
        text: n.active ? t('m.deactivate') : t('m.activate'),
        onPress: () => void act(() => setupApi.updateLocation(n.uuid, { active: !n.active })),
      });
      buttons.push({ text: t('m.move_up'), onPress: () => { const o = moved(siblings(n), n.uuid, -1); if (o) void act(() => setupApi.reorderLocations(o)); } });
      buttons.push({ text: t('m.move_down'), onPress: () => { const o = moved(siblings(n), n.uuid, 1); if (o) void act(() => setupApi.reorderLocations(o)); } });
      buttons.push({
        text: t('delete.confirm'),
        style: 'destructive',
        onPress: () =>
          confirmAction({
            title: t('delete.title', { code: n.full_code }),
            body: t('delete.location'),
            action: t('delete.confirm'),
            destructive: true,
            onConfirm: () => void act(() => setupApi.deleteLocation(n.uuid)),
          }),
      });
    }
    buttons.push({ text: t('common:cancel'), style: 'cancel' });
    Alert.alert(n.full_code, n.name && n.name !== n.code ? n.name : t(`type.${n.type}`), buttons);
  };

  const r = room.data;
  if (!r) {
    return (
      <>
        <Stack.Screen options={{ title: t('fields.room'), headerLargeTitle: false }} />
        <Screen>{room.isLoading ? <ListSkeleton rows={3} /> : <ErrorState error={room.error} onRetry={() => void room.refetch()} />}</Screen>
      </>
    );
  }

  return (
    <>
      <Stack.Screen options={{ title: r.name || r.code, headerLargeTitle: false }} />
      <Screen
        refreshControl={
          <RefreshControl
            refreshing={locs.isRefetching}
            onRefresh={() => {
              void room.refetch();
              void locs.refetch();
            }}
          />
        }>
        <Card>
          <Field label={t('fields.code')} value={r.code} mono />
          <Field label={t('fields.name')} value={r.name} />
          {write ? (
            <View className="flex-row gap-2 pt-2">
              <Button className="flex-1" variant="secondary" title={t('form.edit_room')} onPress={() => setSheet({ kind: 'room' })} />
              <Button
                className="flex-1"
                variant="outline"
                title={t('delete.confirm')}
                onPress={() =>
                  confirmAction({
                    title: t('delete.title', { code: r.code }),
                    body: t('delete.room'),
                    action: t('delete.confirm'),
                    destructive: true,
                    onConfirm: () =>
                      void act(async () => {
                        await setupApi.deleteRoom(uuid);
                        router.back();
                      }),
                  })
                }
              />
            </View>
          ) : null}
        </Card>
        {write ? (
          <ToggleRow
            label={t('fields.active')}
            value={r.active}
            onChange={(b) =>
              void act(async () => qc.setQueryData(sk.room(uuid), await setupApi.updateRoom(uuid, { active: b })))
            }
          />
        ) : null}

        <SectionHeader title={t('locations.tree')} />
        {write ? (
          <View className="flex-row gap-2">
            <Button
              className="flex-1"
              variant="secondary"
              icon={<Icon as={Plus} tone="primary" size={18} />}
              title={t('locations.new_root')}
              onPress={() => setSheet({ kind: 'new', parent: null, types: allowedChildTypes(null) })}
            />
            <Button
              className="flex-1"
              variant="secondary"
              icon={<Icon as={ListTree} tone="primary" size={18} />}
              title={t('bulk.open')}
              onPress={() => go(`/depo/setup/generate?room=${uuid}`)}
            />
          </View>
        ) : null}
        {locs.isLoading ? (
          <ListSkeleton rows={4} />
        ) : locs.error && !locs.data ? (
          <ErrorState error={locs.error} onRetry={() => void locs.refetch()} />
        ) : nodes.length === 0 ? (
          <Text variant="callout" tone="muted" className="px-4">
            {t('locations.no_locations')}
          </Text>
        ) : (
          <View className="gap-1.5">
            {nodes.map((n) => (
              <OrderRow
                key={n.uuid}
                title={n.full_code}
                subtitle={[t(`type.${n.type}`), n.name && n.name !== n.code ? n.name : null].filter(Boolean).join(' · ')}
                inactive={!n.active}
                indent={Math.min(n.depth, 3)}
                canEdit={false}
                onPress={() => menu(n)}
              />
            ))}
          </View>
        )}
      </Screen>

      {sheet?.kind === 'room' ? (
        <CodeNameSheet
          title={t('form.edit_room')}
          initial={{ code: r.code, name: r.name }}
          onClose={() => setSheet(null)}
          onSubmit={async (v) => {
            qc.setQueryData(sk.room(uuid), await setupApi.updateRoom(uuid, { code: v.code, name: v.name }));
            refresh();
          }}
        />
      ) : sheet?.kind === 'new' ? (
        <CodeNameSheet
          title={sheet.parent ? `${t('form.create_location')} · ${sheet.parent.full_code}` : t('form.create_location')}
          types={sheet.types}
          onClose={() => setSheet(null)}
          onSubmit={async (v) => {
            await setupApi.createLocation({
              room_uuid: uuid,
              parent_uuid: sheet.parent?.uuid ?? null,
              type: v.type ?? sheet.types[0],
              code: v.code,
              name: v.name || undefined,
            });
            refresh();
          }}
        />
      ) : sheet?.kind === 'edit' ? (
        <CodeNameSheet
          title={`${t('form.edit_location')} · ${sheet.node.full_code}`}
          initial={{ code: sheet.node.code, name: sheet.node.name }}
          onClose={() => setSheet(null)}
          onSubmit={async (v) => {
            await setupApi.updateLocation(sheet.node.uuid, { code: v.code, name: v.name });
            refresh();
          }}
        />
      ) : null}
    </>
  );
}
