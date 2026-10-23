import * as Haptics from 'expo-haptics';
import { useEffect, useRef } from 'react';
import { AppState } from 'react-native';

import { useSession } from '@/core/auth/session';

import { isGaugeAvailable, isPermissionPromptOpen, pauseCommits, reconnectLast, shutdown, subscribeCommits } from './manager';
import type { Commit } from './readings';
import { useGauge } from './store';

/** Key under which the last gauge is remembered: one per signed-in user. */
export function useGaugeUserKey(): string | null {
  return useSession((s) => s.me?.user.uuid ?? null);
}

/** Screens that keep the gauge connected; it is dropped when the last one leaves. */
let holders = 0;

/**
 * Keeps the gauge connection while the screen is mounted: dropped when the
 * last holding screen unmounts or the app goes to the background, restored
 * when it comes back. `pause` holds steady readings back (device and
 * calibration screens opened over the measuring form).
 */
export function useGaugeHold({ pause = false }: { pause?: boolean } = {}) {
  const userKey = useGaugeUserKey();

  useEffect(() => {
    if (!isGaugeAvailable()) return;
    holders += 1;
    let resume = false;
    const app = AppState.addEventListener('change', (next) => {
      // Android's permission dialog backgrounds the app; that is not leaving it.
      if (isPermissionPromptOpen()) return;
      if (next === 'background') {
        const { status } = useGauge.getState();
        resume = resume || status === 'connected' || status === 'connecting';
        void shutdown();
      } else if (next === 'active' && resume) {
        resume = false;
        void reconnectLast(userKey);
      }
    });
    return () => {
      app.remove();
      holders -= 1;
      if (holders === 0) void shutdown();
    };
  }, [userKey]);

  useEffect(() => {
    if (!pause || !isGaugeAvailable()) return;
    return pauseCommits();
  }, [pause]);
}

/**
 * Binds the gauge to a measuring screen: steady readings go to `onCommit`,
 * the connection lives as long as the screen (useGaugeHold).
 */
export function useGaugeSession(onCommit: (c: Commit) => void) {
  const handler = useRef(onCommit);
  useEffect(() => {
    handler.current = onCommit;
  });
  useGaugeHold();

  useEffect(() => {
    if (!isGaugeAvailable()) return;
    return subscribeCommits((c) => {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success).catch(() => undefined);
      handler.current(c);
    });
  }, []);
}
