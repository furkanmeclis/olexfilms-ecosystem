import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, FlatList, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { formatDateTime, PAGE_SIZE, usePagedList } from '@/features/services/lib';
import { Chips, errorText, ListBody, ListFooter } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { ActionPreviewRows, useSecondsLeft } from './action-card';
import { type AIActionCard, type ActionPreview, formatCountdown } from './chat-state';
import { actionLabel } from './labels';
import { aiKeys } from './use-assistant-chat';

type Source = 'all' | 'mcp' | 'whatsapp';

function Remaining({ expiresAt }: { expiresAt: string }) {
  const { t } = useTranslation('assistant');
  const seconds = useSecondsLeft(expiresAt);
  return seconds > 0 ? (
    <Text variant="footnote" tone="muted" className="tabular-nums">
      {t('card.expires_in', { time: formatCountdown(seconds) })}
    </Text>
  ) : (
    <StatusPill label={t('approvals.expired_short')} tone="problem" />
  );
}

/**
 * Pending AI actions (TEC-403): write tools an MCP client or WhatsApp
 * proposed for the signed-in user in this organization. Nothing is saved
 * until the user confirms here (ai.actions.confirm; the tool's own
 * permission, module and scope are checked again on confirmation).
 */
export function PendingActionsScreen() {
  const { t, i18n } = useTranslation('assistant');
  const qc = useQueryClient();
  const [source, setSource] = useState<Source>('all');
  const list = usePagedList([...aiKeys.pending, { source }], (offset) =>
    unwrap(
      api.GET('/v1/ai/pending-actions', {
        params: { query: { source: source === 'all' ? undefined : source, sort: '-created_at', limit: PAGE_SIZE, offset } },
      }),
    ),
  );

  const decide = useMutation({
    mutationFn: ({ card, confirm }: { card: AIActionCard; confirm: boolean }) =>
      unwrap(
        confirm
          ? api.POST('/v1/ai/pending-actions/{uuid}/confirm', { params: { path: { uuid: card.action_uuid } } })
          : api.POST('/v1/ai/pending-actions/{uuid}/cancel', { params: { path: { uuid: card.action_uuid } } }),
      ),
    onSuccess: (out, { confirm }) => {
      if (!confirm) Alert.alert(t('approvals.rejected'));
      else if (out.status === 'confirmed') Alert.alert(t('approvals.confirmed'));
      else Alert.alert(t('approvals.failed'), out.message);
    },
    onError: (e) =>
      Alert.alert(
        isApiError(e, 'AI_ACTION_EXPIRED')
          ? t('approvals.expired')
          : isApiError(e, 'AI_ACTION_RESOLVED')
            ? t('approvals.resolved')
            : t('approvals.failed'),
        isApiError(e, 'AI_ACTION_EXPIRED') || isApiError(e, 'AI_ACTION_RESOLVED') ? undefined : errorText(e),
      ),
    onSettled: () => void qc.invalidateQueries({ queryKey: aiKeys.pending }),
  });

  const ask = (card: AIActionCard, confirm: boolean) => {
    const preview = (card.preview ?? {}) as ActionPreview;
    const title = actionLabel(t, preview.action ?? card.tool_name, preview.summary);
    Alert.alert(title, confirm ? t('approvals.confirm_body') : t('approvals.reject_body'), [
      { text: t('history.cancel'), style: 'cancel' },
      {
        text: confirm ? t('approvals.confirm') : t('approvals.reject'),
        style: confirm ? 'default' : 'destructive',
        onPress: () => decide.mutate({ card, confirm }),
      },
    ]);
  };

  return (
    <>
      <Stack.Screen options={{ title: t('approvals.title') }} />
      <FlatList
        data={list.items}
        keyExtractor={(a) => a.action_uuid}
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2 px-4 pb-10"
        ListHeaderComponent={
          <View className="gap-3 pb-1 pt-2">
            <Text variant="callout" tone="muted">
              {t('approvals.description')}
            </Text>
            <Chips<Source>
              value={source}
              onChange={setSource}
              options={[
                { value: 'all', label: t('approvals.filter_all') },
                { value: 'mcp', label: t('source.mcp') },
                { value: 'whatsapp', label: t('source.whatsapp') },
              ]}
            />
          </View>
        }
        ListEmptyComponent={
          <ListBody
            isPending={list.isPending}
            error={list.error}
            count={list.items.length}
            onRetry={() => void list.refetch()}
            empty={{ icon: 'inbox', title: t('approvals.empty_title'), body: t('approvals.empty_description') }}
          />
        }
        renderItem={({ item: a }) => {
          const preview = (a.preview ?? {}) as ActionPreview;
          const busy = decide.isPending && decide.variables?.card.action_uuid === a.action_uuid;
          return (
            <View className="gap-2 rounded-lg border border-border bg-card px-4 py-3">
              <View className="flex-row items-center justify-between gap-3">
                <Text variant="headline" className="flex-1" numberOfLines={2}>
                  {actionLabel(t, preview.action ?? a.tool_name, preview.summary)}
                </Text>
                <Remaining expiresAt={a.expires_at} />
              </View>
              <Text variant="footnote" tone="muted">
                {[t(`source.${a.source}`), formatDateTime(a.created_at, i18n.language)].join(' · ')}
              </Text>
              <ActionPreviewRows preview={{ ...preview, summary: preview.action ? preview.summary : undefined }} />
              <View className="flex-row gap-2 pt-1">
                <Button
                  title={t('approvals.reject')}
                  variant="outline"
                  disabled={decide.isPending}
                  loading={busy && decide.variables?.confirm === false}
                  onPress={() => ask(a, false)}
                  className="flex-1"
                />
                <Button
                  title={t('approvals.confirm')}
                  disabled={decide.isPending}
                  loading={busy && decide.variables?.confirm === true}
                  onPress={() => ask(a, true)}
                  className="flex-1"
                />
              </View>
            </View>
          );
        }}
        onEndReachedThreshold={0.5}
        onEndReached={() => {
          if (list.hasNextPage && !list.isFetchingNextPage) void list.fetchNextPage();
        }}
        ListFooterComponent={<ListFooter loading={list.isFetchingNextPage} />}
        refreshControl={
          <RefreshControl refreshing={list.isRefetching && !list.isFetchingNextPage} onRefresh={() => void list.refetch()} />
        }
      />
    </>
  );
}
