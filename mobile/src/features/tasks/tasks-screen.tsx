import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';
import {
  CardRow,
  Chips,
  PagedList,
  PAGE_SIZE,
  useChildHref,
  useDebounced,
  useFormat,
  usePagedQuery,
} from '@/features/admin/kit';
import { Button } from '@/ui/button';
import { StatusPill } from '@/ui/status-pill';

import { isOverdue, priorityTone, statusQuery, statusTone, type StatusFilter, type TaskPriority, type TaskStatus } from './lib';

type Scope = 'all' | 'mine';
const STATUS_FILTERS: StatusFilter[] = ['active', 'done', 'cancelled', 'all'];

export function TasksScreen() {
  const { t } = useTranslation('tasks');
  const f = useFormat();
  const href = useChildHref();
  const me = useSession((s) => s.me);
  const canWrite = !!me?.user.is_super_admin || !!me?.permissions.includes('tasks.write');
  const [scope, setScope] = useState<Scope>('all');
  const [status, setStatus] = useState<StatusFilter>('active');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  const list = usePagedQuery(['tasks', 'list', scope, status, query], (offset) =>
    unwrap(
      api.GET('/v1/tasks', {
        params: {
          query: {
            mine: scope === 'mine' ? true : undefined,
            status: statusQuery(status),
            q: query || undefined,
            limit: PAGE_SIZE,
            offset,
          },
        },
      }),
    ),
  );

  return (
    <>
      <Stack.Screen
        options={{
          title: t('title'),
          headerSearchBarOptions: {
            placeholder: t('search'),
            hideWhenScrolling: false,
            onChangeText: (e) => setQ(e.nativeEvent.text),
            onCancelButtonPress: () => setQ(''),
          },
        }}
      />
      <PagedList
        query={list}
        keyOf={(x) => x.uuid}
        header={
          <View className="gap-1 pb-1">
            {canWrite ? (
              <View className="px-4 pt-2">
                <Button title={t('new')} onPress={() => router.push(href('new'))} />
              </View>
            ) : null}
            <Chips
              options={(['all', 'mine'] as Scope[]).map((k) => ({ key: k, label: t(`scope.${k}`) }))}
              value={scope}
              onChange={setScope}
            />
            <Chips
              options={STATUS_FILTERS.map((k) => ({ key: k, label: t(`filter.${k}`) }))}
              value={status}
              onChange={setStatus}
            />
          </View>
        }
        empty={{ icon: 'task', title: query ? t('no_results') : t('empty') }}
        renderItem={({ item }) => {
          const overdue = isOverdue(item);
          return (
            <CardRow
              title={item.title}
              subtitle={item.subject_organization.name}
              meta={[
                t(`priority.${item.priority as TaskPriority}`),
                item.assignee?.name ?? t('unassigned'),
                item.due_at ? `${overdue ? `${t('overdue')} · ` : ''}${f.date(item.due_at)}` : null,
              ]
                .filter(Boolean)
                .join(' · ')}
              trailing={
                <View className="items-end gap-1">
                  <StatusPill label={t(`status.${item.status as TaskStatus}`)} tone={overdue ? 'problem' : statusTone(item.status)} />
                  {item.priority === 'urgent' || item.priority === 'high' ? (
                    <StatusPill label={t(`priority.${item.priority}`)} tone={priorityTone(item.priority)} />
                  ) : null}
                </View>
              }
              onPress={() => router.push(href(item.uuid))}
            />
          );
        }}
      />
    </>
  );
}
