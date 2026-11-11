import { useEffect, useRef } from 'react';
import { AppState } from 'react-native';

import { useSession } from '@/core/auth/session';

import { callerIdNative } from './native';
import { useCallerId } from './store';
import { syncCallerId } from './sync';

/**
 * Signed-in shell hook: syncs the caller ID list on launch, when the app
 * returns to the foreground (at most every 15 minutes) and at once when the
 * user or the organization changes. Does nothing while the feature is off.
 */
export function useCallerIdSync() {
  const user = useSession((s) => s.me?.user.uuid);
  const organization = useSession((s) => s.me?.active_organization_uuid);
  const key = `${user}|${organization}`;
  const lastKey = useRef<string | null>(null);

  useEffect(() => {
    if (!callerIdNative.available) return;
    const changed = lastKey.current !== null && lastKey.current !== key;
    lastKey.current = key;
    void syncCallerId({ force: changed });
  }, [key]);

  useEffect(() => {
    if (!callerIdNative.available) return;
    const sub = AppState.addEventListener('change', (state) => {
      if (state !== 'active') return;
      void syncCallerId();
      // The user may come back from the system settings.
      if (useCallerId.getState().enabledBy) void useCallerId.getState().refreshSystem();
    });
    return () => sub.remove();
  }, []);
}
