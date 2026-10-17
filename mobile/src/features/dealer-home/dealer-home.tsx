import { useQuery } from '@tanstack/react-query';
import { type Href, router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Pressable, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';
import { activeMembership, useCan } from '@/core/spaces';
import { formatNumber, useLinks } from '@/features/services/lib';
import { ServiceRow } from '@/features/services/services-list-screen';
import { errorText } from '@/features/services/ui';
import { HomeHeaderLeft, SearchButton } from '@/features/shell/header-actions';
import { Button } from '@/ui/button';
import { Gauge, type LucideIcon, Plus, ScanLine, ShieldCheck, Users, Wrench } from '@/ui/icons';
import { Icon } from '@/ui/icon';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

function greetingKey(hour: number) {
  if (hour < 12) return 'home:greeting_morning';
  if (hour < 18) return 'home:greeting_day';
  return 'home:greeting_evening';
}

type Action = { key: string; icon: LucideIcon; href: Href; show: boolean };

/**
 * Dealer home: the last 30 days in numbers (reports overview cards), the
 * latest services and the jobs a dealer starts most.
 */
export function DealerHome() {
  const { t, i18n } = useTranslation('dealer');
  const me = useSession((s) => s.me);
  const org = activeMembership(me);
  const links = useLinks();
  const canWrite = useCan('services.write');
  const canServices = useCan('services.read');
  const canCustomers = useCan('customers.read');
  const canWarranties = useCan('warranties.read');

  const overview = useQuery({
    queryKey: ['dealer-home', 'overview', i18n.language],
    queryFn: () => unwrap(api.GET('/v1/reports/overview', { params: { query: { period: '30d' } } })),
  });
  const recent = useQuery({
    queryKey: ['dealer-home', 'recent'],
    queryFn: () => unwrap(api.GET('/v1/services', { params: { query: { limit: 5, offset: 0 } } })),
    enabled: canServices,
  });

  const cards = overview.data?.series.find((s) => s.key === 'cards')?.points ?? overview.data?.series[0]?.points ?? [];

  const actions: Action[] = [
    { key: 'new_service', icon: Plus, href: links.newService, show: canWrite },
    { key: 'scan', icon: ScanLine, href: '/bayi/scan', show: true },
    { key: 'measure', icon: Gauge, href: '/bayi/measure', show: canWrite },
    { key: 'customers', icon: Users, href: '/bayi/menu/customers' as Href, show: canCustomers },
    { key: 'warranties', icon: ShieldCheck, href: '/bayi/menu/warranties' as Href, show: canWarranties },
    { key: 'services', icon: Wrench, href: '/bayi/services', show: canServices && !canWrite },
  ];

  const refresh = () => {
    void overview.refetch();
    if (canServices) void recent.refetch();
  };

  return (
    <>
      <Stack.Screen
        options={{
          title: t(greetingKey(new Date().getHours()), { name: me?.user.name ?? '' }),
          headerLeft: () => <HomeHeaderLeft />,
          headerRight: () => <SearchButton />,
        }}
      />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<RefreshControl refreshing={overview.isRefetching || recent.isRefetching} onRefresh={refresh} />}>
        {org ? (
          <Text variant="footnote" tone="muted">
            {org.name} · {t(`spaces:org_type.${org.type ?? 'dealer'}`)}
          </Text>
        ) : null}

        <View className="flex-row flex-wrap gap-2.5">
          {actions
            .filter((a) => a.show)
            .map((a) => (
              <Pressable
                key={a.key}
                accessibilityRole="button"
                onPress={() => router.push(a.href)}
                className="min-h-[84px] grow basis-[30%] justify-between rounded-lg border border-border bg-card p-3 active:opacity-80">
                <View className="size-8 items-center justify-center rounded-md bg-secondary">
                  <Icon as={a.icon} tone="primary" size={18} />
                </View>
                <Text variant="callout" className="font-semibold" numberOfLines={1}>
                  {t(`home_action.${a.key}`)}
                </Text>
              </Pressable>
            ))}
        </View>

        {overview.isPending ? (
          <View className="flex-row flex-wrap gap-2.5">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-20 grow basis-[46%] rounded-lg" />
            ))}
          </View>
        ) : cards.length > 0 ? (
          <View className="gap-1.5">
            <Text variant="caption" tone="muted" className="px-1">
              {t('last_30_days')}
            </Text>
            <View className="flex-row flex-wrap gap-2.5">
              {cards.slice(0, 8).map((c) => (
                <View key={c.key} className="min-h-20 grow basis-[46%] justify-between gap-1 rounded-lg border border-border bg-card p-3">
                  <Text variant="footnote" tone="muted" numberOfLines={2}>
                    {c.label}
                  </Text>
                  <Text variant="title" className="tabular-nums">
                    {formatNumber(c.value, i18n.language)}
                  </Text>
                </View>
              ))}
            </View>
          </View>
        ) : null}

        {canServices ? (
          <View className="gap-2">
            <View className="flex-row items-center justify-between px-1">
              <Text variant="caption" tone="muted">
                {t('recent_services')}
              </Text>
              <Pressable accessibilityRole="button" hitSlop={8} onPress={() => router.push('/bayi/services')}>
                <Text variant="footnote" tone="primary" className="font-semibold">
                  {t('see_all')}
                </Text>
              </Pressable>
            </View>
            {recent.isPending ? (
              [0, 1, 2].map((i) => <Skeleton key={i} className="h-[76px] rounded-lg" />)
            ) : recent.error ? (
              <View className="items-center gap-2 rounded-lg border border-border bg-card p-4">
                <Text variant="callout" tone="muted" className="text-center">
                  {errorText(recent.error)}
                </Text>
                <Button title={t('common:retry')} variant="secondary" onPress={() => void recent.refetch()} />
              </View>
            ) : (recent.data?.items.length ?? 0) > 0 ? (
              recent.data!.items.map((s) => (
                <ServiceRow key={s.uuid} s={s} onPress={() => router.push(links.service(s.uuid))} />
              ))
            ) : (
              <View className="items-center gap-2 rounded-lg border border-border bg-card p-5">
                <Icon as={Wrench} tone="muted-foreground" />
                <Text variant="callout" tone="muted" className="text-center">
                  {t('services_empty_body')}
                </Text>
                {canWrite ? <Button title={t('new_service')} onPress={() => router.push(links.newService)} /> : null}
              </View>
            )}
          </View>
        ) : null}
      </ScrollView>
    </>
  );
}
