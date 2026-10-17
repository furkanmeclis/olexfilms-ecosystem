import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { type Href, router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Alert, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { useCan } from '@/core/spaces';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

import { ErrorState, useFormat } from './kit';

const LIMIT = 20;

function useDecision(onDone: () => void) {
  return useMutation({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onSuccess: onDone,
    onError: (e) => Alert.alert(errorMessage(isApiError(e) ? e.code : undefined)),
  });
}

function Section({
  title,
  query,
  children,
  count,
}: {
  title: string;
  query: { isPending: boolean; isError: boolean; error: unknown; refetch: () => unknown; data?: unknown };
  count?: number;
  children: React.ReactNode;
}) {
  // Not this organization's queue (or the module is off): leave the section out.
  if (query.isError && isApiError(query.error) && (query.error.status === 403 || query.error.status === 404)) return null;
  return (
    <View>
      <SectionHeader title={count ? `${title} · ${count}` : title} />
      {query.isPending ? (
        <Skeleton className="h-16 rounded-lg" />
      ) : query.isError && !query.data ? (
        <ErrorState error={query.error} onRetry={() => void query.refetch()} />
      ) : (
        children
      )}
    </View>
  );
}

function Nothing() {
  const { t } = useTranslation('admin');
  return (
    <Text variant="callout" tone="muted" className="px-4 py-2">
      {t('approvals.none')}
    </Text>
  );
}

/** Orders waiting for the seller's approval (orders.approve). */
function OrderApprovals() {
  const { t } = useTranslation('admin');
  const f = useFormat();
  const q = useQuery({
    queryKey: ['approvals', 'orders'],
    queryFn: () => unwrap(api.GET('/v1/orders', { params: { query: { status: 'submitted', side: 'seller', limit: LIMIT } } })),
  });
  const items = q.data?.items ?? [];
  return (
    <Section title={t('approvals.orders')} query={q} count={q.data?.total}>
      {items.length ? (
        <List>
          {items.map((o, i) => (
            <ListRow
              key={o.uuid}
              title={`${o.order_no} · ${o.buyer.name}`}
              subtitle={`${f.money(o.total, o.currency)} · ${f.date(o.submitted_at ?? o.created_at)}`}
              onPress={() => router.push(`/admin/approvals/orders/${o.uuid}` as Href)}
              last={i === items.length - 1}
            />
          ))}
        </List>
      ) : (
        <Nothing />
      )}
    </Section>
  );
}

/** Dealers' module requests to their distributor (modules.manage). */
function ModuleApprovals() {
  const { t } = useTranslation('admin');
  const f = useFormat();
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ['approvals', 'modules'],
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/modules/requests', { params: { query: { status: ['pending'], limit: LIMIT } } })),
  });
  const decide = useDecision(() => void qc.invalidateQueries({ queryKey: ['approvals', 'modules'] }));
  const items = q.data?.items ?? [];

  const ask = (uuid: string, name: string, approve: boolean) =>
    Alert.alert(t(approve ? 'approvals.confirm_approve' : 'approvals.confirm_reject'), name, [
      { text: t('common:cancel'), style: 'cancel' },
      {
        text: t(approve ? 'approvals.approve' : 'approvals.reject'),
        style: approve ? 'default' : 'destructive',
        onPress: () =>
          decide.mutate(() =>
            approve
              ? unwrap(api.POST('/v1/tenant/modules/requests/{uuid}/approve', { params: { path: { uuid } }, body: {} }))
              : unwrap(api.POST('/v1/tenant/modules/requests/{uuid}/reject', { params: { path: { uuid } }, body: {} })),
          ),
      },
    ]);

  return (
    <Section title={t('approvals.modules')} query={q} count={q.data?.total}>
      {items.length ? (
        <View className="gap-2.5">
          {items.map((r) => (
            <View key={r.uuid} className="gap-2 rounded-lg border border-border bg-card px-4 py-3">
              <Text variant="headline">{r.organization_name}</Text>
              <Text variant="footnote" tone="muted">
                {t('approvals.module_line', { module: r.module_key, name: r.requested_by_name || '–' })} · {f.date(r.created_at)}
              </Text>
              {r.note ? <Text variant="callout">{r.note}</Text> : null}
              <View className="flex-row gap-2">
                <Button className="flex-1" title={t('approvals.approve')} disabled={decide.isPending} onPress={() => ask(r.uuid, r.organization_name, true)} />
                <Button className="flex-1" variant="outline" title={t('approvals.reject')} disabled={decide.isPending} onPress={() => ask(r.uuid, r.organization_name, false)} />
              </View>
            </View>
          ))}
        </View>
      ) : (
        <Nothing />
      )}
    </Section>
  );
}

/** Stock reclassification requests (stock.reclassify; approving may ask for step-up on the web). */
function ReclassApprovals() {
  const { t } = useTranslation('admin');
  const f = useFormat();
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ['approvals', 'reclassifications'],
    queryFn: () =>
      unwrap(api.GET('/v1/stock/reclassifications', { params: { query: { status: ['pending'], limit: LIMIT } } })),
  });
  const decide = useDecision(() => void qc.invalidateQueries({ queryKey: ['approvals', 'reclassifications'] }));
  const items = q.data?.items ?? [];

  const ask = (uuid: string, barcode: string, approve: boolean) =>
    Alert.alert(t(approve ? 'approvals.confirm_approve' : 'approvals.confirm_reject'), barcode, [
      { text: t('common:cancel'), style: 'cancel' },
      {
        text: t(approve ? 'approvals.approve' : 'approvals.reject'),
        style: approve ? 'default' : 'destructive',
        onPress: () =>
          decide.mutate(() =>
            approve
              ? unwrap(api.POST('/v1/stock/reclassifications/{uuid}/approve', { params: { path: { uuid } }, body: {} }))
              : unwrap(api.POST('/v1/stock/reclassifications/{uuid}/reject', { params: { path: { uuid } }, body: {} })),
          ),
      },
    ]);

  return (
    <Section title={t('approvals.reclassifications')} query={q} count={q.data?.total}>
      {items.length ? (
        <View className="gap-2.5">
          {items.map((r) => (
            <View key={r.uuid} className="gap-2 rounded-lg border border-border bg-card px-4 py-3">
              <Text variant="headline" selectable>
                {r.unit.barcode}
              </Text>
              <Text variant="callout">
                {r.from_product.name} → {r.to_product.name}
              </Text>
              <Text variant="footnote" tone="muted">
                {r.organization.name} · {r.requested_by?.name ?? '–'} · {f.date(r.created_at)}
              </Text>
              {r.reason ? <Text variant="callout">{r.reason}</Text> : null}
              <View className="flex-row gap-2">
                <Button className="flex-1" title={t('approvals.approve')} disabled={decide.isPending} onPress={() => ask(r.uuid, r.unit.barcode, true)} />
                <Button className="flex-1" variant="outline" title={t('approvals.reject')} disabled={decide.isPending} onPress={() => ask(r.uuid, r.unit.barcode, false)} />
              </View>
            </View>
          ))}
        </View>
      ) : (
        <Nothing />
      )}
    </Section>
  );
}

/**
 * Pending decisions of the admin: orders to approve, dealers' module requests
 * and stock reclassifications. Each section shows only with its permission.
 */
export function ApprovalsScreen() {
  const { t } = useTranslation('admin');
  const qc = useQueryClient();
  const orders = useCan('orders.approve');
  const modules = useCan('modules.manage');
  const reclass = useCan('stock.reclassify');
  const any = orders || modules || reclass;

  return (
    <>
      <Stack.Screen options={{ title: t('approvals.title') }} />
      <Screen
        refreshControl={
          <RefreshControl refreshing={false} onRefresh={() => void qc.invalidateQueries({ queryKey: ['approvals'] })} />
        }>
        {any ? (
          <>
            {orders ? <OrderApprovals /> : null}
            {modules ? <ModuleApprovals /> : null}
            {reclass ? <ReclassApprovals /> : null}
          </>
        ) : (
          <EmptyState icon="warrantyClaim" title={t('approvals.no_access')} body={t('approvals.no_access_body')} />
        )}
      </Screen>
    </>
  );
}
