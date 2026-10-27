import { Redirect } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { startPath } from '@/core/space-memory';
import { EmptyState } from '@/ui/empty-state';

/** Opens where the user left off (space and tab, TEC-1027); without any membership, a clear message. */
export default function Entry() {
  const { t } = useTranslation();
  const path = startPath(useSession((s) => s.me));
  if (path) return <Redirect href={path as never} />;
  return (
    <View className="flex-1 justify-center bg-background">
      <EmptyState
        icon="building"
        title={t('spaces:none_title')}
        body={t('spaces:none_body')}
        action={{ title: t('settings:sign_out'), onPress: () => void useSession.getState().signOut() }}
      />
    </View>
  );
}
