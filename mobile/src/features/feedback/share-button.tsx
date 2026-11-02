import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from 'react-native';

import { Button } from '@/ui/button';

import { SharingUnavailableError, shareFeedbackZip } from './export';
import { useFeedback } from './store';

/** "Create ZIP and share": off while there is nothing to share. Records stay after sharing. */
export function ShareZipButton({ className }: { className?: string }) {
  const { t } = useTranslation('feedback');
  const records = useFeedback((s) => s.records);
  const [busy, setBusy] = useState(false);

  const share = async () => {
    if (!records?.length) return;
    setBusy(true);
    try {
      await shareFeedbackZip(records, t('share_zip'));
    } catch (e) {
      Alert.alert(
        t('share_failed'),
        e instanceof SharingUnavailableError ? t('needs_update') : e instanceof Error ? e.message : String(e),
      );
    } finally {
      setBusy(false);
    }
  };

  return (
    <Button
      title={t('share_zip')}
      icon="share"
      variant="outline"
      disabled={!records?.length}
      loading={busy}
      onPress={() => void share()}
      className={className}
    />
  );
}
