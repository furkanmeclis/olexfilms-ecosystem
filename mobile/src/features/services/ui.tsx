import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Pressable, ScrollView, TextInput, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { CircleAlert, type LucideIcon, Search, X } from '@/ui/icons';
import { cn } from '@/ui/cn';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

/** Localized message of a failed query or mutation. */
export function errorText(e: unknown) {
  return errorMessage(isApiError(e) ? e.code : undefined);
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  const { t } = useTranslation('common');
  return (
    <EmptyState
      icon={CircleAlert}
      title={t('error_generic')}
      body={errorText(error)}
      action={{ title: t('retry'), onPress: onRetry }}
    />
  );
}

/** Placeholder rows while nothing is cached yet. */
export function ListSkeleton({ rows = 6 }: { rows?: number }) {
  return (
    <View className="gap-2.5 px-4 pt-2">
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-[76px] rounded-lg" />
      ))}
    </View>
  );
}

/** Empty, error or loading body of a list (null when there are rows). */
export function ListBody({
  isPending,
  error,
  count,
  onRetry,
  empty,
}: {
  isPending: boolean;
  error: unknown;
  count: number;
  onRetry: () => void;
  empty: { icon: LucideIcon; title: string; body?: string; action?: { title: string; onPress: () => void } };
}) {
  if (count > 0) return null;
  if (isPending) return <ListSkeleton />;
  if (error) return <ErrorState error={error} onRetry={onRetry} />;
  return <EmptyState {...empty} />;
}

export function ListFooter({ loading }: { loading: boolean }) {
  return <View className="h-16 items-center justify-center">{loading ? <ActivityIndicator /> : null}</View>;
}

export function SearchField({
  value,
  onChange,
  placeholder,
  autoFocus,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  autoFocus?: boolean;
}) {
  const { t } = useTranslation('common');
  return (
    <View className="min-h-11 flex-row items-center gap-2 rounded-lg bg-secondary px-3">
      <Icon as={Search} tone="muted-foreground" size={18} />
      <TextInput
        value={value}
        onChangeText={onChange}
        placeholder={placeholder}
        autoFocus={autoFocus}
        placeholderTextColorClassName="accent-muted-foreground"
        returnKeyType="search"
        autoCorrect={false}
        autoCapitalize="none"
        className="flex-1 py-2 text-[17px] text-foreground"
      />
      {value ? (
        <Pressable accessibilityRole="button" accessibilityLabel={t('close')} hitSlop={8} onPress={() => onChange('')}>
          <Icon as={X} tone="muted-foreground" size={18} />
        </Pressable>
      ) : null}
    </View>
  );
}

export function Chips<T extends string>({
  options,
  value,
  onChange,
}: {
  options: { value: T; label: string }[];
  value: T;
  onChange: (v: T) => void;
}) {
  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerClassName="gap-2">
      {options.map((o) => {
        const on = o.value === value;
        return (
          <Pressable
            key={o.value}
            accessibilityRole="button"
            accessibilityState={{ selected: on }}
            onPress={() => onChange(o.value)}
            className={cn(
              'min-h-9 justify-center rounded-full border px-3.5',
              on ? 'border-primary bg-primary' : 'border-border bg-card',
            )}>
            <Text variant="footnote" className={cn('font-semibold', on && 'text-primary-foreground')}>
              {o.label}
            </Text>
          </Pressable>
        );
      })}
    </ScrollView>
  );
}

/** Card section with a caption title, used by every detail screen. */
export function Section({ title, children, right }: { title?: string; children: React.ReactNode; right?: React.ReactNode }) {
  return (
    <View className="gap-1.5">
      {title || right ? (
        <View className="flex-row items-center justify-between px-1">
          {title ? (
            <Text variant="caption" tone="muted">
              {title}
            </Text>
          ) : (
            <View />
          )}
          {right}
        </View>
      ) : null}
      <View className="overflow-hidden rounded-lg border border-border bg-card">{children}</View>
    </View>
  );
}

/** Label / value line inside a Section. */
export function InfoRow({
  label,
  value,
  last,
  onPress,
  mono,
}: {
  label: string;
  value?: string | number | null;
  last?: boolean;
  onPress?: () => void;
  mono?: boolean;
}) {
  if (value === undefined || value === null || value === '') return null;
  const body = (
    <View className={cn('min-h-11 flex-row items-center gap-3 px-4 py-2.5', !last && 'border-b border-border')}>
      <Text variant="callout" tone="muted" className="w-32">
        {label}
      </Text>
      <Text
        variant="callout"
        tone={onPress ? 'primary' : 'default'}
        selectable
        className={cn('flex-1 text-right', mono && 'font-mono')}>
        {String(value)}
      </Text>
    </View>
  );
  if (!onPress) return body;
  return (
    <Pressable accessibilityRole="link" onPress={onPress} className="active:bg-muted">
      {body}
    </Pressable>
  );
}

/** Skeleton of a detail screen (title block + two sections). */
export function DetailSkeleton() {
  return (
    <View className="gap-4 px-4 pt-2">
      <Skeleton className="h-16 rounded-lg" />
      <Skeleton className="h-40 rounded-lg" />
      <Skeleton className="h-32 rounded-lg" />
    </View>
  );
}
