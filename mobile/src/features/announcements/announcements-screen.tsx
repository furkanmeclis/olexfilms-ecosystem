import { useQuery } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { Chips, PagedList, PAGE_SIZE, useChildHref, useFormat, usePagedQuery } from '@/features/admin/kit';
import { cn } from '@/ui/cn';
import { Text } from '@/ui/text';

import { isUnread, previewOf } from './lib';

type Filter = 'all' | 'unread';

export function AnnouncementsScreen() {
  const { t, i18n } = useTranslation('announcements');
  const f = useFormat();
  const href = useChildHref();
  const [filter, setFilter] = useState<Filter>('all');
  const locale = i18n.language as never;

  const unread = useQuery({
    queryKey: ['announcements', 'unread-count', i18n.language],
    queryFn: () => unwrap(api.GET('/v1/announcements/unread-count', { params: { query: { locale } } })),
  });

  const list = usePagedQuery(['announcements', 'list', filter, i18n.language], (offset) =>
    unwrap(
      api.GET('/v1/announcements', {
        params: { query: { locale, unread_only: filter === 'unread' ? true : undefined, limit: PAGE_SIZE, offset } },
      }),
    ),
  );

  const count = unread.data?.unread_count ?? 0;
  // Unread first inside the feed; pinned order from the server is kept as the tiebreak.
  const items = [...list.items].sort((a, b) => Number(isUnread(b)) - Number(isUnread(a)));

  return (
    <>
      <Stack.Screen options={{ title: t('title') }} />
      <PagedList
        query={{ ...list, items }}
        keyOf={(a) => a.uuid}
        header={
          <Chips
            options={[
              { key: 'all' as Filter, label: t('filter.all') },
              { key: 'unread' as Filter, label: count ? t('filter.unread_count', { count }) : t('filter.unread') },
            ]}
            value={filter}
            onChange={setFilter}
          />
        }
        empty={{ icon: 'announcement', title: filter === 'unread' ? t('empty_unread') : t('empty') }}
        renderItem={({ item }) => {
          const fresh = isUnread(item);
          return (
            <Pressable accessibilityRole="button" onPress={() => router.push(href(item.uuid))} className="active:opacity-70">
              <View className="mx-4 flex-row items-start gap-3 rounded-lg border border-border bg-card px-4 py-3">
                <View className={cn('mt-1.5 size-2 rounded-full', fresh ? 'bg-primary' : 'bg-transparent')} />
                <View className="flex-1 gap-0.5">
                  <Text variant="headline" numberOfLines={2} className={fresh ? '' : 'font-normal'}>
                    {item.title}
                  </Text>
                  <Text variant="footnote" tone="muted" numberOfLines={2}>
                    {previewOf(item.body, item.body_format)}
                  </Text>
                  <Text variant="footnote" tone="muted" numberOfLines={1}>
                    {[item.pinned ? t('pinned') : null, f.date(item.publish_at ?? item.created_at)].filter(Boolean).join(' · ')}
                  </Text>
                </View>
              </View>
            </Pressable>
          );
        }}
      />
    </>
  );
}
