import { useMutation, useQueryClient } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Linking, Pressable, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { intlLocale } from '@/core/digits';
import { errorMessage } from '@/core/i18n';
import { useSession } from '@/core/auth/session';
import { kindsOf, useResolvedSpace } from '@/core/spaces';
import { PagedList, PAGE_SIZE, usePagedQuery } from '@/features/admin/kit';
import { Segmented } from '@/features/warehouse/parts';
import { cn } from '@/ui/cn';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { useInbox } from './inbox';
import { type AppNotification, notificationExternalUrl, notificationIcon, notificationRoute, notificationSpace } from './lib';

type Filter = 'all' | 'unread';

/** "Today 14:20", "Yesterday", or the date; in the language's digits. */
function when(iso: string, lang: string, t: (k: string) => string) {
  const d = new Date(iso);
  const now = new Date();
  const days = Math.floor((new Date(now.toDateString()).getTime() - new Date(d.toDateString()).getTime()) / 86_400_000);
  const loc = intlLocale(lang);
  const time = new Intl.DateTimeFormat(loc, { hour: '2-digit', minute: '2-digit' }).format(d);
  if (days === 0) return `${t('today')} ${time}`;
  if (days === 1) return `${t('yesterday')} ${time}`;
  return new Intl.DateTimeFormat(loc, { day: 'numeric', month: 'short', year: days > 300 ? 'numeric' : undefined }).format(d);
}

/**
 * Inbox: newest first, unread rows tinted with a dot, an icon per record kind;
 * tapping marks the row read and opens the record when the app has a screen for it.
 */
export function NotificationsScreen() {
  const { t, i18n } = useTranslation('notifications');
  const qc = useQueryClient();
  const space = useResolvedSpace();
  const kinds = kindsOf(useSession((s) => s.me));
  const inbox = useInbox();
  const [filter, setFilter] = useState<Filter>('all');

  const list = usePagedQuery<AppNotification>(
    ['notifications', 'list', filter],
    (offset) => inbox.list({ unread: filter === 'unread' ? true : undefined, limit: PAGE_SIZE, offset }),
    inbox.enabled,
  );

  const refresh = () => void qc.invalidateQueries({ queryKey: ['notifications'] });
  const onError = (e: unknown) => Alert.alert(errorMessage(isApiError(e) ? e.code : undefined, t('error_generic')));

  const read = useMutation({ mutationFn: inbox.read, onSuccess: refresh, onError });
  const readAll = useMutation({ mutationFn: inbox.readAll, onSuccess: refresh, onError });

  const hasUnread = list.items.some((n) => !n.read_at);

  const open = (n: AppNotification) => {
    if (!n.read_at) read.mutate(n.uuid);
    const href = notificationRoute(n, notificationSpace(n.template_code, space, kinds));
    if (href) {
      router.push(href as never);
      return;
    }
    const external = notificationExternalUrl(n, space);
    if (external) void Linking.openURL(external);
  };

  if (!inbox.enabled) {
    return (
      <>
        <Stack.Screen options={{ title: t('title') }} />
        <EmptyState icon="blocked" title={t('no_permission')} />
      </>
    );
  }

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
          <View className="px-4 pb-2">
            <Segmented
              options={[
                { value: 'all' as Filter, label: t('filter.all') },
                { value: 'unread' as Filter, label: t('filter.unread') },
              ]}
              value={filter}
              onChange={setFilter}
            />
          </View>
        }
        empty={{
          icon: 'bell',
          title: filter === 'unread' ? t('empty_unread') : t('empty'),
          body: filter === 'unread' ? undefined : t(inbox.portal ? 'empty_body_customer' : 'empty_body'),
        }}
        renderItem={({ item }) => <Row n={item} date={when(item.created_at, i18n.language, t)} onPress={() => open(item)} />}
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
      className={cn(
        'mx-4 flex-row items-start gap-3 rounded-2xl border p-4 active:opacity-80',
        unread ? 'border-primary/25 bg-primary/5' : 'border-border bg-card',
      )}
      style={{ borderCurve: 'continuous' }}>
      <View className={cn('size-10 items-center justify-center rounded-full', unread ? 'bg-primary/15' : 'bg-muted')}>
        <Icon name={notificationIcon(n)} tone={unread ? 'primary' : 'muted-foreground'} size={18} />
      </View>
      <View className="flex-1 gap-1">
        <View className="flex-row items-start gap-2">
          <Text variant="headline" numberOfLines={2} className={cn('flex-1', !unread && 'font-medium')}>
            {n.title}
          </Text>
          {unread ? <View className="mt-1.5 size-2 rounded-full bg-primary" /> : null}
        </View>
        {n.body ? (
          <Text variant="callout" tone="muted" numberOfLines={3}>
            {n.body}
          </Text>
        ) : null}
        <Text variant="footnote" tone="muted">
          {date}
        </Text>
      </View>
    </Pressable>
  );
}
