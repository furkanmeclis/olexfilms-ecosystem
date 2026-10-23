import { useMutation, useQueryClient } from '@tanstack/react-query';
import type { Href } from 'expo-router';
import { useSegments } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Alert, View } from 'react-native';

import { errorText } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { Text } from '@/ui/text';

import type { LegalText } from './chat-state';
import { acceptConsent, aiKeys } from './use-assistant-chat';

/** Routes of the assistant screens inside the current space. */
export function useAssistantLinks() {
  const space = (useSegments() as string[])[1] ?? 'bayi';
  const base = `/${space}/menu/assistant`;
  return {
    space,
    chat: (uuid: string) => `${base}/${uuid}` as Href,
    newChat: () => `${base}/new` as Href,
  };
}

/**
 * The AI assistant guidelines: the chat opens only after the user accepts
 * them (POST /v1/consents, the user's own consent record).
 */
export function ConsentCard({ text, onAccepted }: { text: LegalText; onAccepted?: () => void }) {
  const { t } = useTranslation('assistant');
  const qc = useQueryClient();
  const accept = useMutation({
    mutationFn: () => acceptConsent(text),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: aiKeys.status });
      onAccepted?.();
    },
    onError: (e) => Alert.alert(t('consent.failed'), errorText(e)),
  });
  return (
    <View className="gap-3 rounded-lg border border-border bg-card p-4">
      <Text variant="headline">{t('consent.title')}</Text>
      <Text variant="callout" tone="muted">
        {t('consent.description')}
      </Text>
      <Text variant="callout" selectable>
        {text.body}
      </Text>
      <Button title={t('consent.accept')} loading={accept.isPending} onPress={() => accept.mutate()} />
    </View>
  );
}
