import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { DEFAULT_COUNTRY, parsePhone } from '@/features/auth/phone-country';
import { PickerSheet } from '@/features/picker-sheet';
import { fullName, PAGE_SIZE, useDebounced, usePagedList } from '@/features/services/lib';
import { Chips, errorText, Section } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import { fetchAssignees, useLeadLinks, useOrgType } from './hooks';
import {
  followUpIso,
  LEAD_SOURCES,
  LEAD_TEMPERATURES,
  type LeadSource,
  type LeadTargetType,
  type LeadTemperature,
  leadFormErrors,
  leadTargetTypesFor,
} from './lib';

type PickedCustomer = { uuid: string; label: string };
type FollowUpChoice = 'none' | '1' | '3' | '7';

function CustomerPicker({
  selected,
  onPick,
  onClose,
}: {
  selected: PickedCustomer | null;
  onPick: (c: PickedCustomer) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation('leads');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const res = usePagedList(['customers', 'lead-picker', 'paged', query], (offset) =>
    unwrap(api.GET('/v1/customers', { params: { query: { q: query || undefined, limit: PAGE_SIZE, offset } } })),
  );
  const names = new Map(res.items.map((c) => [c.uuid, fullName(c)]));
  return (
    <PickerSheet
      visible
      title={t('customer')}
      options={res.items.map((c) => ({
        key: c.uuid,
        title: c.type === 'corporate' && c.company_name ? c.company_name : fullName(c),
        subtitle: [c.phone, c.email].filter(Boolean).join(' · ') || undefined,
        disabled: c.anonymized,
      }))}
      selectedKey={selected?.uuid}
      selectedLabel={selected?.label}
      search={{ value: q, onChange: setQ }}
      searchPlaceholder={t('customer_search')}
      autoFocusSearch
      loading={res.isPending}
      error={res.error}
      onRetry={() => void res.refetch()}
      fetchingMore={res.isFetchingNextPage}
      onEndReached={() => {
        if (res.hasNextPage && !res.isFetchingNextPage) void res.fetchNextPage();
      }}
      emptyText={t('customers_empty')}
      // The lead keeps the person's name, as before (not the company name).
      onPick={(o) => onPick({ uuid: o.key, label: names.get(o.key) ?? o.title })}
      onClose={onClose}
    />
  );
}

/** Create a lead: a customer of the scope, or a dealer / distributor candidate. */
export function NewLeadScreen() {
  const { t } = useTranslation('leads');
  const links = useLeadLinks();
  const qc = useQueryClient();
  const orgType = useOrgType();
  const targets = leadTargetTypesFor(orgType);
  const [target, setTarget] = useState<LeadTargetType>('customer');
  const [customer, setCustomer] = useState<PickedCustomer | null>(null);
  const [pickCustomer, setPickCustomer] = useState(false);
  const [company, setCompany] = useState('');
  const [contact, setContact] = useState('');
  const [phone, setPhone] = useState('');
  const [email, setEmail] = useState('');
  const [source, setSource] = useState<LeadSource>('incoming_call');
  const [temperature, setTemperature] = useState<LeadTemperature>('warm');
  const [followUp, setFollowUp] = useState<FollowUpChoice>('none');
  const [notes, setNotes] = useState('');
  const [assignee, setAssignee] = useState('');
  const [errors, setErrors] = useState<string[]>([]);

  const members = useQuery({ queryKey: ['leads', 'assignees', 'self'], queryFn: () => fetchAssignees() });

  const phoneValue = () => {
    const raw = phone.trim();
    return raw ? (parsePhone(raw, DEFAULT_COUNTRY) ?? raw) : '';
  };

  const create = useMutation({
    mutationFn: () => {
      const isCustomer = target === 'customer';
      return unwrap(
        api.POST('/v1/leads', {
          body: {
            target_type: target,
            customer_uuid: isCustomer ? customer?.uuid : undefined,
            candidate_company_name: isCustomer ? undefined : company.trim(),
            candidate_contact_name: isCustomer ? undefined : contact.trim(),
            candidate_phone_e164: phoneValue() || undefined,
            candidate_email: email.trim() || undefined,
            source,
            temperature,
            follow_up_date: followUpIso(followUp === 'none' ? null : Number(followUp)),
            assignee_user_uuid: assignee || undefined,
            notes: notes.trim(),
          },
        }),
      );
    },
    onSuccess: (lead) => {
      void qc.invalidateQueries({ queryKey: ['leads', 'list'] });
      router.replace(links.detail(lead.uuid));
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });

  const submit = () => {
    const found = leadFormErrors({
      target_type: target,
      customer_uuid: customer?.uuid ?? '',
      company,
      contact,
      phone: phoneValue(),
      email,
    });
    setErrors(found);
    if (found.length === 0) create.mutate();
  };
  const err = (k: string) => (errors.includes(k) ? t(`error.${k}`) : undefined);

  return (
    <>
      <Stack.Screen options={{ title: t('new') }} />
      <ScrollView
        keyboardShouldPersistTaps="handled"
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2">
        <Chips<LeadTargetType>
          value={target}
          onChange={setTarget}
          options={targets.map((x) => ({ value: x, label: t(`target.${x}`) }))}
        />

        {target === 'customer' ? (
          <Section title={t('customer')}>
            <Pressable
              accessibilityRole="button"
              onPress={() => setPickCustomer(true)}
              className="min-h-12 flex-row items-center gap-3 px-4 py-2.5 active:bg-muted">
              <Text variant="body" tone={customer ? 'default' : 'muted'} className="flex-1" numberOfLines={1}>
                {customer?.label ?? t('customer_pick')}
              </Text>
              <Icon name="forward" tone="muted-foreground" size={16} />
            </Pressable>
            {err('customer') ? (
              <Text variant="footnote" tone="danger" className="px-4 pb-3">
                {err('customer')}
              </Text>
            ) : null}
          </Section>
        ) : (
          <>
            <Input label={t('company')} placeholder={t('company_placeholder')} value={company} onChangeText={setCompany} error={err('company')} />
            <Input label={t('contact_name')} placeholder={t('contact_placeholder')} value={contact} onChangeText={setContact} error={err('contact')} />
          </>
        )}

        <Input
          label={t('phone')}
          placeholder={t('phone_placeholder')}
          value={phone}
          onChangeText={setPhone}
          keyboardType="phone-pad"
          error={err('phone')}
        />
        <Input
          label={t('email')}
          placeholder={t('email_placeholder')}
          value={email}
          onChangeText={setEmail}
          keyboardType="email-address"
          autoCapitalize="none"
          autoCorrect={false}
          error={err('email')}
        />

        <View className="gap-1.5">
          <Text variant="caption" tone="muted" className="px-1">
            {t('source_label')}
          </Text>
          <Chips<LeadSource> value={source} onChange={setSource} options={LEAD_SOURCES.map((s) => ({ value: s, label: t(`source.${s}`) }))} />
        </View>
        <View className="gap-1.5">
          <Text variant="caption" tone="muted" className="px-1">
            {t('temperature_label')}
          </Text>
          <Chips<LeadTemperature>
            value={temperature}
            onChange={setTemperature}
            options={LEAD_TEMPERATURES.map((x) => ({ value: x, label: t(`temperature.${x}`) }))}
          />
        </View>
        <View className="gap-1.5">
          <Text variant="caption" tone="muted" className="px-1">
            {t('follow_up_title')}
          </Text>
          <Chips<FollowUpChoice>
            value={followUp}
            onChange={setFollowUp}
            options={[
              { value: 'none', label: t('follow_up_choice.none') },
              { value: '1', label: t('follow_up_choice.1') },
              { value: '3', label: t('follow_up_choice.3') },
              { value: '7', label: t('follow_up_choice.7') },
            ]}
          />
        </View>
        {members.data && members.data.items.length > 0 ? (
          <View className="gap-1.5">
            <Text variant="caption" tone="muted" className="px-1">
              {t('assignee')}
            </Text>
            <Chips<string>
              value={assignee}
              onChange={setAssignee}
              options={[{ value: '', label: t('unassigned') }, ...members.data.items.map((m) => ({ value: m.uuid, label: fullName(m) }))]}
            />
          </View>
        ) : null}

        <Input
          label={t('notes')}
          placeholder={t('notes_placeholder')}
          value={notes}
          onChangeText={setNotes}
          multiline
          maxLength={2000}
          className="min-h-24 py-3"
        />
        <Button title={t('create')} size="lg" loading={create.isPending} onPress={submit} />
      </ScrollView>
      {pickCustomer ? (
        <CustomerPicker
          selected={customer}
          onClose={() => setPickCustomer(false)}
          onPick={(c) => {
            setCustomer(c);
            setPickCustomer(false);
          }}
        />
      ) : null}
    </>
  );
}
