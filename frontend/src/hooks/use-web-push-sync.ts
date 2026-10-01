"use client";

import { useEffect, useRef } from "react";

import {
  hasBrowserPushSupport,
  registerWebPush,
} from "@/lib/web-push/register";
import { notificationPreferencesService } from "@/services/notification-preferences.service";

/**
 * Silently refresh the browser push subscription when the user opted in
 * and already granted notification permission. Does not show a prompt.
 */
export function useWebPushSync(enabled = true) {
  const syncedRef = useRef(false);

  useEffect(() => {
    if (!enabled || syncedRef.current) return;
    // iOS Safari (not added to the home screen) has no Notification at all;
    // reading it unguarded crashes every page behind the app layout.
    if (!hasBrowserPushSupport() || Notification.permission !== "granted") {
      return;
    }

    let cancelled = false;
    syncedRef.current = true;

    (async () => {
      try {
        const prefs = await notificationPreferencesService.get();
        if (cancelled || !prefs.push_enabled) return;
        await registerWebPush({ silent: true });
      } catch {
        // Ignore sync failures; user can re-save preferences from profile.
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [enabled]);
}
