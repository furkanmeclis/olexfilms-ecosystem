import { useQuery } from '@tanstack/react-query';
import { router } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { formatDigits } from '@/core/digits';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { useInbox } from './inbox';

/** Home header bell with the unread count; hidden when the session has no inbox. */
export function NotificationBell() {
  const { t, i18n } = useTranslation('notifications');
  const inbox = useInbox();
  const can = inbox.enabled;
  const unread = useQuery({
    queryKey: ['notifications', 'unread-count'],
    queryFn: inbox.unreadCount,
    enabled: can,
    refetchInterval: 60_000,
    staleTime: 30_000,
  });
  if (!can) return null;
  const count = unread.data?.count ?? 0;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={count > 0 ? t('bell_unread', { count }) : t('bell')}
      hitSlop={8}
      onPress={() => router.push('/notifications')}>
      <View className="size-8 items-center justify-center">
        <Icon name="bell" tone="primary" />
        {count > 0 ? (
          <View className="absolute -right-0.5 -top-0.5 min-w-4 items-center rounded-full bg-destructive px-1">
            <Text variant="caption" className="text-[10px] font-bold text-primary-foreground">
              {count > 99 ? `${formatDigits(99, i18n.language)}+` : formatDigits(count, i18n.language)}
            </Text>
          </View>
        ) : null}
      </View>
    </Pressable>
  );
}
