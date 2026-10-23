import { useMutation, useQueryClient } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Alert, FlatList, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { formatDateTime } from '@/features/services/lib';
import { ErrorState, errorText, ListBody, ListSkeleton } from '@/features/services/ui';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { type AIConversation, type AIStatus, quotaRemainingPercent, quotaUsedUp } from './chat-state';
import { ConsentCard, useAssistantLinks } from './consent-card';
import { aiKeys, useAssistantStatus, useConversations } from './use-assistant-chat';

/** Why the chat cannot open, or null when it can. */
export function StatusGate({ status }: { status: AIStatus }) {
  const { t } = useTranslation('assistant');
  if (!status.enabled) return <EmptyState icon="assistant" title={t('unavailable.title')} body={t('unavailable.body')} />;
  if (!status.allowed) return <EmptyState icon="blocked" title={t('unavailable.title')} body={t('unavailable.forbidden')} />;
  if (status.consent_required && status.consent) return <ConsentCard text={status.consent} />;
  return null;
}

/** Quota line, or the used-up notice. */
export function QuotaNote({ status, hit }: { status: AIStatus; hit?: boolean }) {
  const { t } = useTranslation('assistant');
  if (hit || quotaUsedUp(status.quota)) {
    return (
      <View className="gap-1 rounded-lg border border-destructive bg-destructive/10 p-3">
        <Text variant="callout" className="font-semibold">
          {t('quota.exceeded_title')}
        </Text>
        <Text variant="footnote" tone="muted">
          {t('quota.exceeded_body')}
        </Text>
      </View>
    );
  }
  const percent = quotaRemainingPercent(status.quota);
  return percent === null ? null : (
    <Text variant="footnote" tone="muted">
      {t('quota.remaining', { percent })}
    </Text>
  );
}

/** Panel AI assistant: guidelines, quota and the conversation history. */
export function AssistantScreen() {
  const { t, i18n } = useTranslation('assistant');
  const links = useAssistantLinks();
  const qc = useQueryClient();
  const status = useAssistantStatus();
  const s = status.data;
  const usable = !!s && s.enabled && s.allowed && !s.consent_required;
  const conversations = useConversations(usable);

  const remove = useMutation({
    mutationFn: (uuid: string) => unwrap(api.DELETE('/v1/ai/conversations/{uuid}', { params: { path: { uuid } } })),
    onSuccess: (_, uuid) =>
      qc.setQueryData<AIConversation[]>(aiKeys.conversations, (list) => list?.filter((c) => c.uuid !== uuid)),
    onError: (e) => Alert.alert(t('errors.generic'), errorText(e)),
  });
  const askDelete = (c: AIConversation) =>
    Alert.alert(t('history.delete_title'), t('history.delete_body', { title: c.title || t('history.untitled') }), [
      { text: t('history.cancel'), style: 'cancel' },
      { text: t('history.delete'), style: 'destructive', onPress: () => remove.mutate(c.uuid) },
    ]);

  return (
    <>
      <Stack.Screen
        options={{
          title: t('page.title'),
          headerRight: usable
            ? () => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('history.new')}
                  hitSlop={10}
                  onPress={() => router.push(links.newChat())}>
                  <Icon name="add" tone="primary" />
                </Pressable>
              )
            : undefined,
        }}
      />
      {!s ? (
        <View className="flex-1 bg-background">
          {status.isPending ? <ListSkeleton rows={3} /> : <ErrorState error={status.error} onRetry={() => void status.refetch()} />}
        </View>
      ) : (
        <FlatList
          data={usable ? (conversations.data ?? []) : []}
          keyExtractor={(c) => c.uuid}
          contentInsetAdjustmentBehavior="automatic"
          className="flex-1 bg-background"
          contentContainerClassName="gap-2 px-4 pb-10"
          ListHeaderComponent={
            <View className="gap-3 pb-1 pt-2">
              <Text variant="callout" tone="muted">
                {t('page.description')}
              </Text>
              <StatusGate status={s} />
              {usable ? <QuotaNote status={s} /> : null}
              {usable && (conversations.data?.length ?? 0) > 0 ? (
                <Text variant="caption" tone="muted">
                  {t('history.title')}
                </Text>
              ) : null}
            </View>
          }
          ListEmptyComponent={
            !usable ? undefined : (
              <ListBody
                isPending={conversations.isPending}
                error={conversations.error}
                count={0}
                onRetry={() => void conversations.refetch()}
                empty={{
                  icon: 'assistant',
                  title: t('history.empty'),
                  body: t('chat.empty'),
                  action: { title: t('history.new'), onPress: () => router.push(links.newChat()) },
                }}
              />
            )
          }
          renderItem={({ item: c }) => (
            <Pressable
              accessibilityRole="button"
              accessibilityHint={t('history.delete_hint')}
              onPress={() => router.push(links.chat(c.uuid))}
              onLongPress={() => askDelete(c)}
              className="gap-1 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
              <Text variant="headline" numberOfLines={1}>
                {c.title || t('history.untitled')}
              </Text>
              <Text variant="footnote" tone="muted">
                {[
                  t('history.messages', { count: c.message_count }),
                  formatDateTime(c.last_message_at ?? c.updated_at, i18n.language),
                ].join(' · ')}
              </Text>
            </Pressable>
          )}
          refreshControl={
            <RefreshControl
              refreshing={status.isRefetching || conversations.isRefetching}
              onRefresh={() => {
                void status.refetch();
                if (usable) void conversations.refetch();
              }}
            />
          }
        />
      )}
    </>
  );
}
