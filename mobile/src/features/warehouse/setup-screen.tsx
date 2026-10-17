import { useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, RefreshControl, Switch, View } from 'react-native';

import { useCan } from '@/core/spaces';
import { ChevronDown, ChevronUp, Plus, Warehouse as WarehouseIcon } from '@/ui/icons';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { wk } from './api';
import { CodeNameSheet } from './code-name-sheet';
import { alertError, confirmAction, go, replace } from './format';
import { ErrorState, IconButton, ListSkeleton } from './parts';
import { CODE_RE, moved, normalizeCode, setupApi, sk, useAllWarehouses, useSetupRooms, useSetupWarehouse } from './setup-api';

/** Refreshes setup lists and the warehouse picker after a change. */
function useSetupRefresh() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: ['warehouse', 'setup'] });
    void qc.invalidateQueries({ queryKey: wk.warehouses() });
    void qc.invalidateQueries({ queryKey: ['warehouse', 'rooms'] });
  };
}

/** A row with up/down reorder buttons; tapping the body opens it. */
export function OrderRow({
  title,
  subtitle,
  inactive,
  onPress,
  onUp,
  onDown,
  indent = 0,
  canEdit,
}: {
  title: string;
  subtitle?: string;
  inactive?: boolean;
  onPress: () => void;
  onUp?: () => void;
  onDown?: () => void;
  indent?: number;
  canEdit: boolean;
}) {
  const { t } = useTranslation('warehouse');
  return (
    <View className="flex-row items-center rounded-lg border border-border bg-card" style={{ marginStart: indent * 16 }}>
      <Pressable accessibilityRole="button" onPress={onPress} className="flex-1 gap-0.5 py-3 ps-4 active:opacity-70">
        <View className="flex-row items-center gap-2">
          <Text variant="callout" className="font-semibold" numberOfLines={1}>
            {title}
          </Text>
          {inactive ? <StatusPill tone="neutral" label={t('m.inactive')} /> : null}
        </View>
        {subtitle ? (
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {subtitle}
          </Text>
        ) : null}
      </Pressable>
      {canEdit ? (
        <View className="flex-row pe-1">
          <IconButton icon={ChevronUp} label={t('m.move_up')} onPress={() => onUp?.()} />
          <IconButton icon={ChevronDown} label={t('m.move_down')} onPress={() => onDown?.()} />
        </View>
      ) : null}
    </View>
  );
}

/** Warehouse setup: every warehouse of the organization, with order and status. */
export function SetupScreen() {
  const { t } = useTranslation('warehouse');
  const write = useCan('warehouse.write');
  const q = useAllWarehouses();
  const refresh = useSetupRefresh();
  const list = q.data ?? [];

  const reorder = async (uuid: string, dir: -1 | 1) => {
    const next = moved(
      list.map((w) => w.uuid),
      uuid,
      dir,
    );
    if (!next) return;
    try {
      await setupApi.reorderWarehouses(next);
      refresh();
    } catch (e) {
      alertError(e);
    }
  };

  return (
    <>
      <Stack.Screen
        options={{
          title: t('m.setup'),
          headerLargeTitle: false,
          headerRight: write
            ? () => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('form.create_warehouse')}
                  hitSlop={8}
                  onPress={() => go('/depo/setup/warehouse/new')}
                  className="size-8 items-center justify-center">
                  <Icon as={Plus} tone="primary" />
                </Pressable>
              )
            : undefined,
        }}
      />
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        <Text variant="callout" tone="muted">
          {t('locations.description')}
        </Text>
        {q.isLoading ? (
          <ListSkeleton rows={3} />
        ) : !q.data ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : list.length === 0 ? (
          <EmptyState
            icon={WarehouseIcon}
            title={t('locations.no_warehouses')}
            action={write ? { title: t('form.create_warehouse'), onPress: () => go('/depo/setup/warehouse/new') } : undefined}
          />
        ) : (
          <View className="gap-2">
            {list.map((w) => (
              <OrderRow
                key={w.uuid}
                title={w.name}
                subtitle={[w.code, w.address].filter(Boolean).join(' · ')}
                inactive={!w.active}
                canEdit={write}
                onPress={() => go(`/depo/setup/warehouse/${w.uuid}`)}
                onUp={() => void reorder(w.uuid, -1)}
                onDown={() => void reorder(w.uuid, 1)}
              />
            ))}
          </View>
        )}
        <Text variant="footnote" tone="muted">
          {t('m.import_web')}
        </Text>
      </Screen>
    </>
  );
}

/** Create or edit one warehouse, and manage its rooms. */
export function WarehouseEditScreen() {
  const { t } = useTranslation('warehouse');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const isNew = uuid === 'new';
  const write = useCan('warehouse.write');
  const qc = useQueryClient();
  const refresh = useSetupRefresh();
  const w = useSetupWarehouse(uuid, !isNew);
  const rooms = useSetupRooms(uuid, !isNew);
  const [draft, setDraft] = useState<{ code: string; name: string; address: string; placement: boolean; active: boolean } | null>(
    null,
  );
  const [busy, setBusy] = useState<string | null>(null);
  const [tried, setTried] = useState(false);
  const [roomSheet, setRoomSheet] = useState(false);

  const base = w.data;
  const v = draft ?? {
    code: base?.code ?? '',
    name: base?.name ?? '',
    address: base?.address ?? '',
    placement: base?.require_location_placement ?? false,
    active: base?.active ?? true,
  };
  const set = (patch: Partial<typeof v>) => setDraft({ ...v, ...patch });
  const codeOk = CODE_RE.test(normalizeCode(v.code));

  const save = async () => {
    setTried(true);
    if (!codeOk) return;
    setBusy('save');
    try {
      if (isNew) {
        const x = await setupApi.createWarehouse({
          code: normalizeCode(v.code),
          name: v.name.trim() || undefined,
          address: v.address.trim() || null,
          require_location_placement: v.placement,
        });
        refresh();
        replace(`/depo/setup/warehouse/${x.uuid}`);
      } else {
        const x = await setupApi.updateWarehouse(uuid, {
          code: normalizeCode(v.code),
          name: v.name.trim(),
          address: v.address.trim() || null,
          require_location_placement: v.placement,
          active: v.active,
        });
        qc.setQueryData(sk.warehouse(uuid), x);
        setDraft(null);
        refresh();
      }
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(null);
    }
  };

  const remove = () =>
    confirmAction({
      title: t('delete.title', { code: base?.code ?? '' }),
      body: t('delete.warehouse'),
      action: t('delete.confirm'),
      destructive: true,
      onConfirm: async () => {
        setBusy('delete');
        try {
          await setupApi.deleteWarehouse(uuid);
          refresh();
          router.back();
        } catch (e) {
          alertError(e);
        } finally {
          setBusy(null);
        }
      },
    });

  const reorderRoom = async (room: string, dir: -1 | 1) => {
    const next = moved((rooms.data ?? []).map((r) => r.uuid), room, dir);
    if (!next) return;
    try {
      await setupApi.reorderRooms(next);
      refresh();
    } catch (e) {
      alertError(e);
    }
  };

  if (!isNew && !base) {
    return (
      <>
        <Stack.Screen options={{ title: t('form.edit_warehouse'), headerLargeTitle: false }} />
        <Screen>{w.isLoading ? <ListSkeleton rows={3} /> : <ErrorState error={w.error} onRetry={() => void w.refetch()} />}</Screen>
      </>
    );
  }

  return (
    <>
      <Stack.Screen options={{ title: isNew ? t('form.create_warehouse') : (base?.name ?? ''), headerLargeTitle: false }} />
      <Screen>
        <Input
          label={t('fields.code')}
          value={v.code}
          onChangeText={(s) => set({ code: s })}
          autoCapitalize="characters"
          autoCorrect={false}
          editable={write}
          hint={t('form.code_hint')}
          error={tried && !codeOk ? t('validation.code') : undefined}
        />
        <Input label={t('fields.name')} value={v.name} onChangeText={(s) => set({ name: s })} editable={write} hint={t('form.name_hint')} />
        <Input label={t('fields.address')} value={v.address} onChangeText={(s) => set({ address: s })} editable={write} multiline className="min-h-20 py-3" />
        <ToggleRow
          label={t('form.require_location_placement')}
          hint={t('form.require_location_placement_hint')}
          value={v.placement}
          disabled={!write}
          onChange={(b) => set({ placement: b })}
        />
        {!isNew ? (
          <ToggleRow label={t('fields.active')} value={v.active} disabled={!write} onChange={(b) => set({ active: b })} />
        ) : null}
        {write ? (
          <View className="gap-2">
            <Button size="lg" title={t('form.save')} loading={busy === 'save'} disabled={!isNew && !draft} onPress={() => void save()} />
            {!isNew ? <Button variant="outline" title={t('delete.confirm')} loading={busy === 'delete'} onPress={remove} /> : null}
          </View>
        ) : null}

        {!isNew ? (
          <>
            <View className="flex-row items-center justify-between">
              <SectionHeader title={t('locations.rooms')} />
              {write ? <Button variant="ghost" title={t('form.create_room')} onPress={() => setRoomSheet(true)} /> : null}
            </View>
            {rooms.isLoading ? (
              <ListSkeleton rows={2} />
            ) : (rooms.data?.length ?? 0) === 0 ? (
              <Text variant="callout" tone="muted" className="px-4">
                {t('locations.no_rooms')}
              </Text>
            ) : (
              <View className="gap-2">
                {rooms.data?.map((r) => (
                  <OrderRow
                    key={r.uuid}
                    title={r.name || r.code}
                    subtitle={r.code}
                    inactive={!r.active}
                    canEdit={write}
                    onPress={() => go(`/depo/setup/room/${r.uuid}`)}
                    onUp={() => void reorderRoom(r.uuid, -1)}
                    onDown={() => void reorderRoom(r.uuid, 1)}
                  />
                ))}
              </View>
            )}
          </>
        ) : null}
      </Screen>
      {roomSheet ? (
        <CodeNameSheet
          title={t('form.create_room')}
          onClose={() => setRoomSheet(false)}
          onSubmit={async (x) => {
            await setupApi.createRoom(uuid, { code: x.code, name: x.name || undefined });
            refresh();
          }}
        />
      ) : null}
    </>
  );
}

export function ToggleRow({
  label,
  hint,
  value,
  onChange,
  disabled,
}: {
  label: string;
  hint?: string;
  value: boolean;
  onChange: (b: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <View className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-3">
      <View className="flex-1 gap-0.5">
        <Text variant="body">{label}</Text>
        {hint ? (
          <Text variant="footnote" tone="muted">
            {hint}
          </Text>
        ) : null}
      </View>
      <Switch accessibilityLabel={label} value={value} onValueChange={onChange} disabled={disabled} />
    </View>
  );
}
