import { useQuery } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Linking, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useModule } from '@/core/modules';
import { useCan } from '@/core/spaces';
import { formatDateTime, fullName } from '@/features/services/lib';
import { DetailSkeleton, ErrorState, InfoRow, Section } from '@/features/services/ui';
import { PullRefresh } from '@/ui/pull-refresh';
import { Text } from '@/ui/text';

import { RefRow } from './ref-row';
import { RelatedList } from './related-list';

/**
 * Staff user summary (TEC-1219, GET /v1/tenant/users/{uuid}/summary from
 * TEC-1216): who the user is, where they work (organization and warehouses)
 * and what they did. Counters the viewer may not read come as null and are
 * not drawn; the reverse lists sit behind their own permissions.
 */
export function UserSummaryScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('admin');
  const q = useQuery({
    queryKey: ['tenant', 'users', uuid, 'summary'],
    queryFn: () => unwrap(api.GET('/v1/tenant/users/{uuid}/summary', { params: { path: { uuid } } })),
    enabled: !!uuid,
  });
  const canServices = useCan('services.read');
  const ordersOn = useModule('orders');
  const canOrders = useCan('orders.read') && ordersOn;
  const s = q.data;
  const count = (n: number | null | undefined) => (n === null || n === undefined ? null : String(n));

  return (
    <>
      <Stack.Screen options={{ title: s ? fullName(s.user) : t('user.title'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<PullRefresh busy={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {!s ? (
          q.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          )
        ) : (
          <>
            <View className="gap-1">
              <Text variant="title" selectable>
                {fullName(s.user)}
              </Text>
              <Text variant="footnote" tone="muted">
                {t(`user.status.${s.user.status}`, { defaultValue: s.user.status })}
              </Text>
            </View>

            <Section title={t('user.contact')}>
              <InfoRow
                label={t('user.phone')}
                value={s.user.phone_e164}
                onPress={s.user.phone_e164 ? () => void Linking.openURL(`tel:${s.user.phone_e164}`) : undefined}
              />
              <InfoRow
                label={t('user.email')}
                value={s.user.email}
                onPress={s.user.email ? () => void Linking.openURL(`mailto:${encodeURIComponent(s.user.email ?? '')}`) : undefined}
              />
              <InfoRow
                label={t('user.last_login')}
                value={s.user.last_login_at ? formatDateTime(s.user.last_login_at, i18n.language) : t('user.never')}
                last
              />
            </Section>

            {s.memberships.length ? (
              <Section title={t('user.memberships')}>
                {s.memberships.map((m, i) => (
                  <RefRow
                    key={m.organization.uuid}
                    kind="organization"
                    label={t(`user.role.${m.role}`)}
                    value={{
                      uuid: m.organization.uuid,
                      name: m.organization.name,
                      sub: m.warehouses.map((w) => w.name).join(', ') || null,
                    }}
                    last={i === s.memberships.length - 1}
                  />
                ))}
              </Section>
            ) : null}

            <Section title={t('user.activity')}>
              <InfoRow label={t('user.count.services_created')} value={count(s.counts.services_created)} />
              <InfoRow label={t('user.count.services_performed')} value={count(s.counts.services_performed)} />
              <InfoRow label={t('user.count.orders_created')} value={count(s.counts.orders_created)} />
              <InfoRow label={t('user.count.open_tasks')} value={count(s.counts.open_tasks)} />
              <InfoRow label={t('user.count.leads_assigned')} value={count(s.counts.leads_assigned)} last />
            </Section>

            <RelatedList
              kind="service"
              title={t('user.related_services')}
              enabled={canServices}
              queryKey={['services', 'list', { performer: uuid }]}
              fetch={async (limit) => {
                const r = await unwrap(
                  api.GET('/v1/services', { params: { query: { performer_uuid: uuid, sort: '-created_at', limit, offset: 0 } } }),
                );
                return {
                  total: r.total,
                  items: r.items.map((x) => ({
                    uuid: x.uuid,
                    title: [x.plate, x.service_no].filter(Boolean).join(' · '),
                    subtitle: [fullName(x.customer), x.status_label].filter(Boolean).join(' · '),
                  })),
                };
              }}
            />
            <RelatedList
              kind="order"
              title={t('user.related_orders')}
              enabled={canOrders}
              queryKey={['orders', 'list', { created_by: uuid }]}
              fetch={async (limit) => {
                const r = await unwrap(api.GET('/v1/orders', { params: { query: { created_by_uuid: uuid, limit, offset: 0 } } }));
                return {
                  total: r.total,
                  items: r.items.map((x) => ({
                    uuid: x.uuid,
                    title: x.order_no,
                    subtitle: `${x.seller.name} → ${x.buyer.name} · ${x.status_label}`,
                  })),
                };
              }}
            />
          </>
        )}
      </ScrollView>
    </>
  );
}
