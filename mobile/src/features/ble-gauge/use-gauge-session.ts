import * as Haptics from 'expo-haptics';
import { useEffect, useRef } from 'react';
import { AppState } from 'react-native';

import { useSession } from '@/core/auth/session';

import type { ButtonEvent } from './button';
import {
  isGaugeAvailable,
  isPermissionPromptOpen,
  pauseCommits,
  reconnectLast,
  shutdown,
  subscribeButton,
  subscribeCommits,
} from './manager';
import type { Commit } from './readings';
import { useGauge } from './store';

/** Key under which the last gauge is remembered: one per signed-in user. */
export function useGaugeUserKey(): string | null {
  return useSession((s) => s.me?.user.uuid ?? null);
}

/**
 * App-wide gauge lifetime, mounted once in the signed-in shell: the
 * connection survives moving between tabs and screens (like the old app) and
 * only ends on "Disconnect", sign-out (the shell unmounts, or the user
 * changes) or the app going to the background; a connection that was up then
 * comes back when the app returns.
 */
export function useGaugeAppSession() {
  const userKey = useGaugeUserKey();

  useEffect(() => {
    if (!isGaugeAvailable()) return;
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
      void shutdown();
    };
  }, [userKey]);
}

/**
 * Screens that use the gauge. The connection itself is app-wide
 * (useGaugeAppSession); `pause` holds steady readings back (device and
 * calibration screens opened over the measuring form).
 */
export function useGaugeHold({ pause = false }: { pause?: boolean } = {}) {
  useEffect(() => {
    if (!pause || !isGaugeAvailable()) return;
    return pauseCommits();
  }, [pause]);
}

/**
 * Binds the gauge to a measuring screen: steady readings go to `onCommit`
 * while the screen is mounted (the connection itself is app-wide).
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

/**
 * The gauge's button in the air while a measuring screen is mounted
 * (button.ts): `short` / `long`, with a haptic so the user feels it without
 * looking at the phone.
 */
export function useGaugeButtons(onButton: (e: ButtonEvent) => void) {
  const handler = useRef(onButton);
  useEffect(() => {
    handler.current = onButton;
  });
  useEffect(() => {
    if (!isGaugeAvailable()) return;
    return subscribeButton((e) => {
      void (e === 'long'
        ? Haptics.notificationAsync(Haptics.NotificationFeedbackType.Warning)
        : Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium)
      ).catch(() => undefined);
      handler.current(e);
    });
  }, []);
}
