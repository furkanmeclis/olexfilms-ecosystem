import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Switch } from 'react-native';

import { List, ListRow, SectionHeader } from '@/ui/list';

import { usePlateCapability } from './capability';

/** Settings section: opt in to on-device plate recognition (downloads the model where needed). */
export function PlateRecognitionSettings() {
  const { t } = useTranslation('plate');
  const { optIn, status, busy, progress, error } = usePlateCapability();

  useEffect(() => {
    if (status === 'unknown') void usePlateCapability.getState().refresh();
  }, [status]);

  let subtitle: string;
  if (status === 'unsupported') subtitle = t('settings_unsupported');
  else if (busy && progress != null) subtitle = t('settings_downloading', { value: Math.round(progress * 100) });
  else if (busy) subtitle = t('settings_preparing');
  // Opted in but Play services dropped the model: turning it on again re-downloads.
  else if (error || (optIn && status === 'not_installed')) subtitle = t('settings_error');
  else if (optIn) subtitle = t('settings_on');
  else subtitle = t('settings_off');

  const toggle = (on: boolean) => {
    const s = usePlateCapability.getState();
    void (on ? s.enable() : s.disable());
  };

  return (
    <>
      <SectionHeader title={t('settings_section')} />
      <List>
        <ListRow
          icon="scan"
          title={t('settings_title')}
          subtitle={subtitle}
          trailing={
            status === 'unsupported' ? null : busy ? (
              <ActivityIndicator />
            ) : (
              <Switch value={optIn} onValueChange={toggle} accessibilityLabel={t('settings_title')} />
            )
          }
          last
        />
      </List>
    </>
  );
}
