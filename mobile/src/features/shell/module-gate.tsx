import { router } from 'expo-router';
import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { moduleOfRoute, useModule } from '@/core/modules';
import { EmptyState } from '@/ui/empty-state';

/**
 * Wraps every screen of a tab stack: a screen whose module is switched off
 * (reached through a notification, search hit or old link) shows a plain
 * "module closed" message instead of failing on its first request.
 */
export function ModuleGate({ routeName, module, children }: { routeName: string; module?: string; children: ReactElement }) {
  const { t } = useTranslation();
  const on = useModule(module ?? moduleOfRoute(routeName));
  if (on) return children;
  return (
    <View className="flex-1 justify-center bg-background">
      <EmptyState
        icon="blocked"
        title={t('module_closed_title')}
        body={t('module_closed_body')}
        action={router.canGoBack() ? { title: t('back'), onPress: () => router.back() } : undefined}
      />
    </View>
  );
}
