import { useQuery } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';

import { isApiError } from '@/core/api/errors';
import { ErrorState, ListSkeleton } from '@/features/admin/kit';
import { EmptyState } from '@/ui/empty-state';
import { PullRefresh } from '@/ui/pull-refresh';
import { Screen } from '@/ui/screen';

import { fetchLegalText } from './api';
import { isLegalKind } from './lib';
import { MarkdownView } from './markdown-view';

/**
 * Privacy policy, KVKK notice or terms of use (TEC-1257), in the app
 * language. Opens from the sign-in screen (no session) and the profile.
 */
export function LegalTextScreen() {
  const { t, i18n } = useTranslation('legal');
  const { kind } = useLocalSearchParams<{ kind: string }>();
  const valid = isLegalKind(kind);
  const q = useQuery({
    queryKey: ['public', 'legal-text', kind, i18n.language],
    queryFn: ({ signal }) => fetchLegalText(kind as never, i18n.language, signal),
    enabled: valid,
    retry: (count, e) => !(isApiError(e) && e.status === 404) && count < 2,
    staleTime: 5 * 60_000,
  });
  const missing = !valid || (isApiError(q.error) && q.error.status === 404);

  return (
    <>
      <Stack.Screen options={{ title: valid ? t(kind) : '', headerLargeTitle: false }} />
      <Screen refreshControl={<PullRefresh busy={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {missing ? (
          <EmptyState icon="info" title={t('not_published')} />
        ) : q.isPending ? (
          <ListSkeleton rows={4} inset={false} />
        ) : q.data ? (
          <MarkdownView source={q.data.body} />
        ) : (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        )}
      </Screen>
    </>
  );
}
