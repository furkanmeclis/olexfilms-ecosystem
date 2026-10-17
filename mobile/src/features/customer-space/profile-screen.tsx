import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Alert, RefreshControl, Switch, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { errorMessage } from '@/core/i18n';
import { errorText } from '@/features/services/ui';
import { UserRound } from '@/ui/icons';
import { Icon } from '@/ui/icon';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

import { isHidden, type NotificationPreferences, retryPortal } from './lib';

type Flag = 'push_enabled' | 'email_enabled' | 'inapp_enabled';
const FLAGS: Flag[] = ['push_enabled', 'email_enabled', 'inapp_enabled'];
const PREFS_KEY = ['portal', 'notification-preferences'];

/** Notification channels (GET/PUT /v1/portal/notification-preferences); rules are sent back unchanged. */
function NotificationPrefs() {
  const { t } = useTranslation('customer');
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: PREFS_KEY,
    retry: retryPortal,
    queryFn: () => unwrap(api.GET('/v1/portal/notification-preferences')),
  });
  const save = useMutation({
    mutationFn: (body: NotificationPreferences) => unwrap(api.PUT('/v1/portal/notification-preferences', { body })),
    onMutate: async (body) => {
      await qc.cancelQueries({ queryKey: PREFS_KEY });
      const prev = qc.getQueryData<NotificationPreferences>(PREFS_KEY);
      qc.setQueryData(PREFS_KEY, body);
      return { prev };
    },
    onError: (e, _b, ctx) => {
      if (ctx?.prev) qc.setQueryData(PREFS_KEY, ctx.prev);
      Alert.alert(errorMessage(isApiError(e) ? e.code : undefined));
    },
    onSuccess: (data) => qc.setQueryData(PREFS_KEY, data),
  });

  if (isHidden(q.error)) return null;
  const p = q.data;
  return (
    <>
      <SectionHeader title={t('notifications')} />
      {!p ? (
        q.isPending ? (
          <Skeleton className="h-36 rounded-lg" />
        ) : (
          <List>
            <ListRow title={errorText(q.error)} subtitle={t('common:retry')} onPress={() => void q.refetch()} last />
          </List>
        )
      ) : (
        <List>
          {FLAGS.map((f, i) => (
            <ListRow
              key={f}
              title={t(`notify_${f}`)}
              last={i === FLAGS.length - 1}
              trailing={
                <Switch
                  value={!!p[f]}
                  disabled={save.isPending}
                  onValueChange={(v) => save.mutate({ ...p, [f]: v, rules: p.rules ?? null })}
                />
              }
            />
          ))}
        </List>
      )}
    </>
  );
}

export function CustomerProfileScreen() {
  const { t } = useTranslation('customer');
  const qc = useQueryClient();
  const user = useSession((s) => s.me?.user);
  const name = [user?.name, user?.surname].filter(Boolean).join(' ');

  const confirmSignOut = () =>
    Alert.alert(t('sign_out_confirm'), undefined, [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t('sign_out'), style: 'destructive', onPress: () => void useSession.getState().signOut() },
    ]);

  return (
    <>
      <Stack.Screen options={{ title: t('profile') }} />
      <Screen
        refreshControl={
          <RefreshControl refreshing={false} onRefresh={() => void qc.invalidateQueries({ queryKey: PREFS_KEY })} />
        }>
        <View className="items-center gap-2 py-4">
          <View className="size-16 items-center justify-center rounded-full bg-secondary">
            <Icon as={UserRound} tone="primary" size={30} />
          </View>
          <Text variant="headline">{name || t('profile')}</Text>
          {user?.email ? (
            <Text variant="footnote" tone="muted" selectable>
              {user.email}
            </Text>
          ) : null}
        </View>

        <NotificationPrefs />

        <SectionHeader title={t('app')} />
        <List>
          <ListRow title={t('settings')} subtitle={t('settings_hint')} onPress={() => router.push('/settings')} last />
        </List>
        <List>
          <ListRow title={t('sign_out')} tone="danger" onPress={confirmSignOut} last />
        </List>
      </Screen>
    </>
  );
}
