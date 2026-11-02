import { type Href, router } from 'expo-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Switch } from 'react-native';

import { List, ListRow, SectionHeader } from '@/ui/list';

import { feedbackSupported } from './native';
import { ShareZipButton } from './share-button';
import { useFeedback } from './store';

/** Settings section: shake-to-report on/off, saved records, ZIP export. */
export function FeedbackSettings() {
  const { t } = useTranslation('feedback');
  const enabled = useFeedback((s) => s.enabled);
  const records = useFeedback((s) => s.records);
  // A registry lookup only; no native module is loaded here.
  const [supported] = useState(feedbackSupported);
  const count = records?.length ?? 0;

  useEffect(() => {
    useFeedback.getState().refresh();
  }, []);

  return (
    <>
      <SectionHeader title={t('section')} />
      <List>
        <ListRow
          icon="shake"
          title={t('shake_title')}
          subtitle={!supported ? t('needs_update') : enabled ? t('shake_on') : t('shake_off')}
          trailing={
            <Switch
              value={supported && enabled}
              disabled={!supported}
              onValueChange={(on) => useFeedback.getState().setEnabled(on)}
              accessibilityLabel={t('shake_title')}
            />
          }
        />
        <ListRow
          icon="inbox"
          title={t('records')}
          subtitle={t('records_count', { count })}
          onPress={() => router.push('/feedback' as Href)}
          last
        />
      </List>
      <ShareZipButton />
    </>
  );
}
