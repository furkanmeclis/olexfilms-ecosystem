import { useMutation, useQueryClient } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { useResolvedSpace } from '@/core/spaces';
import { Chips, PagedList, PAGE_SIZE, useFormat, usePagedQuery } from '@/features/admin/kit';
import { Text } from '@/ui/text';

import { type AppNotification, notificationRoute } from './lib';

type Filter = 'all' | 'unread';

/** Staff inbox: newest first, unread marked, tap opens the record when the app has a screen for it. */
export function NotificationsScreen() {
  const { t } = useTranslation('notifications');
  const f = useFormat();
  const qc = useQueryClient();
  const space = useResolvedSpace();
  const [filter, setFilter] = useState<Filter>('all');

  const list = usePagedQuery<AppNotification>(['notifications', 'list', filter], (offset) =>
    unwrap(
      api.GET('/v1/notifications', {
        params: { query: { unread: filter === 'unread' ? true : undefined, limit: PAGE_SIZE, offset } },
      }),
    ),
  );

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ['notifications'] });
  };
  const onError = (e: unknown) => Alert.alert(errorMessage(isApiError(e) ? e.code : undefined, t('error_generic')));

  const read = useMutation({
    mutationFn: (uuid: string) => unwrap(api.POST('/v1/notifications/{uuid}/read', { params: { path: { uuid } } })),
    onSuccess: refresh,
    onError,
  });
  const readAll = useMutation({
    mutationFn: () => unwrap(api.POST('/v1/notifications/read-all')),
    onSuccess: refresh,
    onError,
  });

  const hasUnread = list.items.some((n) => !n.read_at) || filter === 'unread';

  const open = (n: AppNotification) => {
    if (!n.read_at) read.mutate(n.uuid);
    const href = notificationRoute(n, space);
    if (href) router.push(href as never);
  };

  return (
    <>
      <Stack.Screen
        options={{
          title: t('title'),
          headerRight: hasUnread
            ? () => (
                <Pressable
                  accessibilityRole="button"
                  hitSlop={8}
                  disabled={readAll.isPending}
                  onPress={() => readAll.mutate()}
                  className="active:opacity-60">
                  <Text variant="callout" tone="primary" className="font-semibold">
                    {t('mark_all')}
                  </Text>
                </Pressable>
              )
            : undefined,
        }}
      />
      <PagedList
        query={list}
        keyOf={(n) => n.uuid}
        header={
          <Chips
            options={(['all', 'unread'] as Filter[]).map((k) => ({ key: k, label: t(`filter.${k}`) }))}
            value={filter}
            onChange={setFilter}
          />
        }
        empty={{
          icon: 'bell',
          title: filter === 'unread' ? t('empty_unread') : t('empty'),
          body: filter === 'unread' ? undefined : t('empty_body'),
        }}
        renderItem={({ item }) => <Row n={item} date={f.dateTime(item.created_at)} onPress={() => open(item)} />}
      />
    </>
  );
}

function Row({ n, date, onPress }: { n: AppNotification; date: string; onPress: () => void }) {
  const unread = !n.read_at;
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      className="mx-4 flex-row items-start gap-3 rounded-lg border border-border bg-card px-4 py-3 active:opacity-70">
      <View className={unread ? 'mt-1.5 size-2.5 rounded-full bg-primary' : 'mt-1.5 size-2.5 rounded-full'} />
      <View className="flex-1 gap-0.5">
        <Text variant="headline" numberOfLines={2} className={unread ? '' : 'font-normal'}>
          {n.title}
        </Text>
        {n.body ? (
          <Text variant="footnote" tone="muted" numberOfLines={3}>
            {n.body}
          </Text>
        ) : null}
        <Text variant="caption" tone="muted">
          {date}
        </Text>
      </View>
    </Pressable>
  );
}
