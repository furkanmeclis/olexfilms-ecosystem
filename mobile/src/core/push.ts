import { useQueryClient } from '@tanstack/react-query';
import Constants from 'expo-constants';
import * as Device from 'expo-device';
import * as Notifications from 'expo-notifications';
import { AndroidImportance } from 'expo-notifications';
import { router } from 'expo-router';
import { useEffect, useRef } from 'react';
import { Platform } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';
import { APP_VERSION, MOBILE_API_VERSION, PLATFORM } from '@/core/config';
import { kindsOf, useResolvedSpace } from '@/core/spaces';
import { notificationRoute, notificationSpace, pushNotification } from '@/features/notifications/lib';

/** Pushes that arrive while the app is open still show as a banner. */
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowBanner: true,
    shouldShowList: true,
    shouldPlaySound: true,
    shouldSetBadge: false,
  }),
});

const params = { header: { 'X-Mobile-Api-Version': MOBILE_API_VERSION } };

/**
 * Asks for the notification permission (once; the OS remembers the answer),
 * reads the Expo push token and registers it for the session's device
 * (`PUT /v1/mobile/push-token`; logout revokes it). Simulators, a refused
 * permission or a missing EAS project id register nothing.
 */
export async function registerPushToken(): Promise<string | null> {
  if (!Device.isDevice) return null;
  if (Platform.OS === 'android') {
    await Notifications.setNotificationChannelAsync('default', {
      name: 'Olexfilms',
      importance: AndroidImportance.HIGH,
    });
  }
  let { status } = await Notifications.getPermissionsAsync();
  if (status !== 'granted') ({ status } = await Notifications.requestPermissionsAsync());
  if (status !== 'granted') return null;
  const projectId = Constants.expoConfig?.extra?.eas?.projectId ?? Constants.easConfig?.projectId;
  if (!projectId) return null;
  const { data: token } = await Notifications.getExpoPushTokenAsync({ projectId });
  await unwrap(
    api.PUT('/v1/mobile/push-token', {
      params,
      body: { expo_push_token: token, platform: PLATFORM, app_version: APP_VERSION },
    }),
  );
  return token;
}

/**
 * Signed-in shell hook: registers the push token for the user and
 * organization (again after a switch), refreshes the inbox when a push
 * arrives and opens the record (e.g. the order) when one is tapped.
 */
export function usePushNotifications() {
  const qc = useQueryClient();
  const me = useSession((s) => s.me);
  const space = useResolvedSpace();
  const key = me ? `${me.user.uuid}:${me.active_organization_uuid ?? ''}` : null;
  const registered = useRef<string | null>(null);
  // The tap handler reads the latest session without re-subscribing.
  const latest = useRef({ me, space });
  useEffect(() => {
    latest.current = { me, space };
  }, [me, space]);

  useEffect(() => {
    if (!key || registered.current === key) return;
    registered.current = key;
    registerPushToken().catch(() => {
      // Retried on the next launch or organization switch.
      registered.current = null;
    });
  }, [key]);

  useEffect(() => {
    const open = (data: Record<string, unknown> | undefined) => {
      const n = pushNotification(data);
      const { me: current, space: open } = latest.current;
      const href = notificationRoute(n, notificationSpace(n.event_code, open, kindsOf(current)));
      if (href) router.push(href as never);
    };
    const received = Notifications.addNotificationReceivedListener(() => {
      void qc.invalidateQueries({ queryKey: ['notifications'] });
    });
    const tapped = Notifications.addNotificationResponseReceivedListener((r) => {
      open(r.notification.request.content.data as Record<string, unknown> | undefined);
    });
    // A tap that launched the app from a closed state.
    const last = Notifications.getLastNotificationResponse();
    if (last) {
      open(last.notification.request.content.data as Record<string, unknown> | undefined);
      Notifications.clearLastNotificationResponse();
    }
    return () => {
      received.remove();
      tapped.remove();
    };
  }, [qc]);
}
