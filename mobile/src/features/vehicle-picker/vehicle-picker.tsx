import { FlashList } from '@shopify/flash-list';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Modal, Platform, Pressable, ScrollView, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { BrandMark } from '@/features/customer-space/home-parts';
import { useDebounced } from '@/features/services/lib';
import { errorText, SearchField } from '@/features/services/ui';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

import {
  type BrandChoice,
  buildRows,
  CATALOG_SORT,
  catalogQuery,
  EMPTY_VEHICLE,
  frequentList,
  type ModelChoice,
  pickBrand,
  pickModel,
  PICKER_STEPS,
  type PickerRow,
  type PickerStep,
  previousStep,
  reachableSteps,
  startStep,
  type VehicleValue,
  vehicleTitle,
  withSortFallback,
  yearOptions,
  yearSpan,
} from './picker';

type CatalogBrand = components['schemas']['VehicleBrand'];
type CatalogModel = components['schemas']['VehicleModel'];
type FrequentBrand = components['schemas']['FrequentCarBrand'];
type FrequentModel = components['schemas']['FrequentCarModel'];

const PAGE = 100;
const CATALOG_STALE = 30 * 60_000;

export type VehiclePickerProps = {
  value: VehicleValue | null;
  onChange: (v: VehicleValue) => void;
  /** Field label; default "Vehicle". */
  label?: string;
  error?: string;
  /** List the most used brands / models first (organization + brand usage, dealer side). Default: on. */
  frequent?: boolean;
  disabled?: boolean;
};

/**
 * Vehicle field: brand logo + "Brand Model · Year" summary; tapping it opens a
 * step-by-step sheet (brand → model → year) with search, back navigation and
 * a "Frequently used" section. The one vehicle picker of the app.
 */
export function VehiclePicker({ value, onChange, label, error, frequent = true, disabled }: VehiclePickerProps) {
  const { t } = useTranslation('common');
  const [open, setOpen] = useState(false);
  const title = vehicleTitle(value);
  const sub = value?.model ? (value.year ? String(value.year) : t('vehicle_picker.year_none')) : '';

  return (
    <View className="gap-1.5">
      <Text variant="footnote" tone="muted" className="font-medium">
        {label ?? t('vehicle_picker.label')}
      </Text>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={[label ?? t('vehicle_picker.label'), title, sub].filter(Boolean).join(', ')}
        accessibilityHint={t('vehicle_picker.placeholder')}
        disabled={disabled}
        onPress={() => setOpen(true)}
        className={cn(
          'min-h-[60px] flex-row items-center gap-3 rounded-lg border bg-card px-3 py-2.5 active:bg-muted',
          error ? 'border-destructive' : 'border-border',
          disabled && 'opacity-50',
        )}
        style={{ borderCurve: 'continuous' }}>
        {value?.brand ? (
          <BrandMark uuid={value.brand.uuid} name={value.brand.name} size={40} light />
        ) : (
          <View className="h-10 w-10 items-center justify-center rounded-xl bg-secondary" style={{ borderCurve: 'continuous' }}>
            <Icon name="vehicle" tone="primary" size={20} />
          </View>
        )}
        <View className="flex-1 gap-0.5">
          <Text variant="body" numberOfLines={1} tone={title ? 'default' : 'muted'}>
            {title || t('vehicle_picker.placeholder')}
          </Text>
          {sub ? (
            <Text variant="footnote" tone="muted" numberOfLines={1}>
              {sub}
            </Text>
          ) : null}
        </View>
        <Text variant="footnote" tone="primary" className="font-semibold">
          {title ? t('vehicle_picker.change') : t('vehicle_picker.select')}
        </Text>
      </Pressable>
      {error ? (
        <Text variant="footnote" tone="danger">
          {error}
        </Text>
      ) : null}
      {open ? (
        <VehiclePickerSheet
          initial={value ?? EMPTY_VEHICLE}
          frequent={frequent}
          onClose={() => setOpen(false)}
          onDone={(v) => {
            onChange(v);
            setOpen(false);
          }}
        />
      ) : null}
    </View>
  );
}

function VehiclePickerSheet({
  initial,
  frequent,
  onClose,
  onDone,
}: {
  initial: VehicleValue;
  frequent: boolean;
  onClose: () => void;
  onDone: (v: VehicleValue) => void;
}) {
  const { t } = useTranslation('common');
  const insets = useSafeAreaInsets();
  const [draft, setDraft] = useState<VehicleValue>(initial);
  const [step, setStep] = useState<PickerStep>(() => startStep(initial));
  const back = previousStep(step);

  const tap = () => void Haptics.selectionAsync().catch(() => undefined);

  return (
    <Modal visible animationType="slide" presentationStyle="pageSheet" onRequestClose={back ? () => setStep(back) : onClose}>
      {/* iOS presents a page sheet below the status bar; Android shows it full screen. */}
      <View className="flex-1 bg-background" style={{ paddingTop: Platform.OS === 'ios' ? 0 : insets.top }}>
        <View className="min-h-14 flex-row items-center gap-2 px-2">
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={back ? t('back') : t('cancel')}
            hitSlop={8}
            onPress={back ? () => setStep(back) : onClose}
            className="min-h-11 min-w-20 flex-row items-center gap-0.5 px-2">
            {back ? <Icon name="back" tone="primary" size={20} weight="semibold" /> : null}
            <Text variant="body" tone="primary">
              {back ? t('back') : t('cancel')}
            </Text>
          </Pressable>
          <Text variant="headline" numberOfLines={1} className="flex-1 text-center">
            {t(`vehicle_picker.title.${step}`)}
          </Text>
          {back ? (
            <Pressable
              accessibilityRole="button"
              hitSlop={8}
              onPress={onClose}
              className="min-h-11 min-w-20 items-end justify-center px-2">
              <Text variant="body" tone="primary">
                {t('cancel')}
              </Text>
            </Pressable>
          ) : (
            <View className="min-w-20" />
          )}
        </View>

        <StepBar
          value={draft}
          step={step}
          onStep={(s) => {
            tap();
            setStep(s);
          }}
        />

        {step === 'brand' ? (
          <BrandStep
            selected={draft.brand}
            frequent={frequent}
            bottom={insets.bottom}
            onPick={(b) => {
              tap();
              setDraft((d) => pickBrand(d, b));
              setStep('model');
            }}
          />
        ) : null}
        {step === 'model' && draft.brand ? (
          <ModelStep
            brand={draft.brand}
            selected={draft.model}
            frequent={frequent}
            bottom={insets.bottom}
            onPick={(m) => {
              tap();
              setDraft((d) => pickModel(d, m));
              setStep('year');
            }}
          />
        ) : null}
        {step === 'year' && draft.brand && draft.model ? (
          <YearStep
            model={draft.model}
            selected={draft.year}
            bottom={insets.bottom}
            onPick={(year) => {
              tap();
              onDone({ ...draft, year });
            }}
          />
        ) : null}
      </View>
    </Modal>
  );
}

/** Three steps with the picked value under each; done steps are tappable to go back. */
function StepBar({ value, step, onStep }: { value: VehicleValue; step: PickerStep; onStep: (s: PickerStep) => void }) {
  const { t } = useTranslation('common');
  const reachable = reachableSteps(value, step);
  const at = PICKER_STEPS.indexOf(step);
  const picked: Record<PickerStep, string> = {
    brand: value.brand?.name ?? '',
    model: value.model?.name ?? '',
    year: value.year ? String(value.year) : '',
  };
  return (
    <View className="flex-row gap-2 px-4 pb-3">
      {PICKER_STEPS.map((s, i) => {
        const current = s === step;
        const done = i < at || (!!picked[s] && !current);
        const can = reachable.has(s) && !current;
        return (
          <Pressable
            key={s}
            accessibilityRole="tab"
            accessibilityState={{ selected: current, disabled: !can && !current }}
            accessibilityLabel={[t(`vehicle_picker.step.${s}`), picked[s]].filter(Boolean).join(': ')}
            disabled={!can}
            onPress={() => onStep(s)}
            className={cn(
              'min-h-[52px] flex-1 justify-center gap-0.5 rounded-lg border px-2.5 py-1.5',
              current ? 'border-primary bg-accent' : 'border-border bg-card',
              !can && !current && !done && 'opacity-60',
            )}
            style={{ borderCurve: 'continuous' }}>
            <View className="flex-row items-center gap-1">
              <View
                className={cn(
                  'h-4 w-4 items-center justify-center rounded-full',
                  done ? 'bg-primary' : current ? 'border border-primary' : 'border border-border',
                )}>
                {done ? (
                  <Icon name="check" tone="primary-foreground" size={10} weight="semibold" />
                ) : (
                  <Text className={cn('text-[10px] font-bold', current ? 'text-primary' : 'text-muted-foreground')}>{i + 1}</Text>
                )}
              </View>
              <Text variant="caption" tone={current ? 'primary' : 'muted'} className="font-semibold">
                {t(`vehicle_picker.step.${s}`)}
              </Text>
            </View>
            <Text variant="footnote" numberOfLines={1} tone={picked[s] ? 'default' : 'muted'} className="font-medium">
              {picked[s] || '—'}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

// ------------------------------------------------------------------- brand

function BrandStep({
  selected,
  frequent,
  bottom,
  onPick,
}: {
  selected: BrandChoice | null;
  frequent: boolean;
  bottom: number;
  onPick: (b: BrandChoice) => void;
}) {
  const { t } = useTranslation('common');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  const list = useInfiniteQuery({
    queryKey: ['vehicle-catalog', 'brands', 'picker', CATALOG_SORT, query],
    initialPageParam: 0,
    staleTime: CATALOG_STALE,
    // Most used first; by name on a backend that refuses popularity.
    queryFn: ({ pageParam }) =>
      withSortFallback((sort) =>
        unwrap(
          api.GET('/v1/vehicle-catalog/brands', {
            params: { query: catalogQuery({ q: query, sort, limit: PAGE, offset: pageParam }) },
          }),
        ),
      ),
    getNextPageParam: nextOffset,
  });
  const top = useQuery({
    queryKey: ['vehicle-catalog', 'frequent', 'brands'],
    enabled: frequent,
    staleTime: 5 * 60_000,
    retry: false,
    // An older backend without the endpoint (404) or a role without access just means no section.
    queryFn: async () => frequentList<FrequentBrand>((await unwrap(api.GET('/v1/stats/frequent-vehicles')))?.brands),
  });

  const items = useMemo(() => (list.data?.pages ?? []).flatMap((p) => p.items), [list.data]);
  const rows = useMemo(
    () =>
      buildRows<BrandChoice & { model_count?: number }, FrequentBrand>(
        items.map((b: CatalogBrand) => ({ uuid: b.uuid, name: b.name, model_count: b.model_count })),
        top.data,
        !!query,
        (f) => ({ uuid: f.uuid, name: f.name }),
      ),
    [items, top.data, query],
  );

  return (
    <StepList
      search={{ value: q, onChange: setQ, placeholder: t('vehicle_picker.search_brand') }}
      resetKey={query}
      rows={rows}
      headerTitle={(s) => t(s === 'frequent' ? 'vehicle_picker.frequent' : 'vehicle_picker.all_brands')}
      loading={list.isPending}
      error={list.error}
      onRetry={() => void list.refetch()}
      empty={t('vehicle_picker.no_brand')}
      fetchingMore={list.isFetchingNextPage}
      onEnd={() => {
        if (list.hasNextPage && !list.isFetchingNextPage) void list.fetchNextPage();
      }}
      bottom={bottom}
      renderItem={(b, isTop) => (
        <PickRow
          title={b.name}
          subtitle={!isTop && b.model_count ? t('vehicle_picker.model_count', { count: b.model_count }) : undefined}
          selected={b.uuid === selected?.uuid}
          leading={<BrandMark uuid={b.uuid} name={b.name} size={36} light />}
          onPress={() => onPick({ uuid: b.uuid, name: b.name })}
        />
      )}
    />
  );
}

// ------------------------------------------------------------------- model

function ModelStep({
  brand,
  selected,
  frequent,
  bottom,
  onPick,
}: {
  brand: BrandChoice;
  selected: ModelChoice | null;
  frequent: boolean;
  bottom: number;
  onPick: (m: ModelChoice) => void;
}) {
  const { t } = useTranslation('common');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  const list = useInfiniteQuery({
    queryKey: ['vehicle-catalog', 'models', 'picker', CATALOG_SORT, brand.uuid, query],
    initialPageParam: 0,
    staleTime: CATALOG_STALE,
    queryFn: ({ pageParam }) =>
      withSortFallback((sort) =>
        unwrap(
          api.GET('/v1/vehicle-catalog/models', {
            params: { query: catalogQuery({ brandUuid: brand.uuid, q: query, sort, limit: PAGE, offset: pageParam }) },
          }),
        ),
      ),
    getNextPageParam: nextOffset,
  });
  const top = useQuery({
    queryKey: ['vehicle-catalog', 'frequent', 'models', brand.uuid],
    enabled: frequent,
    staleTime: 5 * 60_000,
    retry: false,
    queryFn: async () =>
      frequentList<FrequentModel>(
        (await unwrap(api.GET('/v1/stats/frequent-vehicles', { params: { query: { car_brand_uuid: brand.uuid } } })))?.models,
      ),
  });

  type Row = ModelChoice & { body?: string | null; powertrain?: string | null };
  const items = useMemo(() => (list.data?.pages ?? []).flatMap((p) => p.items), [list.data]);
  const rows = useMemo(
    () =>
      buildRows<Row, FrequentModel>(
        items.map((m: CatalogModel) => ({
          uuid: m.uuid,
          name: m.name,
          yearStart: m.year_start,
          yearStop: m.year_stop,
          body: m.body_type,
          powertrain: m.powertrain,
        })),
        top.data,
        !!query,
        (f) => ({ uuid: f.uuid, name: f.name }),
      ),
    [items, top.data, query],
  );

  return (
    <StepList
      search={{ value: q, onChange: setQ, placeholder: t('vehicle_picker.search_model', { brand: brand.name }) }}
      resetKey={query}
      rows={rows}
      headerTitle={(s) => t(s === 'frequent' ? 'vehicle_picker.frequent' : 'vehicle_picker.all_models')}
      loading={list.isPending}
      error={list.error}
      onRetry={() => void list.refetch()}
      empty={t('vehicle_picker.no_model')}
      fetchingMore={list.isFetchingNextPage}
      onEnd={() => {
        if (list.hasNextPage && !list.isFetchingNextPage) void list.fetchNextPage();
      }}
      bottom={bottom}
      leading={<BrandMark uuid={brand.uuid} name={brand.name} size={28} light />}
      renderItem={(m) => (
        <PickRow
          title={m.name}
          subtitle={[yearSpan(m.yearStart, m.yearStop), m.body, m.powertrain].filter(Boolean).join(' · ') || undefined}
          selected={m.uuid === selected?.uuid}
          onPress={() => onPick({ uuid: m.uuid, name: m.name, yearStart: m.yearStart, yearStop: m.yearStop })}
        />
      )}
    />
  );
}

// -------------------------------------------------------------------- year

function YearStep({
  model,
  selected,
  bottom,
  onPick,
}: {
  model: ModelChoice;
  selected: number | null;
  bottom: number;
  onPick: (y: number | null) => void;
}) {
  const { t } = useTranslation('common');
  const span = yearSpan(model.yearStart, model.yearStop);
  const years = useMemo(() => {
    const ys = yearOptions(model);
    // Keep an existing year that lies outside the catalog span (edited vehicle).
    return selected != null && !ys.includes(selected) ? [selected, ...ys] : ys;
  }, [model, selected]);

  return (
    <ScrollView className="flex-1" contentContainerClassName="gap-4 px-4 pt-1" contentContainerStyle={{ paddingBottom: bottom + 24 }}>
      {span ? (
        <Text variant="footnote" tone="muted">
          {t('vehicle_picker.year_span', { span })}
        </Text>
      ) : null}
      <View className="flex-row flex-wrap gap-2">
        {years.map((y) => {
          const on = y === selected;
          return (
            <Pressable
              key={y}
              accessibilityRole="button"
              accessibilityState={{ selected: on }}
              onPress={() => onPick(y)}
              className={cn(
                'min-h-12 items-center justify-center rounded-lg border active:opacity-80',
                on ? 'border-primary bg-primary' : 'border-border bg-card',
              )}
              style={{ width: '23%', flexGrow: 1, maxWidth: '24%', borderCurve: 'continuous' }}>
              <Text variant="callout" className={cn('font-semibold tabular-nums', on && 'text-primary-foreground')}>
                {y}
              </Text>
            </Pressable>
          );
        })}
      </View>
      <Pressable
        accessibilityRole="button"
        onPress={() => onPick(null)}
        className="min-h-12 flex-row items-center justify-center gap-2 rounded-lg border border-dashed border-border px-4 active:bg-muted"
        style={{ borderCurve: 'continuous' }}>
        <Text variant="callout" tone="primary" className="font-semibold">
          {t('vehicle_picker.year_skip')}
        </Text>
      </Pressable>
    </ScrollView>
  );
}

// ------------------------------------------------------------------ shared

function nextOffset(last: { items: unknown[]; offset: number; total: number }) {
  const next = last.offset + last.items.length;
  return last.items.length > 0 && next < last.total ? next : undefined;
}

function StepList<T extends { uuid: string }>({
  search,
  resetKey,
  rows,
  headerTitle,
  loading,
  error,
  onRetry,
  empty,
  fetchingMore,
  onEnd,
  bottom,
  leading,
  renderItem,
}: {
  search: { value: string; onChange: (v: string) => void; placeholder: string };
  /** Changes with the debounced search; remounts the list at the top. */
  resetKey: string;
  rows: PickerRow<T>[];
  headerTitle: (s: 'frequent' | 'all') => string;
  loading: boolean;
  error: unknown;
  onRetry: () => void;
  empty: string;
  fetchingMore: boolean;
  onEnd: () => void;
  bottom: number;
  leading?: React.ReactNode;
  renderItem: (item: T, frequent: boolean) => React.ReactElement;
}) {
  const { t } = useTranslation('common');

  return (
    <View className="flex-1">
      <View className="flex-row items-center gap-2 px-4 pb-2">
        {leading}
        <View className="flex-1">
          <SearchField value={search.value} onChange={search.onChange} placeholder={search.placeholder} />
        </View>
      </View>
      {loading ? (
        <View className="gap-2 px-4 pt-2">
          {Array.from({ length: 7 }, (_, i) => (
            <Skeleton key={i} className="h-14 rounded-lg" />
          ))}
        </View>
      ) : error && !rows.length ? (
        <View className="items-center gap-3 px-6 py-10">
          <Text variant="callout" tone="muted" className="text-center">
            {errorText(error)}
          </Text>
          <Pressable accessibilityRole="button" onPress={onRetry} hitSlop={8}>
            <Text variant="callout" tone="primary" className="font-semibold">
              {t('retry')}
            </Text>
          </Pressable>
        </View>
      ) : (
        <FlashList
          // A new search result starts at the top of the list.
          key={resetKey}
          data={rows}
          keyExtractor={(r) => r.key}
          getItemType={(r) => r.kind}
          keyboardShouldPersistTaps="handled"
          keyboardDismissMode="on-drag"
          onEndReached={onEnd}
          onEndReachedThreshold={0.6}
          contentContainerStyle={{ paddingBottom: bottom + 24 }}
          ListEmptyComponent={
            <Text variant="callout" tone="muted" className="px-5 py-10 text-center">
              {empty}
            </Text>
          }
          ListFooterComponent={fetchingMore ? <ActivityIndicator className="py-4" /> : null}
          renderItem={({ item: r }) =>
            r.kind === 'header' ? (
              <View className="flex-row items-center gap-1.5 px-4 pb-1.5 pt-4">
                {r.section === 'frequent' ? <Icon name="star" tone="primary" size={13} weight="semibold" /> : null}
                <Text variant="caption" tone={r.section === 'frequent' ? 'primary' : 'muted'} className="font-semibold">
                  {headerTitle(r.section)}
                </Text>
              </View>
            ) : (
              renderItem(r.item, r.frequent)
            )
          }
        />
      )}
    </View>
  );
}

function PickRow({
  title,
  subtitle,
  selected,
  leading,
  onPress,
}: {
  title: string;
  subtitle?: string;
  selected?: boolean;
  leading?: React.ReactNode;
  onPress: () => void;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ selected: !!selected }}
      onPress={onPress}
      className={cn('min-h-14 flex-row items-center gap-3 px-4 py-2 active:bg-muted', selected && 'bg-accent')}>
      {leading}
      <View className="flex-1 gap-0.5">
        <Text variant="body" numberOfLines={1} className={selected ? 'font-semibold' : undefined}>
          {title}
        </Text>
        {subtitle ? (
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {subtitle}
          </Text>
        ) : null}
      </View>
      {selected ? <Icon name="check" tone="primary" size={18} weight="semibold" /> : <Icon name="forward" tone="muted-foreground" size={16} />}
    </Pressable>
  );
}
