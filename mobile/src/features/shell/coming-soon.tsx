import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';

import { EmptyState } from '@/ui/empty-state';
import { Screen } from '@/ui/screen';

/** Placeholder for a tab whose screens are built in a later step. */
export function ComingSoon({ titleKey }: { titleKey: string }) {
  const { t } = useTranslation();
  return (
    <>
      <Stack.Screen options={{ title: t(titleKey) }} />
      <Screen>
        <EmptyState icon="tools" title={t('common:coming_soon_title')} body={t('common:coming_soon_body')} />
      </Screen>
    </>
  );
}

export const comingSoon = (titleKey: string) =>
  function Placeholder() {
    return <ComingSoon titleKey={titleKey} />;
  };
