import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Linking, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { ErrorState, ListSkeleton, useFormat } from '@/features/admin/kit';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { toBlocks, type Part } from './lib';

function Inline({ parts }: { parts: Part[] }) {
  return (
    <>
      {parts.map((p, i) =>
        p.url ? (
          <Text
            key={i}
            variant="body"
            tone="primary"
            accessibilityRole="link"
            className="underline"
            onPress={() => void Linking.openURL(p.url!).catch(() => undefined)}>
            {p.text}
          </Text>
        ) : (
          p.text
        ),
      )}
    </>
  );
}

/** Opening an announcement marks it read on the server (idempotent receipt). */
export function AnnouncementDetailScreen() {
  const { t, i18n } = useTranslation('announcements');
  const f = useFormat();
  const qc = useQueryClient();
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const locale = i18n.language as never;

  const q = useQuery({
    queryKey: ['announcements', 'detail', uuid, i18n.language],
    queryFn: () => unwrap(api.GET('/v1/announcements/{uuid}', { params: { path: { uuid: uuid! }, query: { locale } } })),
    enabled: !!uuid,
  });

  const loaded = !!q.data;
  useEffect(() => {
    if (!loaded) return;
    void qc.invalidateQueries({ queryKey: ['announcements', 'list'] });
    void qc.invalidateQueries({ queryKey: ['announcements', 'unread-count'] });
  }, [loaded, qc]);

  const a = q.data;
  const blocks = a ? toBlocks(a.body, a.body_format) : [];

  return (
    <>
      <Stack.Screen options={{ title: '', headerLargeTitle: false }} />
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {q.isPending ? (
          <ListSkeleton rows={4} inset={false} />
        ) : !a ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : (
          <>
            <View className="gap-1">
              <Text variant="title" selectable>
                {a.title}
              </Text>
              <Text variant="footnote" tone="muted">
                {[a.pinned ? t('pinned') : null, f.dateTime(a.publish_at ?? a.created_at)].filter(Boolean).join(' · ')}
              </Text>
            </View>
            <View className="gap-3">
              {blocks.map((b, i) =>
                b.kind === 'heading' ? (
                  <Text key={i} variant="headline" selectable>
                    <Inline parts={b.parts} />
                  </Text>
                ) : b.kind === 'bullet' ? (
                  <View key={i} className="flex-row gap-2 pl-1">
                    <Text variant="body">{b.marker ?? '•'}</Text>
                    <Text variant="body" selectable className="flex-1">
                      <Inline parts={b.parts} />
                    </Text>
                  </View>
                ) : (
                  <Text key={i} variant="body" selectable>
                    <Inline parts={b.parts} />
                  </Text>
                ),
              )}
            </View>
            {a.expires_at ? (
              <Text variant="footnote" tone="muted">
                {t('expires', { date: f.date(a.expires_at) })}
              </Text>
            ) : null}
          </>
        )}
      </Screen>
    </>
  );
}
