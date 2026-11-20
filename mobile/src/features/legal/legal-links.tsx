import { type Href, router } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { List, ListRow, SectionHeader } from '@/ui/list';
import { Text } from '@/ui/text';

import { LEGAL_KINDS, type LegalKind } from './lib';

const open = (kind: LegalKind) => router.push(`/legal/${kind}` as Href);

/** Small links under the sign-in form (TEC-1257; App Store, KVKK). */
export function LegalFooterLinks() {
  const { t } = useTranslation('legal');
  return (
    <View className="flex-row flex-wrap justify-center gap-x-4 gap-y-1" accessibilityRole="list">
      {LEGAL_KINDS.map((kind) => (
        <Text
          key={kind}
          variant="footnote"
          tone="muted"
          accessibilityRole="link"
          className="underline"
          onPress={() => open(kind)}>
          {t(kind)}
        </Text>
      ))}
    </View>
  );
}

/** "Legal" section of the profile and the settings. */
export function LegalListSection() {
  const { t } = useTranslation('legal');
  return (
    <>
      <SectionHeader title={t('section')} />
      <List>
        {LEGAL_KINDS.map((kind, i) => (
          <ListRow key={kind} title={t(kind)} icon="shield" onPress={() => open(kind)} last={i === LEGAL_KINDS.length - 1} />
        ))}
      </List>
    </>
  );
}
