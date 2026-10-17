import { useInfiniteQuery, type QueryKey } from '@tanstack/react-query';
import { usePathname } from 'expo-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, type ListRenderItem, Pressable, RefreshControl, ScrollView, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { CloudOff, Inbox, type LucideIcon } from '@/ui/icons';
import { cn } from '@/ui/cn';
import { EmptyState } from '@/ui/empty-state';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

/** Page shape of the list contract (docs/list-contract.md). */
export type Page<T> = { items: T[]; total: number; limit: number; offset: number };

export const PAGE_SIZE = 20;

/** Offset-paged list query; `fetchPage` gets the offset of the next page. */
export function usePagedQuery<T>(queryKey: QueryKey, fetchPage: (offset: number) => Promise<Page<T>>, enabled = true) {
  const q = useInfiniteQuery({
    queryKey,
    enabled,
    initialPageParam: 0,
    queryFn: ({ pageParam }) => fetchPage(pageParam),
    getNextPageParam: (last) => {
      const next = last.offset + last.items.length;
      return last.items.length > 0 && next < last.total ? next : undefined;
    },
  });
  const items = q.data?.pages.flatMap((p) => p.items) ?? [];
  const total = q.data?.pages[0]?.total;
  return { ...q, items, total };
}

/** Value that settles after the user stops typing. */
export function useDebounced<T>(value: T, ms = 300) {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return v;
}

/** Href of a child route of the current screen (screens are shared between spaces). */
export function useChildHref() {
  const path = usePathname().replace(/\/$/, '');
  return (child: string) => `${path}/${child}` as never;
}

export function useFormat() {
  const { i18n } = useTranslation();
  const locale = i18n.language;
  return {
    locale,
    number: (n: number | string | null | undefined, digits = 0) => {
      const v = typeof n === 'string' ? Number(n) : n;
      if (v === null || v === undefined || Number.isNaN(v)) return '–';
      return new Intl.NumberFormat(locale, { maximumFractionDigits: digits }).format(v);
    },
    money: (n: string | number | null | undefined, currency: string) => {
      const v = typeof n === 'string' ? Number(n) : n;
      if (v === null || v === undefined || Number.isNaN(v)) return '–';
      try {
        return new Intl.NumberFormat(locale, { style: 'currency', currency }).format(v);
      } catch {
        return `${v.toFixed(2)} ${currency}`;
      }
    },
    date: (iso: string | null | undefined) =>
      iso ? new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(new Date(iso)) : '–',
    dateTime: (iso: string | null | undefined) =>
      iso ? new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(iso)) : '–',
  };
}

/** Error with a retry button; the message comes from the shared error catalog. */
export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const { t } = useTranslation();
  const network = isApiError(error, 'NETWORK_ERROR');
  const msg = network
    ? t('common:error_network')
    : errorMessage(isApiError(error) ? error.code : undefined, t('common:error_generic'));
  return (
    <EmptyState
      icon={CloudOff}
      title={msg}
      action={onRetry ? { title: t('common:retry'), onPress: onRetry } : undefined}
    />
  );
}

export function ListSkeleton({ rows = 6, inset = true }: { rows?: number; inset?: boolean }) {
  return (
    <View className={cn('gap-2.5 pt-2', inset && 'px-4')}>
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-16 rounded-lg" />
      ))}
    </View>
  );
}

/** Horizontal single-choice chips (filters, periods). */
export function Chips<K extends string>({
  options,
  value,
  onChange,
  className,
}: {
  options: { key: K; label: string }[];
  value: K;
  onChange: (k: K) => void;
  className?: string;
}) {
  return (
    <ScrollView
      horizontal
      showsHorizontalScrollIndicator={false}
      className={cn('grow-0', className)}
      contentContainerClassName="gap-2 px-4 py-2">
      {options.map((o) => {
        const on = o.key === value;
        return (
          <Pressable
            key={o.key}
            accessibilityRole="button"
            accessibilityState={{ selected: on }}
            onPress={() => onChange(o.key)}
            className={cn(
              'min-h-9 justify-center rounded-full border px-3.5',
              on ? 'border-primary bg-primary' : 'border-border bg-card active:bg-muted',
            )}>
            <Text variant="footnote" tone={on ? 'onPrimary' : 'default'} className="font-semibold">
              {o.label}
            </Text>
          </Pressable>
        );
      })}
    </ScrollView>
  );
}

/** Label / value row inside a List. */
export function InfoRow({ label, value, last }: { label: string; value?: string | null; last?: boolean }) {
  return (
    <View className={cn('min-h-12 flex-row items-center gap-3 px-4 py-3', !last && 'border-b border-border')}>
      <Text variant="callout" tone="muted" className="flex-1">
        {label}
      </Text>
      <Text variant="callout" className="shrink text-right" selectable>
        {value || '–'}
      </Text>
    </View>
  );
}

type PagedListProps<T> = {
  query: ReturnType<typeof usePagedQuery<T>>;
  renderItem: ListRenderItem<T>;
  keyOf: (item: T) => string;
  empty: { icon?: LucideIcon; title: string; body?: string };
  header?: React.ReactElement;
};

/**
 * Infinite list with pull-to-refresh, skeleton (nothing cached yet), empty and
 * error states. Rows render as cards so every list looks the same.
 */
export function PagedList<T>({ query, renderItem, keyOf, empty, header }: PagedListProps<T>) {
  const { items } = query;
  let body: React.ReactElement | null = null;
  if (query.isPending) body = <ListSkeleton />;
  else if (query.isError && !items.length) body = <ErrorState error={query.error} onRetry={() => void query.refetch()} />;
  else if (!items.length) body = <EmptyState icon={empty.icon ?? Inbox} title={empty.title} body={empty.body} />;

  return (
    <FlatList
      data={items}
      keyExtractor={keyOf}
      renderItem={renderItem}
      contentInsetAdjustmentBehavior="automatic"
      keyboardShouldPersistTaps="handled"
      className="flex-1 bg-background"
      contentContainerClassName="gap-2.5 pb-10"
      ListHeaderComponent={header}
      ListEmptyComponent={body ?? undefined}
      onEndReachedThreshold={0.4}
      onEndReached={() => {
        if (query.hasNextPage && !query.isFetchingNextPage) void query.fetchNextPage();
      }}
      ListFooterComponent={query.isFetchingNextPage ? <Skeleton className="mx-4 h-16 rounded-lg" /> : undefined}
      refreshControl={
        <RefreshControl refreshing={query.isRefetching && !query.isFetchingNextPage} onRefresh={() => void query.refetch()} />
      }
    />
  );
}

/** Card row used by the lists of this slice. */
export function CardRow({
  title,
  subtitle,
  meta,
  trailing,
  onPress,
}: {
  title: string;
  subtitle?: string;
  meta?: string;
  trailing?: React.ReactNode;
  onPress?: () => void;
}) {
  const inner = (
    <View className="mx-4 flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-3">
      <View className="flex-1 gap-0.5">
        <Text variant="headline" numberOfLines={1}>
          {title}
        </Text>
        {subtitle ? (
          <Text variant="footnote" tone="muted" numberOfLines={2}>
            {subtitle}
          </Text>
        ) : null}
        {meta ? (
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {meta}
          </Text>
        ) : null}
      </View>
      {trailing}
    </View>
  );
  if (!onPress) return inner;
  return (
    <Pressable accessibilityRole="button" onPress={onPress} className="active:opacity-70">
      {inner}
    </Pressable>
  );
}
