import { Redirect } from 'expo-router';
import { Building2 } from '@/ui/icons';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { useResolvedSpace } from '@/core/spaces';
import { EmptyState } from '@/ui/empty-state';

/** Opens the last used space; a user without any membership gets a clear message. */
export default function Entry() {
  const { t } = useTranslation();
  const space = useResolvedSpace();
  if (space) return <Redirect href={`/${space}`} />;
  return (
    <View className="flex-1 justify-center bg-background">
      <EmptyState
        icon={Building2}
        title={t('spaces:none_title')}
        body={t('spaces:none_body')}
        action={{ title: t('settings:sign_out'), onPress: () => void useSession.getState().signOut() }}
      />
    </View>
  );
}
