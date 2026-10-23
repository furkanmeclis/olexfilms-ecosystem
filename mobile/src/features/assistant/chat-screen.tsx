import { router, Stack, useLocalSearchParams } from 'expo-router';
import { type ComponentRef, useCallback, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, KeyboardAvoidingView, Platform, Pressable, ScrollView, TextInput, View } from 'react-native';

import { useCan } from '@/core/spaces';
import { DetailSkeleton, ErrorState, ListSkeleton } from '@/features/services/ui';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { ActionCard } from './action-card';
import { type AIActionCard, type ChatBlock, type ChatMessage, quotaUsedUp, recordPath } from './chat-state';
import { QuotaNote, StatusGate } from './assistant-screen';
import { ConsentCard, useAssistantLinks } from './consent-card';
import { chatErrorLabel, toolLabel } from './labels';
import { useAssistantChat, useAssistantStatus } from './use-assistant-chat';

function Block({
  block,
  busy,
  canConfirm,
  space,
  onConfirm,
  onCancel,
}: {
  block: ChatBlock;
  busy: boolean;
  canConfirm: boolean;
  space: string;
  onConfirm: (card: AIActionCard, edits?: Record<string, unknown>) => void;
  onCancel: (card: AIActionCard) => void;
}) {
  const { t } = useTranslation('assistant');
  switch (block.type) {
    case 'text':
      return (
        <Text variant="body" selectable>
          {block.text}
        </Text>
      );
    case 'tool':
      return (
        <View className="flex-row items-center gap-2">
          {block.status === 'running' ? <ActivityIndicator size="small" /> : null}
          <Text variant="footnote" tone={block.status === 'error' ? 'danger' : 'muted'}>
            {t('chat.tool_running', { tool: toolLabel(t, block.name) })}
          </Text>
        </View>
      );
    case 'confirm':
      return (
        <ActionCard
          card={block.card}
          busy={busy}
          canConfirm={canConfirm}
          onConfirm={(edits) => onConfirm(block.card, edits)}
          onCancel={() => onCancel(block.card)}
        />
      );
    case 'action': {
      const a = block.action;
      const path = a.status === 'confirmed' && a.link ? recordPath(space, a.link.kind, a.link.uuid) : null;
      return (
        <View className="gap-1">
          <Text variant="callout" tone={a.status === 'failed' ? 'danger' : 'default'}>
            {a.message || t(`action.${a.status}`)}
          </Text>
          {path ? (
            <Pressable accessibilityRole="link" onPress={() => router.push(path as never)}>
              <Text variant="callout" tone="primary">
                {t('action.open_record')}
              </Text>
            </Pressable>
          ) : null}
        </View>
      );
    }
    case 'error':
      return (
        <Text variant="callout" tone="danger">
          {chatErrorLabel(t, block.code, block.message)}
        </Text>
      );
  }
}

function MessageView(props: {
  message: ChatMessage;
  busy: boolean;
  canConfirm: boolean;
  space: string;
  onConfirm: (card: AIActionCard, edits?: Record<string, unknown>) => void;
  onCancel: (card: AIActionCard) => void;
}) {
  const { t } = useTranslation('assistant');
  const { message } = props;
  const mine = message.role === 'user';
  return (
    <View
      className={cn(
        'max-w-[88%] gap-2 rounded-xl px-3.5 py-2.5',
        mine ? 'self-end bg-primary' : 'self-start border border-border bg-card',
      )}>
      {mine ? (
        <Text variant="body" tone="onPrimary" selectable>
          {message.blocks.map((b) => (b.type === 'text' ? b.text : '')).join('')}
        </Text>
      ) : (
        <>
          {message.blocks.map((b, i) => (
            <Block key={i} block={b} {...props} />
          ))}
          {message.status === 'pending' && message.blocks.length === 0 ? (
            <View className="flex-row items-center gap-2">
              <ActivityIndicator size="small" />
              <Text variant="footnote" tone="muted">
                {t('chat.thinking')}
              </Text>
            </View>
          ) : null}
          {message.status === 'cancelled' ? (
            <Text variant="footnote" tone="muted">
              {t('chat.stopped')}
            </Text>
          ) : null}
        </>
      )}
    </View>
  );
}

/**
 * One assistant conversation (`new`: created on the first message). The
 * answer streams in as it is written; proposed changes run only after the
 * user confirms the card.
 */
export function ChatScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t } = useTranslation('assistant');
  const links = useAssistantLinks();
  const canConfirm = useCan('ai.actions.confirm');
  const status = useAssistantStatus();
  const [draft, setDraft] = useState('');
  const scroll = useRef<ComponentRef<typeof ScrollView>>(null);
  const onCreated = useCallback((id: string) => router.setParams({ uuid: id }), []);
  const chat = useAssistantChat(uuid && uuid !== 'new' ? uuid : null, onCreated);

  const s = status.data;
  const usable = !!s && s.enabled && s.allowed && !s.consent_required;
  const lastFailed = chat.messages[chat.messages.length - 1]?.status === 'error';
  const quotaOut = chat.quotaHit || quotaUsedUp(s?.quota);

  const submit = async () => {
    const text = draft;
    setDraft('');
    if (!(await chat.send(text))) setDraft(text);
  };

  return (
    <>
      <Stack.Screen options={{ title: chat.title || t('page.title'), headerLargeTitle: false }} />
      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        className="flex-1 bg-background">
        <ScrollView
          ref={scroll}
          keyboardShouldPersistTaps="handled"
          keyboardDismissMode="interactive"
          contentInsetAdjustmentBehavior="automatic"
          className="flex-1"
          contentContainerClassName="gap-3 px-4 pb-4 pt-2"
          onContentSizeChange={() => scroll.current?.scrollToEnd({ animated: true })}>
          {!s ? (
            status.isPending ? (
              <ListSkeleton rows={2} />
            ) : (
              <ErrorState error={status.error} onRetry={() => void status.refetch()} />
            )
          ) : !usable ? (
            <StatusGate status={s} />
          ) : chat.loading ? (
            <DetailSkeleton />
          ) : chat.loadError ? (
            <ErrorState error={chat.loadError} onRetry={() => void chat.reload()} />
          ) : (
            <>
              <QuotaNote status={s} hit={chat.quotaHit} />
              {chat.consent ? <ConsentCard text={chat.consent} onAccepted={chat.clearConsent} /> : null}
              {chat.messages.length === 0 ? (
                <Text variant="callout" tone="muted" className="py-6 text-center">
                  {t('chat.empty')}
                </Text>
              ) : null}
              {chat.messages.map((m) => (
                <MessageView
                  key={m.id}
                  message={m}
                  busy={chat.streaming}
                  canConfirm={canConfirm}
                  space={links.space}
                  onConfirm={(card, edits) => void chat.confirm(card, edits)}
                  onCancel={(card) => void chat.cancel(card)}
                />
              ))}
              {lastFailed && chat.canRetry && !chat.streaming ? (
                <Pressable accessibilityRole="button" onPress={chat.retry} className="self-start">
                  <Text variant="callout" tone="primary">
                    {t('chat.retry')}
                  </Text>
                </Pressable>
              ) : null}
            </>
          )}
        </ScrollView>
        {usable && !chat.loading && !chat.loadError ? (
          <View className="flex-row items-end gap-2 border-t border-border bg-background px-4 pb-6 pt-2">
            <TextInput
              value={draft}
              onChangeText={setDraft}
              placeholder={t('chat.placeholder')}
              accessibilityLabel={t('chat.placeholder')}
              placeholderTextColorClassName="accent-muted-foreground"
              multiline
              maxLength={8000}
              editable={!chat.consent}
              className="max-h-32 min-h-11 flex-1 rounded-lg bg-secondary px-3 py-2.5 text-[17px] text-foreground"
            />
            {chat.streaming ? (
              <Pressable accessibilityRole="button" accessibilityLabel={t('chat.stop')} hitSlop={8} onPress={chat.stop} className="pb-2">
                <Icon name="stop" tone="primary" size={30} />
              </Pressable>
            ) : (
              <Pressable
                accessibilityRole="button"
                accessibilityLabel={t('chat.send')}
                hitSlop={8}
                disabled={!draft.trim() || quotaOut}
                onPress={() => void submit()}
                className={cn('pb-2', (!draft.trim() || quotaOut) && 'opacity-40')}>
                <Icon name="send" tone="primary" size={30} />
              </Pressable>
            )}
          </View>
        ) : null}
      </KeyboardAvoidingView>
    </>
  );
}
