import { useTranslation } from 'react-i18next';
import { View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { applyUpdate, useUpdateStore } from '@/core/updates';
import { Button } from '@/ui/button';
import { Text } from '@/ui/text';

/**
 * Non-blocking "new version ready" strip under the status bar (never covers the tab bar). An OTA update is already downloaded when this shows;
 * ignoring it just applies the update on the next cold start.
 */
export function UpdateBanner() {
  const { t } = useTranslation('update');
  const insets = useSafeAreaInsets();
  const visible = useUpdateStore((s) => s.ready && !s.dismissed);
  if (!visible) return null;
  return (
    <View
      className="absolute inset-x-3 flex-row items-center gap-2 rounded-xl border border-border bg-card px-4 py-1"
      style={{ top: insets.top + 4, borderCurve: 'continuous' }}>
      <Text variant="callout" className="flex-1 font-semibold">
        {t('ota_ready')}
      </Text>
      <Button title={t('ota_later')} variant="ghost" onPress={() => useUpdateStore.getState().dismiss()} />
      <Button title={t('ota_restart')} onPress={() => void applyUpdate()} />
    </View>
  );
}
