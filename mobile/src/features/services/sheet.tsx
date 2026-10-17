import { useTranslation } from 'react-i18next';
import { ActivityIndicator, KeyboardAvoidingView, Modal, Platform, Pressable, ScrollView, View } from 'react-native';

import { Text } from '@/ui/text';

/**
 * Page-sheet modal with Cancel / title / Save in its own header, for short
 * edit forms (no route needed, works the same in every stack).
 */
export function Sheet({
  title,
  saveTitle,
  busy,
  canSave = true,
  onSave,
  onClose,
  children,
}: {
  title: string;
  saveTitle?: string;
  busy?: boolean;
  canSave?: boolean;
  onSave: () => void;
  onClose: () => void;
  children: React.ReactNode;
}) {
  const { t } = useTranslation('common');
  return (
    <Modal visible animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="flex-1 bg-background">
        <View className="min-h-14 flex-row items-center justify-between gap-3 border-b border-border px-4">
          <Pressable accessibilityRole="button" hitSlop={8} onPress={onClose} disabled={busy}>
            <Text variant="body" tone="primary">
              {t('cancel')}
            </Text>
          </Pressable>
          <Text variant="headline" numberOfLines={1} className="flex-1 text-center">
            {title}
          </Text>
          {busy ? (
            <ActivityIndicator />
          ) : (
            <Pressable accessibilityRole="button" hitSlop={8} onPress={onSave} disabled={!canSave}>
              <Text variant="body" tone="primary" className={canSave ? 'font-semibold' : 'opacity-40'}>
                {saveTitle ?? t('save')}
              </Text>
            </Pressable>
          )}
        </View>
        <ScrollView keyboardShouldPersistTaps="handled" contentContainerClassName="gap-4 p-4 pb-12">
          {children}
        </ScrollView>
      </KeyboardAvoidingView>
    </Modal>
  );
}
