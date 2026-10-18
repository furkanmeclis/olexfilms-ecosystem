import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { errorMessage } from '@/core/i18n';
import { Chips, ErrorState, InfoRow, ListSkeleton, useFormat } from '@/features/admin/kit';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { List, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { isOverdue, nextStatuses, priorityTone, statusTone, TASK_PRIORITIES, type TaskPriority, type TaskStatus } from './lib';

export function TaskDetailScreen() {
  const { t } = useTranslation('tasks');
  const f = useFormat();
  const qc = useQueryClient();
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const me = useSession((s) => s.me);
  const canWrite = !!me?.user.is_super_admin || !!me?.permissions.includes('tasks.write');
  const [comment, setComment] = useState('');

  const q = useQuery({
    queryKey: ['tasks', 'detail', uuid],
    queryFn: () => unwrap(api.GET('/v1/tasks/{uuid}', { params: { path: { uuid: uuid! } } })),
    enabled: !!uuid,
  });
  const comments = useQuery({
    queryKey: ['tasks', 'comments', uuid],
    queryFn: () => unwrap(api.GET('/v1/tasks/{uuid}/comments', { params: { path: { uuid: uuid! }, query: { limit: 100, offset: 0 } } })),
    enabled: !!uuid,
  });
  const assignees = useQuery({
    queryKey: ['tasks', 'assignees'],
    queryFn: () => unwrap(api.GET('/v1/tasks/assignees')),
    enabled: canWrite,
  });

  const update = useMutation({
    mutationFn: (body: { status?: TaskStatus; priority?: TaskPriority; assignee_user_uuid?: string | null }) =>
      unwrap(api.PATCH('/v1/tasks/{uuid}', { params: { path: { uuid: uuid! } }, body })),
    onSuccess: (data) => {
      qc.setQueryData(['tasks', 'detail', uuid], data);
      void qc.invalidateQueries({ queryKey: ['tasks', 'list'] });
    },
    onError: (e) => Alert.alert(errorMessage(isApiError(e) ? e.code : undefined)),
  });
  const addComment = useMutation({
    mutationFn: (body: string) => unwrap(api.POST('/v1/tasks/{uuid}/comments', { params: { path: { uuid: uuid! } }, body: { body } })),
    onSuccess: () => {
      setComment('');
      void qc.invalidateQueries({ queryKey: ['tasks', 'comments', uuid] });
      void qc.invalidateQueries({ queryKey: ['tasks', 'list'] });
    },
    onError: (e) => Alert.alert(errorMessage(isApiError(e) ? e.code : undefined)),
  });

  const task = q.data;
  const overdue = task ? isOverdue(task) : false;
  const assigneeOptions = [
    { key: '', label: t('unassigned') },
    ...(assignees.data?.items ?? []).map((a) => ({ key: a.uuid, label: a.name })),
  ];

  const refetchAll = () => {
    void q.refetch();
    void comments.refetch();
  };

  return (
    <>
      <Stack.Screen options={{ title: task?.title ?? '', headerLargeTitle: false }} />
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={refetchAll} />}>
        {q.isPending ? (
          <ListSkeleton rows={4} inset={false} />
        ) : !task ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : (
          <>
            <View className="gap-2">
              <Text variant="title" selectable>
                {task.title}
              </Text>
              <View className="flex-row flex-wrap gap-2">
                <StatusPill label={t(`status.${task.status}`)} tone={overdue ? 'problem' : statusTone(task.status)} />
                <StatusPill label={t(`priority.${task.priority}`)} tone={priorityTone(task.priority)} />
              </View>
            </View>

            {task.description ? (
              <View className="rounded-lg border border-border bg-card px-4 py-3">
                <Text variant="callout" selectable>
                  {task.description}
                </Text>
              </View>
            ) : null}

            {canWrite ? (
              <View className="gap-2">
                {nextStatuses(task.status).map((s) => (
                  <Button
                    key={s}
                    title={t(`action.${s}`)}
                    variant={s === 'cancelled' ? 'outline' : s === 'done' ? 'primary' : 'secondary'}
                    loading={update.isPending && update.variables?.status === s}
                    disabled={update.isPending}
                    onPress={() => update.mutate({ status: s })}
                  />
                ))}
              </View>
            ) : null}

            <List>
              <InfoRow label={t('subject')} value={task.subject_organization.name} />
              <InfoRow label={t('assignee')} value={task.assignee?.name ?? t('unassigned')} />
              <InfoRow label={t('due')} value={task.due_at ? `${overdue ? `${t('overdue')} · ` : ''}${f.date(task.due_at)}` : '–'} />
              <InfoRow label={t('created_by')} value={task.created_by?.name ?? '–'} />
              <InfoRow label={t('created')} value={f.dateTime(task.created_at)} />
              {task.closed_at ? <InfoRow label={t('closed')} value={f.dateTime(task.closed_at)} /> : null}
              <InfoRow label={t('source')} value={t(`source_${task.source}`)} last />
            </List>

            {canWrite ? (
              <>
                <SectionHeader title={t('priority_label')} />
                <Chips
                  className="-mx-4"
                  options={TASK_PRIORITIES.map((p) => ({ key: p, label: t(`priority.${p}`) }))}
                  value={task.priority}
                  onChange={(p) => {
                    if (p !== task.priority) update.mutate({ priority: p });
                  }}
                />
                <SectionHeader title={t('assign')} />
                <Chips
                  className="-mx-4"
                  options={assigneeOptions}
                  value={task.assignee?.uuid ?? ''}
                  onChange={(k) => {
                    if (k !== (task.assignee?.uuid ?? '')) update.mutate({ assignee_user_uuid: k || null });
                  }}
                />
              </>
            ) : null}

            <SectionHeader title={t('comments', { count: comments.data?.items.length ?? task.comment_count })} />
            {comments.data?.items.length ? (
              <List>
                {comments.data.items.map((c, i, arr) => (
                  <View key={c.uuid} className={i < arr.length - 1 ? 'gap-0.5 border-b border-border px-4 py-3' : 'gap-0.5 px-4 py-3'}>
                    <Text variant="callout" selectable>
                      {c.body}
                    </Text>
                    <Text variant="footnote" tone="muted">
                      {[c.author?.name, f.dateTime(c.created_at)].filter(Boolean).join(' · ')}
                    </Text>
                  </View>
                ))}
              </List>
            ) : (
              <Text variant="callout" tone="muted" className="px-4">
                {t('no_comments')}
              </Text>
            )}
            {canWrite ? (
              <View className="gap-2">
                <Input
                  label={t('comment_label')}
                  placeholder={t('comment_placeholder')}
                  value={comment}
                  onChangeText={setComment}
                  multiline
                  className="min-h-[88px] py-3"
                  textAlignVertical="top"
                />
                <Button
                  title={t('comment_send')}
                  loading={addComment.isPending}
                  disabled={!comment.trim()}
                  onPress={() => addComment.mutate(comment.trim())}
                />
              </View>
            ) : null}
          </>
        )}
      </Screen>
    </>
  );
}
