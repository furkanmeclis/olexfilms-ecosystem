import { Download } from '@/ui/icons';
import { useTranslation } from 'react-i18next';
import { Linking, View } from 'react-native';

import { useAppStatus } from '@/core/app-status';
import { EmptyState } from '@/ui/empty-state';

/** Shown on 426: the backend no longer accepts this app version. Queued work is kept. */
export function UpdateRequiredScreen() {
  const { t } = useTranslation('update');
  const storeUrl = useAppStatus((s) => s.updateRequired?.storeUrl);
  return (
    <View className="flex-1 justify-center bg-background">
      <EmptyState
        icon={Download}
        title={t('title')}
        body={t('body')}
        action={storeUrl ? { title: t('button'), onPress: () => void Linking.openURL(storeUrl) } : undefined}
      />
    </View>
  );
}
