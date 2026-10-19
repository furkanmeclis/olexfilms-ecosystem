import * as Haptics from 'expo-haptics';
import { useEffect, useRef } from 'react';
import { AppState } from 'react-native';

import { useSession } from '@/core/auth/session';

import { isGaugeAvailable, isPermissionPromptOpen, reconnectLast, shutdown, subscribeCommits } from './manager';
import type { Commit } from './readings';
import { useGauge } from './store';

/** Key under which the last gauge is remembered: one per signed-in user. */
export function useGaugeUserKey(): string | null {
  return useSession((s) => s.me?.user.uuid ?? null);
}

/**
 * Binds the gauge to a measuring screen: steady readings go to `onCommit`,
 * the connection is dropped when the screen unmounts or the app goes to the
 * background, and restored when it comes back.
 */
export function useGaugeSession(onCommit: (c: Commit) => void) {
  const userKey = useGaugeUserKey();
  const handler = useRef(onCommit);
  useEffect(() => {
    handler.current = onCommit;
  });

  useEffect(() => {
    if (!isGaugeAvailable()) return;
    const unsub = subscribeCommits((c) => {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success).catch(() => undefined);
      handler.current(c);
    });
    let resume = false;
    const app = AppState.addEventListener('change', (next) => {
      // Android's permission dialog backgrounds the app; that is not leaving it.
      if (isPermissionPromptOpen()) return;
      if (next === 'background') {
        const { status } = useGauge.getState();
        resume = status === 'connected' || status === 'connecting';
        void shutdown();
      } else if (next === 'active' && resume) {
        resume = false;
        void reconnectLast(userKey);
      }
    });
    return () => {
      unsub();
      app.remove();
      void shutdown();
    };
  }, [userKey]);
}
