import * as Haptics from 'expo-haptics';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Pressable, View } from 'react-native';

import { cn } from '@/ui/cn';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

import { whError } from './format';
import type { IconName } from '@/ui/icons';

/** iOS-style segmented control (text + selected fill, never color alone). */
export function Segmented<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
}) {
  return (
    <View className="flex-row rounded-md bg-muted p-0.5" accessibilityRole="tablist">
      {options.map((o) => {
        const on = o.value === value;
        return (
          <Pressable
            key={o.value}
            accessibilityRole="tab"
            accessibilityState={{ selected: on }}
            onPress={() => {
              if (on) return;
              void Haptics.selectionAsync();
              onChange(o.value);
            }}
            className={cn('min-h-9 flex-1 items-center justify-center rounded-sm px-2', on && 'bg-card')}>
            <Text variant="footnote" numberOfLines={1} className={cn('font-semibold', !on && 'text-muted-foreground')}>
              {o.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

/** Horizontal filter chips (status filters). */
export function Chips<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
}) {
  return (
    <View className="flex-row flex-wrap gap-2">
      {options.map((o) => {
        const on = o.value === value;
        return (
          <Pressable
            key={o.value}
            accessibilityRole="button"
            accessibilityState={{ selected: on }}
            onPress={() => onChange(o.value)}
            className={cn('min-h-8 justify-center rounded-full border px-3', on ? 'border-primary bg-primary' : 'border-border bg-card')}>
            <Text variant="footnote" className={cn('font-semibold', on ? 'text-primary-foreground' : 'text-foreground')}>
              {o.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

/** Placeholder rows shown while nothing is cached yet. */
export function ListSkeleton({ rows = 6 }: { rows?: number }) {
  return (
    <View className="gap-2.5">
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-16 rounded-lg" />
      ))}
    </View>
  );
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <EmptyState
      icon="error"
      title={t('common:error_generic')}
      body={whError(error)}
      action={{ title: t('common:retry'), onPress: onRetry }}
    />
  );
}

/** Loading / error / empty switch for a list body. */
export function ListStatus({
  isLoading,
  error,
  empty,
  onRetry,
  emptyIcon,
  emptyTitle,
  emptyBody,
  emptyAction,
}: {
  isLoading: boolean;
  error: unknown;
  empty: boolean;
  onRetry: () => void;
  emptyIcon: IconName;
  emptyTitle: string;
  emptyBody?: string;
  emptyAction?: { title: string; onPress: () => void };
}) {
  if (isLoading) return <ListSkeleton />;
  if (error && empty) return <ErrorState error={error} onRetry={onRetry} />;
  if (empty) return <EmptyState icon={emptyIcon} title={emptyTitle} body={emptyBody} action={emptyAction} />;
  return null;
}

/** Card row for documents (transfers, counts, entries). */
export function DocCard({
  title,
  subtitle,
  meta,
  status,
  onPress,
}: {
  title: string;
  subtitle?: string;
  meta?: string;
  status: React.ReactNode;
  onPress: () => void;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      className="gap-1.5 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
      <View className="flex-row items-center gap-2">
        <Text variant="headline" numberOfLines={1} className="flex-1">
          {title}
        </Text>
        {status}
      </View>
      {subtitle ? (
        <Text variant="callout" numberOfLines={1}>
          {subtitle}
        </Text>
      ) : null}
      {meta ? (
        <Text variant="footnote" tone="muted" numberOfLines={1}>
          {meta}
        </Text>
      ) : null}
    </Pressable>
  );
}

/** Label / value pair inside a detail card. */
export function Field({ label, value, mono }: { label: string; value?: string | null; mono?: boolean }) {
  if (!value) return null;
  return (
    <View className="flex-row items-start justify-between gap-3 py-1">
      <Text variant="footnote" tone="muted">
        {label}
      </Text>
      <Text variant="callout" selectable className={cn('flex-1 text-right', mono && 'font-mono')}>
        {value}
      </Text>
    </View>
  );
}

export function Card({ children, className }: { children: React.ReactNode; className?: string }) {
  return <View className={cn('gap-1 rounded-lg border border-border bg-card p-4', className)}>{children}</View>;
}

/** Small icon button for list rows (remove, etc.). */
export function IconButton({
  icon,
  label,
  onPress,
  busy,
  tone = 'muted-foreground',
}: {
  icon: IconName;
  label: string;
  onPress: () => void;
  busy?: boolean;
  tone?: 'muted-foreground' | 'destructive' | 'primary';
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      hitSlop={8}
      disabled={busy}
      onPress={onPress}
      className="size-9 items-center justify-center rounded-md active:bg-muted">
      {busy ? <ActivityIndicator size="small" /> : <Icon name={icon} tone={tone} size={18} />}
    </Pressable>
  );
}
