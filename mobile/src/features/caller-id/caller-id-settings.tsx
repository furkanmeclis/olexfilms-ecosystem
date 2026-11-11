import { router } from 'expo-router';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import { useSession } from '@/core/auth/session';
import { List, ListRow, SectionHeader } from '@/ui/list';

import { callerIdNative } from './native';
import { useCallerId } from './store';
import { callerIdEligible } from './sync';

/** One-line state of the feature, shared by the settings row and the screen. */
export function useCallerIdState(): 'off' | 'syncing' | 'needs_permission' | 'ready' {
  const me = useSession((s) => s.me);
  const enabledBy = useCallerId((s) => s.enabledBy);
  const busy = useCallerId((s) => s.busy);
  const system = useCallerId((s) => s.system);
  if (!me || enabledBy !== me.user.uuid) return 'off';
  if (system && !system.ready) return 'needs_permission';
  if (busy) return 'syncing';
  return 'ready';
}

/**
 * Settings section that opens the caller ID screen. Hidden for customers,
 * for roles that cannot read customers and in builds without the native
 * module.
 */
export function CallerIdSettings() {
  const { t } = useTranslation('callerid');
  const me = useSession((s) => s.me);
  const state = useCallerIdState();
  const visible = callerIdNative.available && callerIdEligible(me);

  useEffect(() => {
    if (visible) void useCallerId.getState().refreshSystem();
  }, [visible]);

  if (!visible) return null;
  return (
    <>
      <SectionHeader title={t('settings_section')} />
      <List>
        <ListRow
          icon="phone"
          title={t('title')}
          subtitle={t(`state_${state}`)}
          onPress={() => router.push('/caller-id')}
          last
        />
      </List>
    </>
  );
}
