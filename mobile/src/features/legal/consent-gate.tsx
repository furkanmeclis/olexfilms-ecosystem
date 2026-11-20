import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal, ScrollView, Switch, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { api, unwrap } from '@/core/api/client';
import { Button } from '@/ui/button';
import { Text } from '@/ui/text';

import { MarkdownView } from './markdown-view';
import { pendingConsent } from './lib';

const pendingKey = (locale: string) => ['portal', 'consents', 'pending', locale];

/**
 * After sign-in the customer is asked the AI guidelines once per text version
 * (GET /v1/portal/consents/pending; K22, TEC-1257), like the web portal's
 * AIConsentDialog. The switch starts off; "Continue" records its state
 * (accept or decline). Declining never locks the app, only the AI features.
 */
export function ConsentGate() {
  const { t, i18n } = useTranslation('legal');
  const insets = useSafeAreaInsets();
  const qc = useQueryClient();
  const locale = i18n.language;
  const [accepted, setAccepted] = useState(false);

  const q = useQuery({
    queryKey: pendingKey(locale),
    queryFn: () => unwrap(api.GET('/v1/portal/consents/pending', { params: { query: { locale } } })),
    staleTime: 5 * 60_000,
    retry: 1,
  });
  const text = pendingConsent(q.data?.items);

  const decide = useMutation({
    mutationFn: () => {
      if (!text) return Promise.resolve(null);
      return unwrap(
        api.POST('/v1/portal/consents', {
          body: { kind: text.kind, locale: text.locale, version: text.version, accepted },
        }),
      );
    },
    onSuccess: () => {
      setAccepted(false);
      qc.setQueryData(pendingKey(locale), { items: (q.data?.items ?? []).filter((i) => i !== text) });
      void qc.invalidateQueries({ queryKey: ['portal', 'consents'] });
    },
  });

  if (!text) return null;
  return (
    // No close gesture: the user answers (accept or decline) to go on.
    <Modal visible animationType="slide" presentationStyle="fullScreen" onRequestClose={() => undefined}>
      <View className="flex-1 bg-background" style={{ paddingTop: insets.top + 12, paddingBottom: Math.max(insets.bottom, 16) }}>
        <View className="gap-1 px-5 pb-3">
          <Text variant="title">{t('consent_title')}</Text>
          <Text variant="footnote" tone="muted">
            {t('consent_note')}
          </Text>
        </View>
        <ScrollView className="flex-1" contentContainerClassName="px-5 pb-6">
          <MarkdownView source={text.body} />
        </ScrollView>
        <View className="gap-4 border-t border-border px-5 pt-4">
          <View className="flex-row items-center gap-3">
            <Switch value={accepted} onValueChange={setAccepted} accessibilityLabel={t('consent_checkbox')} />
            <Text variant="callout" className="flex-1" onPress={() => setAccepted((v) => !v)}>
              {t('consent_checkbox')}
            </Text>
          </View>
          {decide.isError ? (
            <Text variant="footnote" tone="danger">
              {t('consent_failed')}
            </Text>
          ) : null}
          <Button size="lg" title={t('consent_continue')} loading={decide.isPending} onPress={() => decide.mutate()} />
        </View>
      </View>
    </Modal>
  );
}
