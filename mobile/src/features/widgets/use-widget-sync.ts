import { useEffect, useRef } from 'react';
import { AppState } from 'react-native';
import { useTranslation } from 'react-i18next';

import { useSession } from '@/core/auth/session';
import { usePreferences } from '@/core/preferences';

import { syncWidgets } from './sync';

/**
 * Widget'ları tazeler: uygulama öne gelince 15 dakika kısıtıyla, kullanıcı,
 * kuruluş, dil ya da rakam tercihi değişince kısıtı aşarak. Hata sessizdir.
 */
export function useWidgetSync() {
  const { i18n } = useTranslation();
  const user = useSession((s) => s.me?.user.uuid);
  const organization = useSession((s) => s.me?.active_organization_uuid);
  const digits = usePreferences((s) => s.digits);
  const key = `${user}|${organization}|${i18n.language}|${digits}`;
  const lastKey = useRef<string | null>(null);

  useEffect(() => {
    // Açılıştaki ilk çalıştırma kısıta tabidir; sonraki değişimler zorlar.
    const force = lastKey.current !== null && lastKey.current !== key;
    lastKey.current = key;
    void syncWidgets(useSession.getState().me, { force });
  }, [key]);

  useEffect(() => {
    const sub = AppState.addEventListener('change', (state) => {
      if (state === 'active') void syncWidgets(useSession.getState().me, { force: false });
    });
    return () => sub.remove();
  }, []);
}
