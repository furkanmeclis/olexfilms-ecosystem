import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { type Href, router, Stack, useLocalSearchParams, useSegments } from 'expo-router';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useCan } from '@/core/spaces';
import { useChildHref, useFormat } from '@/features/admin/kit';
import { FleetList, FleetReportRow, FleetRow, FleetVehicleRow } from '@/features/customer-space/fleet-screens';
import { fleetLinkTone } from '@/features/customer-space/fleet-lib';
import { PAGE_SIZE, useDebounced } from '@/features/services/lib';
import { Sheet } from '@/features/services/sheet';
import { Chips, DetailSkeleton, ErrorState, errorText, InfoRow, SearchField, Section } from '@/features/services/ui';
import { StatTile } from '@/features/customer-space/parts';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { List, ListRow } from '@/ui/list';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import {
  closedPeriods,
  FLEET_LINK_FILTERS,
  type FleetLinkFilter,
  type FleetListItem,
  type FleetReportKind,
  fleetReportTone,
  fleetUserTone,
  inviteErrors,
  monthOf,
  monthRange,
  type PanelFleetReport,
  type PanelFleetUser,
  type PanelFleetVehicle,
  shiftMonth,
} from './lib';

const cardKey = (uuid: string) => ['fleets', 'card', uuid] as const;

function useFleetUuid() {
  return useLocalSearchParams<{ uuid: string }>().uuid;
}

/** Panel routes of the current space (`/<space>/menu/...`) for customer and service detail. */
function useSpaceLinks() {
  const space = (useSegments() as string[])[1] ?? 'bayi';
  return {
    customer: (uuid: string) => `/${space}/menu/customers/${encodeURIComponent(uuid)}` as Href,
    service: (uuid: string) => `/${space}/menu/customers/services/${encodeURIComponent(uuid)}` as Href,
  };
}

function AddButton({ label, onPress }: { label: string; onPress: () => void }) {
  return (
    <Pressable accessibilityRole="button" accessibilityLabel={label} hitSlop={10} onPress={onPress}>
      <Icon name="add" tone="primary" />
    </Pressable>
  );
}

/** Fleets linked to the organizations in scope (one row per dealer link): search and link status filter. */
export function FleetsListScreen() {
  const { t } = useTranslation('fleets');
  const { t: tc } = useTranslation('customer');
  const fmt = useFormat();
  const child = useChildHref();
  const [filter, setFilter] = useState<FleetLinkFilter>('all');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  return (
    <FleetList<FleetListItem & { fleetUuid: string }>
      title={t('title')}
      queryKey={['fleets', 'list', { filter, q: query }]}
      fetchPage={async (offset) => {
        const page = await unwrap(
          api.GET('/v1/fleets', {
            params: {
              query: {
                q: query || undefined,
                status: filter === 'all' ? undefined : [filter],
                limit: PAGE_SIZE,
                offset,
              },
            },
          }),
        );
        // One row per link: the same fleet may appear for two dealers of the scope.
        return { ...page, items: page.items.map((f) => ({ ...f, uuid: `${f.uuid}:${f.link.uuid}`, fleetUuid: f.uuid })) };
      }}
      header={
        <View className="gap-3 pb-1">
          <SearchField value={q} onChange={setQ} placeholder={t('search')} />
          <Chips<FleetLinkFilter>
            value={filter}
            onChange={setFilter}
            options={FLEET_LINK_FILTERS.map((f) => ({
              value: f,
              label: f === 'all' ? t('filter_all') : tc(`fleet_link_status.${f}`),
            }))}
          />
        </View>
      }
      empty={{ icon: 'truck', title: query || filter !== 'all' ? t('empty_filtered') : t('empty'), body: t('empty_body') }}
      renderItem={(f) => (
        <FleetRow
          title={f.name}
          subtitle={f.legal_name !== f.name ? f.legal_name : undefined}
          meta={[
            t('vehicle_count', { count: f.vehicle_count }),
            f.link.dealer_name,
            f.last_service_at ? t('last_service', { date: fmt.date(f.last_service_at) }) : null,
          ]
            .filter(Boolean)
            .join(' · ')}
          pill={<StatusPill label={tc(`fleet_link_status.${f.link.status}`)} tone={fleetLinkTone(f.link.status)} />}
          onPress={() => router.push(child(f.fleetUuid))}
        />
      )}
    />
  );
}

/** Fleet card: profile, figures, cari balance, dealer links, recent services and the sub screens. */
export function FleetCardScreen() {
  const uuid = useFleetUuid();
  const { t } = useTranslation('fleets');
  const { t: tc } = useTranslation('customer');
  const fmt = useFormat();
  const child = useChildHref();
  const links = useSpaceLinks();
  const q = useQuery({
    queryKey: cardKey(uuid),
    queryFn: () => unwrap(api.GET('/v1/fleets/{uuid}', { params: { path: { uuid } } })),
  });
  const fleet = q.data;
  // The fleet's primary user owns the fleet vehicles: its customer card lists the services (web: services tab).
  const primary = useQuery({
    queryKey: ['fleets', uuid, 'primary-user'],
    enabled: !!fleet?.has_primary_user,
    queryFn: async () => {
      const page = await unwrap(
        api.GET('/v1/fleets/{uuid}/users', { params: { path: { uuid }, query: { limit: 100, offset: 0, sort: 'created_at' } } }),
      );
      return page.items.find((u) => u.is_primary) ?? null;
    },
  });

  const entries: { key: string; icon: 'truck' | 'customers' | 'accounting' | 'report'; title: string }[] = [
    { key: 'vehicles', icon: 'truck', title: t('vehicles') },
    { key: 'users', icon: 'customers', title: t('users') },
    { key: 'statement', icon: 'accounting', title: t('statement') },
    { key: 'reports', icon: 'report', title: t('reports') },
  ];

  return (
    <>
      <Stack.Screen options={{ title: fleet?.name ?? t('fleet'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {!fleet ? (
          q.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          )
        ) : (
          <>
            <View className="flex-row flex-wrap gap-2.5">
              <StatTile icon="truck" label={tc('fleet_vehicles')} value={fmt.number(fleet.vehicle_count)} />
              <StatTile icon="warranty" label={tc('active_warranties')} value={fmt.number(fleet.active_warranty_count)} />
              <StatTile icon="service" label={t('service_total')} value={fmt.number(fleet.service_count)} />
              {fleet.cari ? (
                <StatTile icon="accounting" label={t('balance')} value={fmt.money(fleet.cari.balance, fleet.cari.currency)} />
              ) : null}
            </View>

            <List>
              {entries.map((e, i) => (
                <ListRow
                  key={e.key}
                  icon={e.icon}
                  title={e.title}
                  onPress={() => router.push(child(e.key))}
                  last={i === entries.length - 1 && !primary.data}
                />
              ))}
              {primary.data ? (
                <ListRow
                  icon="customer"
                  title={t('customer_card')}
                  subtitle={t('customer_card_body')}
                  onPress={() => router.push(links.customer(primary.data!.user_uuid))}
                  last
                />
              ) : null}
            </List>

            <Section title={t('profile')}>
              <InfoRow label={t('legal_name')} value={fleet.profile.legal_name} />
              <InfoRow label={t('tax_number')} value={fleet.profile.tax_number} mono />
              <InfoRow label={t('tax_office')} value={fleet.profile.tax_office} />
              <InfoRow label={t('contact_name')} value={fleet.profile.contact_name} />
              <InfoRow label={t('contact_phone')} value={fleet.profile.contact_phone} />
              <InfoRow label={t('billing_email')} value={fleet.profile.billing_email} />
              <InfoRow label={t('report_frequency')} value={t(`report_frequency_value.${fleet.profile.report_frequency}`)} last />
            </Section>

            {fleet.links.length > 0 ? (
              <Section title={t('links')}>
                {fleet.links.map((l, i) => (
                  <ListRow
                    key={l.uuid}
                    title={l.dealer_name}
                    subtitle={fmt.date(l.started_at ?? l.created_at)}
                    trailing={<StatusPill label={tc(`fleet_link_status.${l.status}`)} tone={fleetLinkTone(l.status)} />}
                    last={i === fleet.links.length - 1}
                  />
                ))}
              </Section>
            ) : null}

            <Section title={t('recent_services')}>
              {fleet.recent_services.length === 0 ? (
                <Text variant="callout" tone="muted" className="px-4 py-3">
                  {t('recent_services_empty')}
                </Text>
              ) : (
                fleet.recent_services.map((s, i) => (
                  <ListRow
                    key={s.uuid}
                    icon="service"
                    title={[s.plate, s.service_no].filter(Boolean).join(' · ')}
                    subtitle={[s.organization_name, fmt.date(s.completed_at ?? s.created_at)].join(' · ')}
                    onPress={() => router.push(links.service(s.uuid))}
                    last={i === fleet.recent_services.length - 1}
                  />
                ))
              )}
            </Section>
          </>
        )}
      </ScrollView>
    </>
  );
}

/** Vehicles of a fleet; with fleets.manage a vehicle can be taken out of the fleet. */
export function FleetVehiclesScreen() {
  const uuid = useFleetUuid();
  const { t } = useTranslation('fleets');
  const fmt = useFormat();
  const qc = useQueryClient();
  const canManage = useCan('fleets.manage');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  const remove = useMutation({
    mutationFn: (vehicle: string) =>
      unwrap(api.DELETE('/v1/fleets/{uuid}/vehicles/{vehicle_uuid}', { params: { path: { uuid, vehicle_uuid: vehicle } } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['fleets', uuid, 'vehicles'] });
      void qc.invalidateQueries({ queryKey: cardKey(uuid) });
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });
  const confirmRemove = (v: PanelFleetVehicle) =>
    Alert.alert(t('vehicle_remove_title'), v.plate ?? undefined, [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t('vehicle_remove'), style: 'destructive', onPress: () => remove.mutate(v.uuid) },
    ]);

  return (
    <FleetList<PanelFleetVehicle>
      title={t('vehicles')}
      queryKey={['fleets', uuid, 'vehicles', { q: query }]}
      fetchPage={(offset) =>
        unwrap(
          api.GET('/v1/fleets/{uuid}/vehicles', {
            params: { path: { uuid }, query: { q: query || undefined, limit: PAGE_SIZE, offset } },
          }),
        )
      }
      header={
        <View className="gap-2 pb-1">
          <SearchField value={q} onChange={setQ} placeholder={t('vehicles_search')} />
          {canManage ? (
            <Text variant="footnote" tone="muted">
              {t('vehicles_add_web')}
            </Text>
          ) : null}
        </View>
      }
      empty={{ icon: 'truck', title: t('vehicles_empty') }}
      renderItem={(v) => (
        <FleetVehicleRow
          vehicle={v}
          meta={[v.vin, v.last_service_at ? t('last_service', { date: fmt.date(v.last_service_at) }) : null]
            .filter(Boolean)
            .join(' · ')}>
          {canManage ? (
            <View className="mt-1 flex-row">
              <Button
                title={t('vehicle_remove')}
                variant="outline"
                loading={remove.isPending && remove.variables === v.uuid}
                disabled={remove.isPending}
                onPress={() => confirmRemove(v)}
              />
            </View>
          ) : null}
        </FleetVehicleRow>
      )}
    />
  );
}

/** Portal users of a fleet: invite (fleets.manage) and disable. */
export function FleetUsersScreen() {
  const uuid = useFleetUuid();
  const { t } = useTranslation('fleets');
  const fmt = useFormat();
  const qc = useQueryClient();
  const canManage = useCan('fleets.manage');
  const [inviteOpen, setInviteOpen] = useState(false);
  const [form, setForm] = useState({ email: '', name: '', surname: '' });
  const errors = inviteErrors(form);

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ['fleets', uuid, 'users'] });
    void qc.invalidateQueries({ queryKey: ['fleets', uuid, 'primary-user'] });
    void qc.invalidateQueries({ queryKey: cardKey(uuid) });
  };
  const invite = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST('/v1/fleets/{uuid}/users', {
          params: { path: { uuid } },
          body: { email: form.email.trim(), name: form.name.trim(), surname: form.surname.trim() || undefined },
        }),
      ),
    onSuccess: () => {
      setInviteOpen(false);
      setForm({ email: '', name: '', surname: '' });
      refresh();
      Alert.alert(t('invite_sent'), t('invite_sent_body'));
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });
  const disable = useMutation({
    mutationFn: (user: string) =>
      unwrap(api.POST('/v1/fleets/{uuid}/users/{user_uuid}/disable', { params: { path: { uuid, user_uuid: user } } })),
    onSuccess: refresh,
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });
  const confirmDisable = (u: PanelFleetUser) =>
    Alert.alert(t('user_disable_title'), [u.name, u.surname].filter(Boolean).join(' '), [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t('user_disable'), style: 'destructive', onPress: () => disable.mutate(u.uuid) },
    ]);

  return (
    <>
      <FleetList<PanelFleetUser>
        title={t('users')}
        queryKey={['fleets', uuid, 'users', 'list']}
        fetchPage={(offset) =>
          unwrap(api.GET('/v1/fleets/{uuid}/users', { params: { path: { uuid }, query: { sort: 'created_at', limit: PAGE_SIZE, offset } } }))
        }
        headerRight={canManage ? () => <AddButton label={t('invite')} onPress={() => setInviteOpen(true)} /> : undefined}
        empty={{ icon: 'customers', title: t('users_empty'), body: t('users_empty_body') }}
        renderItem={(u) => (
          <FleetRow
            title={[u.name, u.surname].filter(Boolean).join(' ')}
            subtitle={u.email ?? undefined}
            meta={[
              u.is_primary ? t('user_primary') : null,
              u.last_login_at ? t('last_login', { date: fmt.dateTime(u.last_login_at) }) : t('never_logged_in'),
            ]
              .filter(Boolean)
              .join(' · ')}
            pill={<StatusPill label={t(`user_status.${u.status}`)} tone={fleetUserTone(u.status)} />}>
            {canManage && u.status === 'active' && !u.is_primary ? (
              <View className="mt-1 flex-row">
                <Button
                  title={t('user_disable')}
                  variant="outline"
                  loading={disable.isPending && disable.variables === u.uuid}
                  disabled={disable.isPending}
                  onPress={() => confirmDisable(u)}
                />
              </View>
            ) : null}
          </FleetRow>
        )}
      />
      {inviteOpen ? (
        <Sheet
          title={t('invite')}
          saveTitle={t('invite_send')}
          busy={invite.isPending}
          canSave={errors.length === 0}
          onClose={() => setInviteOpen(false)}
          onSave={() => invite.mutate()}>
          <Text variant="callout" tone="muted">
            {t('invite_body')}
          </Text>
          <Input
            label={t('email')}
            placeholder={t('email_placeholder')}
            value={form.email}
            onChangeText={(email) => setForm((f) => ({ ...f, email }))}
            keyboardType="email-address"
            autoCapitalize="none"
            autoCorrect={false}
            autoFocus
          />
          <Input
            label={t('name')}
            placeholder={t('name_placeholder')}
            value={form.name}
            onChangeText={(name) => setForm((f) => ({ ...f, name }))}
            maxLength={100}
          />
          <Input
            label={t('surname')}
            placeholder={t('surname_placeholder')}
            value={form.surname}
            onChangeText={(surname) => setForm((f) => ({ ...f, surname }))}
            maxLength={100}
          />
        </Sheet>
      ) : null}
    </>
  );
}

/** Periodic reports: list with state; fleets.manage requests one for a closed period. The PDF opens on the web. */
export function FleetReportsScreen() {
  const uuid = useFleetUuid();
  const { t } = useTranslation('fleets');
  const qc = useQueryClient();
  const canManage = useCan('fleets.manage');
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<FleetReportKind>('monthly');
  const periods = useMemo(() => closedPeriods(kind, new Date()), [kind]);
  const [period, setPeriod] = useState<string>('');
  const chosen = periods.includes(period) ? period : periods[0]!;

  const request = useMutation({
    mutationFn: () =>
      unwrap(api.POST('/v1/fleets/{uuid}/reports', { params: { path: { uuid } }, body: { period_kind: kind, period: chosen } })),
    onSuccess: () => {
      setOpen(false);
      void qc.invalidateQueries({ queryKey: ['fleets', uuid, 'reports'] });
      Alert.alert(t('report_requested'), t('report_requested_body'));
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });

  return (
    <>
      <FleetList<PanelFleetReport>
        title={t('reports')}
        queryKey={['fleets', uuid, 'reports', 'list']}
        fetchPage={(offset) =>
          unwrap(api.GET('/v1/fleets/{uuid}/reports', { params: { path: { uuid }, query: { sort: '-period_start', limit: PAGE_SIZE, offset } } }))
        }
        headerRight={canManage ? () => <AddButton label={t('report_request')} onPress={() => setOpen(true)} /> : undefined}
        header={
          <Text variant="footnote" tone="muted" className="pb-1">
            {t('reports_web')}
          </Text>
        }
        empty={{ icon: 'report', title: t('reports_empty') }}
        renderItem={(r) => (
          <FleetReportRow report={r} pill={<StatusPill label={t(`report_status.${r.status}`)} tone={fleetReportTone(r.status)} />} />
        )}
      />
      {open ? (
        <Sheet
          title={t('report_request')}
          saveTitle={t('report_request_send')}
          busy={request.isPending}
          onClose={() => setOpen(false)}
          onSave={() => request.mutate()}>
          <Chips<FleetReportKind>
            value={kind}
            onChange={(k) => {
              setKind(k);
              setPeriod('');
            }}
            options={[
              { value: 'monthly', label: t('report_kind.monthly') },
              { value: 'quarterly', label: t('report_kind.quarterly') },
            ]}
          />
          <Text variant="footnote" tone="muted">
            {t('report_period')}
          </Text>
          <Chips<string> value={chosen} onChange={setPeriod} options={periods.map((p) => ({ value: p, label: p }))} />
        </Sheet>
      ) : null}
    </>
  );
}

/** Cari statement of a month (opening, income, collections, closing and the lines). Export stays on the web. */
export function FleetStatementScreen() {
  const uuid = useFleetUuid();
  const { t } = useTranslation('fleets');
  const fmt = useFormat();
  const [month, setMonth] = useState(() => monthOf(new Date()));
  const current = monthOf(new Date());
  const range = monthRange(month, new Date());
  const q = useQuery({
    queryKey: ['fleets', uuid, 'statement', range.from, range.to],
    queryFn: () =>
      unwrap(api.GET('/v1/fleets/{uuid}/statement', { params: { path: { uuid }, query: { period_from: range.from, period_to: range.to } } })),
  });
  const s = q.data;
  const money = (v: string) => (s ? fmt.money(v, s.currency) : '–');

  return (
    <>
      <Stack.Screen options={{ title: t('statement'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        <View className="flex-row items-center justify-between gap-3">
          <Pressable accessibilityRole="button" accessibilityLabel={t('month_prev')} hitSlop={10} onPress={() => setMonth(shiftMonth(month, -1))}>
            <Icon name="back" tone="primary" />
          </Pressable>
          <Text variant="headline">{`${fmt.date(range.from)} – ${fmt.date(range.to)}`}</Text>
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t('month_next')}
            accessibilityState={{ disabled: month >= current }}
            disabled={month >= current}
            hitSlop={10}
            onPress={() => setMonth(shiftMonth(month, 1))}
            className={month >= current ? 'opacity-30' : undefined}>
            <Icon name="forward" tone="primary" />
          </Pressable>
        </View>
        {!s ? (
          q.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          )
        ) : (
          <>
            <Section title={t('statement_totals')}>
              <InfoRow label={t('opening_balance')} value={money(s.opening_balance)} />
              <InfoRow label={t('service_income')} value={money(s.service_income_total)} />
              <InfoRow label={t('collections')} value={money(s.collection_total)} />
              <InfoRow label={t('closing_balance')} value={money(s.closing_balance)} />
              <InfoRow label={t('service_total')} value={fmt.number(s.service_count)} last />
            </Section>
            <Section title={t('statement_lines')}>
              {s.lines.length === 0 ? (
                <Text variant="callout" tone="muted" className="px-4 py-3">
                  {t('statement_empty')}
                </Text>
              ) : (
                s.lines.map((l, i) => (
                  <View key={l.uuid} className={`gap-0.5 px-4 py-2.5 ${i < s.lines.length - 1 ? 'border-b border-border' : ''}`}>
                    <View className="flex-row items-center justify-between gap-3">
                      <Text variant="callout" className="flex-1" numberOfLines={1}>
                        {l.service ? [l.service.plate, l.service.service_no].filter(Boolean).join(' · ') : t(`line_kind.${l.kind}`)}
                      </Text>
                      <Text variant="callout" className="tabular-nums">
                        {Number(l.debit) > 0 ? money(l.debit) : `− ${money(l.credit)}`}
                      </Text>
                    </View>
                    <Text variant="footnote" tone="muted" numberOfLines={1}>
                      {[fmt.date(l.date), l.description, t('balance_after', { amount: money(l.balance) })].filter(Boolean).join(' · ')}
                    </Text>
                  </View>
                ))
              )}
            </Section>
            <Text variant="footnote" tone="muted">
              {t('statement_web')}
            </Text>
          </>
        )}
      </ScrollView>
    </>
  );
}
