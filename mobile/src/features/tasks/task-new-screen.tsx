import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { Chips, useDebounced } from '@/features/admin/kit';
import { PickerField, PickerSheet } from '@/features/picker-sheet';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { dueDateToIso, QUICK_DUES, type QuickDue, quickDueDate, TASK_PRIORITIES, type TaskPriority, taskFormMissing } from './lib';

/** Search results shown in the subject sheet (the organizations endpoint is not paged). */
const ORG_LIMIT = 50;

/** Opens a manual task about a distributor or dealer of the brand. */
export function TaskNewScreen() {
  const { t } = useTranslation('tasks');
  const qc = useQueryClient();
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [priority, setPriority] = useState<TaskPriority>('normal');
  const [due, setDue] = useState('');
  const [assignee, setAssignee] = useState('');
  const [orgQ, setOrgQ] = useState('');
  const [org, setOrg] = useState<{ uuid: string; name: string } | null>(null);
  const [picking, setPicking] = useState(false);
  const search = useDebounced(orgQ.trim());

  // Server search (the endpoint is not paged), loaded only while the sheet is open.
  const orgs = useQuery({
    queryKey: ['tasks', 'orgs', search],
    queryFn: () => unwrap(api.GET('/v1/tenant/organizations', { params: { query: { q: search || undefined, limit: ORG_LIMIT, offset: 0 } } })),
    enabled: picking,
  });
  const assignees = useQuery({
    queryKey: ['tasks', 'assignees'],
    queryFn: () => unwrap(api.GET('/v1/tasks/assignees')),
  });

  const dueIso = dueDateToIso(due);
  const missing = taskFormMissing({ title, subject: !!org, due });
  // Errors show once the user tried to create (or left the date field), not while typing.
  const [tried, setTried] = useState(false);
  const [dueLeft, setDueLeft] = useState(false);
  const showError = (f: (typeof missing)[number]) => missing.includes(f) && (tried || (f === 'due' && dueLeft));
  const quick = QUICK_DUES.find((k) => quickDueDate(k) === due.trim()) ?? null;

  const submit = () => {
    setTried(true);
    if (missing.length) {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
      return;
    }
    create.mutate();
  };

  const create = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST('/v1/tasks', {
          body: {
            subject_organization_uuid: org!.uuid,
            title: title.trim(),
            description: description.trim() || undefined,
            priority,
            assignee_user_uuid: assignee || null,
            due_at: dueIso ?? null,
          },
        }),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks', 'list'] });
      router.back();
    },
    onError: (e) => Alert.alert(errorMessage(isApiError(e) ? e.code : undefined)),
  });

  return (
    <>
      <Stack.Screen options={{ title: t('new'), headerLargeTitle: false }} />
      <Screen>
        <Input
          label={t('field.title')}
          placeholder={t('field.title_placeholder')}
          value={title}
          onChangeText={setTitle}
          error={showError('title') ? t('field.title_required') : undefined}
        />
        <Input
          label={t('field.description')}
          placeholder={t('field.description_placeholder')}
          value={description}
          onChangeText={setDescription}
          multiline
          className="min-h-[96px] py-3"
          textAlignVertical="top"
        />

        <SectionHeader title={t('subject')} />
        <PickerField
          icon="building"
          value={org?.name}
          placeholder={t('field.subject_search')}
          error={showError('subject') ? t('field.subject_required') : undefined}
          onPress={() => setPicking(true)}
        />
        <PickerSheet
          visible={picking}
          title={t('field.subject_search')}
          options={(orgs.data?.items ?? []).map((o) => ({
            key: o.uuid,
            title: o.name,
            subtitle: [o.district, o.city].filter(Boolean).join(', ') || undefined,
            icon: 'building' as const,
          }))}
          selectedKey={org?.uuid}
          selectedLabel={org?.name}
          search={{ value: orgQ, onChange: setOrgQ }}
          searchPlaceholder={t('field.subject_placeholder')}
          autoFocusSearch
          loading={orgs.isPending}
          error={orgs.error}
          onRetry={() => void orgs.refetch()}
          onPick={(o) => setOrg({ uuid: o.key, name: o.title })}
          onClose={() => setPicking(false)}
        />

        <SectionHeader title={t('priority_label')} />
        <Chips
          className="-mx-4"
          options={TASK_PRIORITIES.map((p) => ({ key: p, label: t(`priority.${p}`) }))}
          value={priority}
          onChange={setPriority}
        />

        <SectionHeader title={t('assign')} />
        <Chips
          className="-mx-4"
          options={[{ key: '', label: t('unassigned') }, ...(assignees.data?.items ?? []).map((a) => ({ key: a.uuid, label: a.name }))]}
          value={assignee}
          onChange={setAssignee}
        />

        <SectionHeader title={t('field.due')} />
        <Chips<string>
          className="-mx-4"
          options={[
            { key: '', label: t('due_quick.none') },
            ...QUICK_DUES.map((k) => ({ key: k, label: t(`due_quick.${k}`) })),
          ]}
          value={quick ?? (due.trim() ? 'custom' : '')}
          onChange={(k) => {
            setDue(k ? quickDueDate(k as QuickDue) : '');
            setDueLeft(false);
          }}
        />
        <Input
          label={t('field.due_custom')}
          placeholder={t('field.due_placeholder')}
          value={due}
          onChangeText={setDue}
          onBlur={() => setDueLeft(true)}
          keyboardType="numbers-and-punctuation"
          autoCapitalize="none"
          error={showError('due') ? t('field.due_invalid') : undefined}
        />

        <View className="gap-2 pt-2">
          {tried && missing.length ? (
            <Text variant="footnote" tone="danger">
              {missing.map((m) => t(`missing.${m}`)).join(' ')}
            </Text>
          ) : null}
          {/* Always pressable: a tap says what is missing instead of doing nothing. */}
          <Button title={t('create')} size="lg" loading={create.isPending} onPress={submit} />
        </View>
      </Screen>
    </>
  );
}
