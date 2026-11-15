import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

/** Placeholder of a chart while its first answer loads (keeps the layout in place). */
export function ChartSkeleton({ height = 180 }: { height?: number }) {
  return (
    <View style={{ height }}>
      <Skeleton className="h-full w-full rounded-lg" />
    </View>
  );
}

/** "No data for this period" inside a chart card. */
export function ChartEmpty({ height = 120, text }: { height?: number; text?: string }) {
  const { t } = useTranslation('analytics');
  return (
    <View className="items-center justify-center gap-2" style={{ minHeight: height }}>
      <Icon name="chart" tone="muted-foreground" size={22} />
      <Text variant="footnote" tone="muted" className="text-center">
        {text ?? t('empty')}
      </Text>
    </View>
  );
}

/** Compact error inside a chart card, with retry. */
export function ChartError({ error, onRetry, height = 120 }: { error: unknown; onRetry?: () => void; height?: number }) {
  const { t } = useTranslation();
  const msg = isApiError(error, 'NETWORK_ERROR')
    ? t('common:error_network')
    : errorMessage(isApiError(error) ? error.code : undefined, t('common:error_generic'));
  return (
    <View className="items-center justify-center gap-2" style={{ minHeight: height }}>
      <Icon name="offline" tone="muted-foreground" size={22} />
      <Text variant="footnote" tone="muted" className="text-center">
        {msg}
      </Text>
      {onRetry ? <Button title={t('common:retry')} variant="ghost" onPress={onRetry} /> : null}
    </View>
  );
}

/**
 * Loading / error / empty / content of one chart query: cached data renders
 * even while a refetch fails; the error shows only without data.
 */
export function ChartState({
  pending,
  error,
  hasData,
  empty,
  onRetry,
  height,
  children,
}: {
  pending: boolean;
  error?: unknown;
  hasData: boolean;
  empty: boolean;
  onRetry?: () => void;
  height?: number;
  children: React.ReactNode;
}) {
  if (pending && !hasData) return <ChartSkeleton height={height} />;
  if (error && !hasData) return <ChartError error={error} onRetry={onRetry} height={height} />;
  if (empty) return <ChartEmpty height={height} />;
  return <>{children}</>;
}
