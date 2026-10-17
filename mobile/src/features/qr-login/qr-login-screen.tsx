import { useMutation, useQuery } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { MOBILE_API_VERSION } from '@/core/config';
import { errorMessage } from '@/core/i18n';
import { activeMembership } from '@/core/spaces';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

const header = { 'X-Mobile-Api-Version': MOBILE_API_VERSION };

/** Approve a web panel sign-in by scanning its QR code (design.md §8, mobile scope). */
export function QrLoginScreen() {
  const { t, i18n } = useTranslation('qr');
  const { code } = useLocalSearchParams<{ code: string }>();
  const org = activeMembership(useSession((s) => s.me));

  const scan = useQuery({
    queryKey: ['qr-login', code],
    queryFn: () => unwrap(api.GET('/v1/mobile/auth/qr/{code}', { params: { header, path: { code } } })),
    gcTime: 0,
  });

  const decide = useMutation({
    mutationFn: (approve: boolean) =>
      unwrap(
        approve
          ? api.POST('/v1/mobile/auth/qr/{code}/approve', { params: { header, path: { code } } })
          : api.POST('/v1/mobile/auth/qr/{code}/reject', { params: { header, path: { code } } }),
      ),
    onSuccess: (_, approve) => {
      void Haptics.notificationAsync(
        approve ? Haptics.NotificationFeedbackType.Success : Haptics.NotificationFeedbackType.Warning,
      );
      router.back();
    },
  });

  const expired = isApiError(scan.error) && scan.error.status === 410;
  const failure = decide.error ?? scan.error;

  return (
    <>
      <Stack.Screen options={{ title: t('title') }} />
      <Screen className="pt-4">
        {scan.isPending ? (
          <Skeleton className="h-36" />
        ) : scan.data ? (
          <List>
            <ListRow title={t('browser')} subtitle={scan.data.web_user_agent ?? '—'} />
            <ListRow title="IP" subtitle={scan.data.web_ip ?? '—'} />
            <ListRow title={t('organization')} subtitle={org?.name ?? '—'} />
            <ListRow
              title={t('expires')}
              subtitle={new Date(scan.data.expires_at).toLocaleTimeString(i18n.language)}
              last
            />
          </List>
        ) : (
          <EmptyState
            icon="devices"
            title={expired ? t('expired') : errorMessage(isApiError(failure) ? failure.code : undefined)}
          />
        )}
        {scan.data ? (
          <View className="gap-3">
            <Text variant="footnote" tone="muted" className="text-center">
              {t('warning')}
            </Text>
            {decide.error ? (
              <Text variant="footnote" tone="danger" className="text-center">
                {errorMessage(isApiError(decide.error) ? decide.error.code : undefined)}
              </Text>
            ) : null}
            <View className="flex-row gap-3">
              <Button
                title={t('reject')}
                variant="secondary"
                size="lg"
                className="flex-1"
                disabled={decide.isPending}
                onPress={() => decide.mutate(false)}
              />
              <Button
                title={t('approve')}
                size="lg"
                className="flex-1"
                loading={decide.isPending && decide.variables}
                disabled={decide.isPending}
                onPress={() => decide.mutate(true)}
              />
            </View>
          </View>
        ) : null}
      </Screen>
    </>
  );
}
