import { useQuery } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import type { paths } from '@/core/api/schema';
import { useCan } from '@/core/spaces';
import { CardRow, Chips, PagedList, PAGE_SIZE, useDebounced, useFormat, usePagedQuery } from '@/features/admin/kit';
import { EmptyState } from '@/ui/empty-state';
import { Input } from '@/ui/input';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { DEFAULT_THRESHOLD, deviationTone, formatDeviation } from './lib';

type Item<P extends keyof paths> = paths[P] extends {
  get: { responses: { 200: { content: { 'application/json': { data: { items: (infer I)[] } } } } } };
}
  ? I
  : never;
type Recommended = Item<'/v1/tenant/pricing/recommended/current'>;
type Discipline = Item<'/v1/pricing/discipline'>;

/**
 * `/v1/tenant/pricing/recommended/current` is not in the mobile panel mirror
 * allowlist yet (it answers 404), so the recommended prices tab stays off.
 * Turn this on once the backend allowlist has `tenant/pricing`
 * (waiting for the product owner's approval).
 */
const MIRROR_READY = false;

type Tab = 'recommended' | 'discipline';
type Over = 'all' | 'over';

function SearchBox({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { t } = useTranslation('governance');
  return (
    <View className="px-4 pb-1 pt-2">
      <Input
        label={t('pricing.search')}
        placeholder={t('pricing.search_placeholder')}
        value={value}
        onChangeText={onChange}
        autoCapitalize="none"
        autoCorrect={false}
        returnKeyType="search"
      />
    </View>
  );
}

function RecommendedList({ header }: { header: React.ReactElement }) {
  const { t } = useTranslation('governance');
  const f = useFormat();
  const [q, setQ] = useState('');
  const term = useDebounced(q.trim());
  const list = usePagedQuery<Recommended>(['pricing', 'recommended', term], (offset) =>
    unwrap(
      api.GET('/v1/tenant/pricing/recommended/current', {
        params: { query: { limit: PAGE_SIZE, offset, q: term || undefined, active: true } },
      }),
    ),
  );
  return (
    <PagedList
      query={list}
      keyOf={(r) => `${r.product_uuid}:${r.country_iso2}:${r.currency}`}
      header={
        <View>
          {header}
          <SearchBox value={q} onChange={setQ} />
        </View>
      }
      empty={{ icon: 'accounting', title: t('pricing.recommended_empty') }}
      renderItem={({ item }) => (
        <CardRow
          title={item.product_name}
          subtitle={`${item.product_sku} · ${item.country_iso2}`}
          meta={t('pricing.effective_from', { date: f.date(item.effective_from) })}
          trailing={
            <Text variant="headline" className="tabular-nums">
              {f.money(item.price, item.currency)}
            </Text>
          }
        />
      )}
    />
  );
}

function DisciplineList({ header }: { header: React.ReactElement }) {
  const { t } = useTranslation('governance');
  const f = useFormat();
  const [over, setOver] = useState<Over>('over');
  const [q, setQ] = useState('');
  const term = useDebounced(q.trim());
  const summary = useQuery({
    queryKey: ['pricing', 'discipline', 'summary'],
    queryFn: () => unwrap(api.GET('/v1/pricing/discipline/summary', { params: { query: {} } })),
  });
  const threshold = summary.data?.threshold_pct ?? DEFAULT_THRESHOLD;
  const list = usePagedQuery<Discipline>(['pricing', 'discipline', over, term], (offset) =>
    unwrap(
      api.GET('/v1/pricing/discipline', {
        params: { query: { limit: PAGE_SIZE, offset, q: term || undefined, over_threshold: over === 'over' ? true : undefined } },
      }),
    ),
  );
  return (
    <PagedList
      query={list}
      keyOf={(r) => `${r.organization_uuid}:${r.product_uuid}:${r.snapshot_date}`}
      header={
        <View>
          {header}
          <Chips
            options={[
              { key: 'over', label: t('pricing.over_threshold', { pct: threshold }) },
              { key: 'all', label: t('pricing.all') },
            ]}
            value={over}
            onChange={setOver}
          />
          {summary.data?.snapshot_date ? (
            <Text variant="footnote" tone="muted" className="px-4 pb-1">
              {t('pricing.snapshot', { date: f.date(summary.data.snapshot_date) })}
            </Text>
          ) : null}
          <SearchBox value={q} onChange={setQ} />
        </View>
      }
      empty={{ icon: 'check', title: t(over === 'over' ? 'pricing.discipline_clean' : 'pricing.discipline_empty') }}
      renderItem={({ item }) => (
        <View className="mx-4 gap-1.5 rounded-lg border border-border bg-card px-4 py-3">
          <View className="flex-row items-start justify-between gap-3">
            <View className="flex-1 gap-0.5">
              <Text variant="headline" numberOfLines={1}>
                {item.organization_name}
              </Text>
              <Text variant="callout" numberOfLines={2}>
                {item.product_name}
              </Text>
            </View>
            <StatusPill label={formatDeviation(item.deviation_pct, f.locale)} tone={deviationTone(item.deviation_pct, threshold)} />
          </View>
          <View className="flex-row justify-between">
            <Text variant="footnote" tone="muted">
              {t('pricing.list_price')}
            </Text>
            <Text variant="footnote" className="tabular-nums">
              {f.money(item.list_price, item.currency)}
            </Text>
          </View>
          <View className="flex-row justify-between">
            <Text variant="footnote" tone="muted">
              {t('pricing.recommended_price')}
            </Text>
            <Text variant="footnote" className="tabular-nums">
              {f.money(item.recommended_price, item.currency)}
            </Text>
          </View>
          <View className="flex-row justify-between">
            <Text variant="footnote" tone="muted">
              {t('pricing.avg_sale')}
            </Text>
            <Text variant="footnote" className="tabular-nums">
              {item.avg_sale_price ? f.money(item.avg_sale_price, item.currency) : '–'}
            </Text>
          </View>
        </View>
      )}
    />
  );
}

/**
 * Pricing governance: the recommended price list in force and the price
 * discipline snapshot (list price against the recommended one). Read only;
 * publishing and importing stay on the web.
 */
export function PricingScreen() {
  const { t } = useTranslation('governance');
  const canRecommended = useCan('pricing.recommended.read') && MIRROR_READY;
  const canDiscipline = useCan('pricing.discipline.read');
  const [tab, setTab] = useState<Tab>('recommended');
  const tabs = [
    ...(canRecommended ? [{ key: 'recommended' as const, label: t('pricing.tab_recommended') }] : []),
    ...(canDiscipline ? [{ key: 'discipline' as const, label: t('pricing.tab_discipline') }] : []),
  ];
  const active: Tab = tab === 'discipline' && canDiscipline ? 'discipline' : canRecommended ? 'recommended' : 'discipline';
  const header = tabs.length > 1 ? <Chips options={tabs} value={active} onChange={setTab} /> : <View />;

  return (
    <>
      <Stack.Screen options={{ title: t('pricing.title') }} />
      {tabs.length === 0 ? (
        <EmptyState icon="blocked" title={t('no_access')} />
      ) : active === 'recommended' ? (
        <RecommendedList header={header} />
      ) : (
        <DisciplineList header={header} />
      )}
    </>
  );
}
