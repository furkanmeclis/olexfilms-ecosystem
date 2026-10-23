import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, FlatList, Pressable, RefreshControl, ScrollView, Switch, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useChildHref, useFormat } from '@/features/admin/kit';
import { FleetRow } from '@/features/customer-space/fleet-screens';
import { Sheet } from '@/features/services/sheet';
import { Chips, DetailSkeleton, ErrorState, errorText, InfoRow, ListBody, SearchField, Section } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import {
  filterStaff,
  listAllStaff,
  paymentTone,
  type StaffForm,
  staffCreateInput,
  staffForm,
  staffFormErrors,
  type StaffProfile,
  staffPatchInput,
} from './lib';

const staffKey = ['staff', 'all'] as const;

function useStaffList() {
  return useQuery({
    queryKey: staffKey,
    queryFn: () =>
      listAllStaff((limit, offset) => unwrap(api.GET('/v1/staff-profiles', { params: { query: { limit, offset } } }))),
  });
}

/** Create / edit form of a staff card (name, title, hire day, monthly salary, active). */
function StaffFormSheet({
  title,
  saveTitle,
  initial,
  busy,
  onSave,
  onClose,
}: {
  title: string;
  saveTitle: string;
  initial: StaffForm;
  busy: boolean;
  onSave: (v: StaffForm) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation('staff');
  const [v, setV] = useState(initial);
  const [touched, setTouched] = useState(false);
  const errors = staffFormErrors(v);
  const err = (k: (typeof errors)[number]) => (touched && errors.includes(k) ? t(`error.${k}`) : undefined);
  const set = <K extends keyof StaffForm>(k: K, value: StaffForm[K]) => setV((f) => ({ ...f, [k]: value }));

  return (
    <Sheet
      title={title}
      saveTitle={saveTitle}
      busy={busy}
      onClose={onClose}
      onSave={() => {
        setTouched(true);
        if (errors.length === 0) onSave(v);
      }}>
      <Input
        label={t('name')}
        placeholder={t('name_placeholder')}
        value={v.name}
        onChangeText={(x) => set('name', x)}
        maxLength={200}
        error={err('name')}
        autoFocus
      />
      <Input
        label={t('job_title')}
        placeholder={t('job_title_placeholder')}
        value={v.title}
        onChangeText={(x) => set('title', x)}
        maxLength={100}
        error={err('title')}
      />
      <Input
        label={t('hired_on')}
        placeholder={t('hired_on_placeholder')}
        value={v.hired_on}
        onChangeText={(x) => set('hired_on', x)}
        keyboardType="numbers-and-punctuation"
        maxLength={10}
        error={err('hired_on')}
      />
      <Input
        label={t('monthly_salary')}
        placeholder={t('monthly_salary_placeholder')}
        value={v.monthly_salary}
        onChangeText={(x) => set('monthly_salary', x)}
        keyboardType="decimal-pad"
        error={err('monthly_salary')}
        hint={t('monthly_salary_hint')}
      />
      <View className="min-h-12 flex-row items-center justify-between gap-3 rounded-lg border border-border bg-card px-4">
        <Text variant="body">{t('active')}</Text>
        <Switch value={v.active} onValueChange={(x) => set('active', x)} accessibilityLabel={t('active')} />
      </View>
    </Sheet>
  );
}

type ActiveFilter = 'all' | 'active' | 'inactive';

/** Staff cards of the own book (staff.manage): search, state filter and a new card. */
export function StaffListScreen() {
  const { t } = useTranslation('staff');
  const fmt = useFormat();
  const child = useChildHref();
  const qc = useQueryClient();
  const q = useStaffList();
  const [filter, setFilter] = useState<ActiveFilter>('active');
  const [search, setSearch] = useState('');
  const [creating, setCreating] = useState(false);
  const items = filterStaff(q.data ?? [], search, filter);

  const create = useMutation({
    mutationFn: (v: StaffForm) => unwrap(api.POST('/v1/staff-profiles', { body: staffCreateInput(v) })),
    onSuccess: (s) => {
      setCreating(false);
      void qc.invalidateQueries({ queryKey: staffKey });
      router.push(child(s.uuid));
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });

  return (
    <>
      <Stack.Screen
        options={{
          title: t('title'),
          headerRight: () => (
            <Pressable accessibilityRole="button" accessibilityLabel={t('new')} hitSlop={10} onPress={() => setCreating(true)}>
              <Icon name="add" tone="primary" />
            </Pressable>
          ),
        }}
      />
      <FlatList
        data={items}
        keyExtractor={(s) => s.uuid}
        contentInsetAdjustmentBehavior="automatic"
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2.5 px-4 pb-10 pt-2"
        ListHeaderComponent={
          <View className="gap-3 pb-1">
            <SearchField value={search} onChange={setSearch} placeholder={t('search')} />
            <Chips<ActiveFilter>
              value={filter}
              onChange={setFilter}
              options={[
                { value: 'active', label: t('filter_active') },
                { value: 'inactive', label: t('filter_inactive') },
                { value: 'all', label: t('filter_all') },
              ]}
            />
          </View>
        }
        ListEmptyComponent={
          <ListBody
            isPending={q.isPending}
            error={q.error}
            count={items.length}
            onRetry={() => void q.refetch()}
            empty={{
              icon: 'customers',
              title: search || filter !== 'active' ? t('empty_filtered') : t('empty'),
              body: t('empty_body'),
              action: { title: t('new'), onPress: () => setCreating(true) },
            }}
          />
        }
        renderItem={({ item: s }) => (
          <FleetRow
            title={s.name}
            subtitle={s.title ?? undefined}
            meta={[
              s.monthly_salary ? t('salary', { amount: fmt.money(s.monthly_salary, s.currency) }) : t('no_salary'),
              s.hired_on ? t('hired', { date: fmt.date(s.hired_on) }) : null,
            ]
              .filter(Boolean)
              .join(' · ')}
            pill={<StatusPill label={s.active ? t('status_active') : t('status_inactive')} tone={s.active ? 'done' : 'neutral'} />}
            onPress={() => router.push(child(s.uuid))}
          />
        )}
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}
      />
      {creating ? (
        <StaffFormSheet
          title={t('new')}
          saveTitle={t('create')}
          initial={staffForm(null)}
          busy={create.isPending}
          onSave={(v) => create.mutate(v)}
          onClose={() => setCreating(false)}
        />
      ) : null}
    </>
  );
}

/** One staff card: details, edit, deactivate / reactivate and the payment history (payments are made on the web). */
export function StaffDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t } = useTranslation('staff');
  const fmt = useFormat();
  const qc = useQueryClient();
  const list = useStaffList();
  const staff = list.data?.find((s) => s.uuid === uuid);
  const [editing, setEditing] = useState(false);

  const payments = useQuery({
    queryKey: ['staff', uuid, 'payments'],
    queryFn: () =>
      unwrap(api.GET('/v1/staff-profiles/{uuid}/payments', { params: { path: { uuid }, query: { sort: '-paid_on', limit: 20, offset: 0 } } })),
  });

  const update = useMutation({
    mutationFn: (v: StaffForm) => unwrap(api.PATCH('/v1/staff-profiles/{uuid}', { params: { path: { uuid } }, body: staffPatchInput(v) })),
    onSuccess: (s) => {
      setEditing(false);
      qc.setQueryData<StaffProfile[]>(staffKey, (old) => old?.map((x) => (x.uuid === s.uuid ? s : x)));
      void qc.invalidateQueries({ queryKey: staffKey });
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });
  const toggleActive = (s: StaffProfile) => {
    const next = { ...staffForm(s), active: !s.active };
    if (s.active) {
      Alert.alert(t('deactivate_title'), t('deactivate_body', { name: s.name }), [
        { text: t('common:cancel'), style: 'cancel' },
        { text: t('deactivate'), style: 'destructive', onPress: () => update.mutate(next) },
      ]);
    } else {
      update.mutate(next);
    }
  };

  return (
    <>
      <Stack.Screen
        options={{
          title: staff?.name ?? t('staff'),
          headerLargeTitle: false,
          headerRight: staff
            ? () => (
                <Pressable accessibilityRole="button" accessibilityLabel={t('edit')} hitSlop={10} onPress={() => setEditing(true)}>
                  <Icon name="edit" tone="primary" />
                </Pressable>
              )
            : undefined,
        }}
      />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={
          <RefreshControl
            refreshing={list.isRefetching}
            onRefresh={() => {
              void list.refetch();
              void payments.refetch();
            }}
          />
        }>
        {!staff ? (
          list.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={list.error ?? new Error('not found')} onRetry={() => void list.refetch()} />
          )
        ) : (
          <>
            <View className="gap-2 rounded-lg border border-border bg-card p-4">
              <View className="flex-row items-center justify-between gap-3">
                <Text variant="title" className="flex-1" numberOfLines={2}>
                  {staff.name}
                </Text>
                <StatusPill
                  label={staff.active ? t('status_active') : t('status_inactive')}
                  tone={staff.active ? 'done' : 'neutral'}
                />
              </View>
              {staff.title ? (
                <Text variant="callout" tone="muted">
                  {staff.title}
                </Text>
              ) : null}
              <View className="mt-2 flex-row gap-2">
                <Button title={t('edit')} variant="secondary" onPress={() => setEditing(true)} className="flex-1" />
                <Button
                  title={staff.active ? t('deactivate') : t('reactivate')}
                  variant="outline"
                  loading={update.isPending && !editing}
                  disabled={update.isPending}
                  onPress={() => toggleActive(staff)}
                  className="flex-1"
                />
              </View>
            </View>

            <Section title={t('details')}>
              <InfoRow label={t('hired_on')} value={staff.hired_on ? fmt.date(staff.hired_on) : t('none')} />
              <InfoRow
                label={t('monthly_salary')}
                value={staff.monthly_salary ? fmt.money(staff.monthly_salary, staff.currency) : t('no_salary')}
              />
              <InfoRow label={t('linked_user')} value={staff.user_uuid ? t('linked_yes') : t('linked_no')} />
              <InfoRow label={t('created_at')} value={fmt.date(staff.created_at)} last />
            </Section>

            <Section title={t('payments')}>
              {payments.data && payments.data.items.length > 0 ? (
                payments.data.items.map((p, i, all) => (
                  <View key={p.uuid} className={`gap-0.5 px-4 py-2.5 ${i < all.length - 1 ? 'border-b border-border' : ''}`}>
                    <View className="flex-row items-center justify-between gap-3">
                      <Text variant="callout" className="flex-1">
                        {`${t(`payment_type.${p.type}`)} · ${p.period}`}
                      </Text>
                      <Text variant="callout" className="tabular-nums">
                        {fmt.money(p.amount, p.currency)}
                      </Text>
                    </View>
                    <View className="flex-row items-center justify-between gap-3">
                      <Text variant="footnote" tone="muted">
                        {fmt.date(p.paid_on)}
                      </Text>
                      <StatusPill label={t(`payment_status.${p.status}`)} tone={paymentTone(p.status)} />
                    </View>
                  </View>
                ))
              ) : (
                <Text variant="callout" tone="muted" className="px-4 py-3">
                  {payments.isError ? errorText(payments.error) : payments.isPending ? t('common:loading') : t('payments_empty')}
                </Text>
              )}
            </Section>
            <Text variant="footnote" tone="muted">
              {t('payments_web')}
            </Text>
          </>
        )}
      </ScrollView>
      {editing && staff ? (
        <StaffFormSheet
          title={t('edit')}
          saveTitle={t('common:save')}
          initial={staffForm(staff)}
          busy={update.isPending}
          onSave={(v) => update.mutate(v)}
          onClose={() => setEditing(false)}
        />
      ) : null}
    </>
  );
}
