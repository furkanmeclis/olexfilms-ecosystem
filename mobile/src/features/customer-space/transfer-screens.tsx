import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { type Href, router, Stack, useLocalSearchParams, useSegments } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import type { components } from '@/core/api/schema';
import { formatDateTime } from '@/features/services/lib';
import { DetailSkeleton, ErrorState, Section } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { retryPortal, useFleetRoles } from './lib';
import {
  canVerify,
  cleanCode,
  isTransferPhone,
  normalizePhone,
  pendingTransfer,
  transferErrorKey,
  verifyBody,
} from './transfer-lib';

type Transfer = components['schemas']['VehicleTransfer'];

const transfersKey = (uuid: string) => ['portal', 'vehicles', 'transfers', uuid] as const;

/** Transfer screen link that stays inside the current tab's stack (home or warranties). */
export function useTransferLink() {
  const seg = useSegments() as string[];
  const tab = seg[2];
  return (uuid: string) => {
    const uid = encodeURIComponent(uuid);
    return (tab === 'warranties' ? `/musteri/warranties/vehicle/transfer/${uid}` : `/musteri/vehicle/transfer/${uid}`) as Href;
  };
}

function transferTone(status: Transfer['status']): StatusTone {
  if (status === 'completed') return 'done';
  if (status === 'pending') return 'progress';
  return 'neutral';
}

function useTransferError() {
  const { t } = useTranslation('customer');
  return (e: unknown) => {
    const err = isApiError(e) ? e : { status: 0, code: null as string | null, details: undefined };
    const m = transferErrorKey(err);
    return t(m.key, m.params);
  };
}

function PhoneStep({ vehicleUuid, onError }: { vehicleUuid: string; onError: (e: unknown) => void }) {
  const { t } = useTranslation('customer');
  const qc = useQueryClient();
  const [phone, setPhone] = useState('');
  const start = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST('/v1/portal/vehicles/{uuid}/transfers', {
          params: { path: { uuid: vehicleUuid } },
          body: { phone: normalizePhone(phone) },
        }),
      ),
    onSuccess: () => void qc.invalidateQueries({ queryKey: transfersKey(vehicleUuid) }),
    onError,
  });
  return (
    <View className="gap-3">
      <Text variant="callout" tone="muted">
        {t('transfer_intro')}
      </Text>
      <Input
        label={t('transfer_phone')}
        icon="phone"
        placeholder="+905551234567"
        hint={t('transfer_phone_hint')}
        keyboardType="phone-pad"
        autoComplete="off"
        value={phone}
        onChangeText={setPhone}
      />
      <Button
        title={t('transfer_start')}
        size="lg"
        loading={start.isPending}
        disabled={!isTransferPhone(phone)}
        onPress={() => start.mutate()}
      />
    </View>
  );
}

function CodeStep({
  transfer,
  vehicleUuid,
  onError,
  onDone,
}: {
  transfer: Transfer;
  vehicleUuid: string;
  onError: (e: unknown) => void;
  onDone: (t: Transfer) => void;
}) {
  const { t, i18n } = useTranslation('customer');
  const qc = useQueryClient();
  const [fromCode, setFromCode] = useState('');
  const [toCode, setToCode] = useState('');
  const [name, setName] = useState('');
  const [surname, setSurname] = useState('');
  const refresh = () => void qc.invalidateQueries({ queryKey: transfersKey(vehicleUuid) });

  const verify = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST('/v1/portal/vehicle-transfers/{uuid}/verify', {
          params: { path: { uuid: transfer.uuid } },
          body: verifyBody(transfer, { fromCode, toCode, name, surname }),
        }),
      ),
    onSuccess: (res) => {
      setFromCode('');
      setToCode('');
      if (res.status === 'completed') onDone(res);
      else refresh();
    },
    // A wrong code changes the attempt counter (or cancels the transfer).
    onError: (e) => {
      onError(e);
      refresh();
    },
  });
  const cancel = useMutation({
    mutationFn: () =>
      unwrap(api.POST('/v1/portal/vehicle-transfers/{uuid}/cancel', { params: { path: { uuid: transfer.uuid } } })),
    onSuccess: refresh,
    onError: (e) => {
      onError(e);
      refresh();
    },
  });

  const confirmCancel = () =>
    Alert.alert(t('transfer_cancel_title'), t('transfer_cancel_body'), [
      { text: t('keep'), style: 'cancel' },
      { text: t('transfer_cancel'), style: 'destructive', onPress: () => cancel.mutate() },
    ]);

  return (
    <View className="gap-4">
      <View className="gap-2 rounded-lg border border-border bg-card p-4">
        <View className="flex-row items-center justify-between gap-3">
          <Text variant="headline">{t('transfer_pending')}</Text>
          <StatusPill label={t('transfer_status.pending')} tone="progress" />
        </View>
        <Text variant="callout" tone="muted">
          {t('transfer_pending_info', { phone: transfer.to_phone_masked })}
        </Text>
        <Text variant="footnote" tone="muted">
          {t('transfer_expires', { time: formatDateTime(transfer.expires_at, i18n.language) })}
        </Text>
        <Text variant="footnote" tone="muted">
          {t('transfer_attempts', { used: transfer.attempts, max: transfer.max_attempts })}
        </Text>
      </View>

      <View className="gap-3">
        {transfer.from_verified ? (
          <VerifiedLine label={t('transfer_from_code')} />
        ) : (
          <Input
            label={t('transfer_from_code')}
            icon="code"
            placeholder="000000"
            keyboardType="number-pad"
            autoComplete="one-time-code"
            maxLength={6}
            value={fromCode}
            onChangeText={(v) => setFromCode(cleanCode(v))}
          />
        )}
        {transfer.to_verified ? (
          <VerifiedLine label={t('transfer_to_code')} />
        ) : (
          <Input
            label={t('transfer_to_code')}
            icon="code"
            placeholder="000000"
            hint={t('transfer_to_code_hint')}
            keyboardType="number-pad"
            maxLength={6}
            value={toCode}
            onChangeText={(v) => setToCode(cleanCode(v))}
          />
        )}
        {!transfer.new_owner_known ? (
          <View className="gap-3">
            <Text variant="footnote" tone="muted">
              {t('transfer_new_owner_hint')}
            </Text>
            <Input
              label={t('transfer_new_owner_name')}
              icon="customer"
              placeholder={t('transfer_new_owner_name')}
              maxLength={100}
              value={name}
              onChangeText={setName}
            />
            <Input
              label={t('transfer_new_owner_surname')}
              placeholder={t('transfer_new_owner_surname')}
              maxLength={100}
              value={surname}
              onChangeText={setSurname}
            />
          </View>
        ) : null}
      </View>

      <Button
        title={t('transfer_verify')}
        size="lg"
        loading={verify.isPending}
        disabled={!canVerify(transfer, { fromCode, toCode, name })}
        onPress={() => verify.mutate()}
      />
      <Button title={t('transfer_cancel')} variant="outline" loading={cancel.isPending} onPress={confirmCancel} />
    </View>
  );
}

function VerifiedLine({ label }: { label: string }) {
  const { t } = useTranslation('customer');
  return (
    <View className="min-h-12 flex-row items-center gap-3 rounded-lg border border-border bg-card px-4">
      <Icon name="check" tone="primary" size={18} />
      <Text variant="callout" className="flex-1">
        {label}
      </Text>
      <Text variant="footnote" tone="muted">
        {t('transfer_verified')}
      </Text>
    </View>
  );
}

/**
 * Start, verify (two codes) or cancel the ownership transfer of the signed-in
 * customer's vehicle (`/v1/portal/vehicles/{uuid}/transfers`). An open transfer
 * resumes at the code step. A fleet account is read only and cannot transfer.
 */
export function CustomerVehicleTransferScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('customer');
  const qc = useQueryClient();
  const { readOnly } = useFleetRoles();
  const toMessage = useTransferError();
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState<Transfer | null>(null);

  const q = useQuery({
    queryKey: transfersKey(uuid),
    retry: retryPortal,
    enabled: !readOnly,
    queryFn: () => unwrap(api.GET('/v1/portal/vehicles/{uuid}/transfers', { params: { path: { uuid } } })),
  });
  const items = q.data?.items ?? [];
  const pending = pendingTransfer(items);
  const history = items.filter((x) => x.status !== 'pending');
  const fail = (e: unknown) => setError(toMessage(e));

  const finish = () => {
    void qc.invalidateQueries({ queryKey: ['portal', 'vehicles'] });
    void qc.invalidateQueries({ queryKey: ['portal', 'warranties'] });
    router.replace('/musteri/vehicles' as Href);
  };

  let body: React.ReactNode;
  if (readOnly) {
    body = (
      <Text variant="callout" tone="muted">
        {t('transfer_read_only')}
      </Text>
    );
  } else if (done) {
    body = (
      <View className="items-center gap-3 py-6">
        <Icon name="check" tone="primary" size={36} />
        <Text variant="title" className="text-center">
          {t('transfer_done_title')}
        </Text>
        <Text variant="callout" tone="muted" className="text-center">
          {t('transfer_done', { count: done.warranties_moved })}
        </Text>
        <Button title={t('transfer_back_to_vehicles')} onPress={finish} />
      </View>
    );
  } else if (q.isPending) {
    body = <DetailSkeleton />;
  } else if (q.error) {
    body = <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  } else if (pending) {
    body = <CodeStep key={pending.uuid} transfer={pending} vehicleUuid={uuid} onError={fail} onDone={setDone} />;
  } else {
    body = <PhoneStep vehicleUuid={uuid} onError={fail} />;
  }

  return (
    <>
      <Stack.Screen options={{ title: t('transfer_title'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        keyboardShouldPersistTaps="handled"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {error ? (
          <View className="rounded-lg border border-destructive bg-card p-3">
            <Text variant="callout" tone="danger" accessibilityRole="alert">
              {error}
            </Text>
          </View>
        ) : null}
        {body}
        {!done && !readOnly && history.length > 0 ? (
          <Section title={t('transfer_history')}>
            {history.map((h, i) => (
              <View
                key={h.uuid}
                className={`min-h-12 flex-row items-center gap-3 px-4 py-3 ${i < history.length - 1 ? 'border-b border-border' : ''}`}>
                <View className="flex-1 gap-0.5">
                  <Text variant="callout" numberOfLines={1}>
                    {h.to_phone_masked}
                  </Text>
                  <Text variant="footnote" tone="muted">
                    {formatDateTime(h.completed_at ?? h.cancelled_at ?? h.created_at, i18n.language)}
                  </Text>
                </View>
                <StatusPill label={t(`transfer_status.${h.status}`)} tone={transferTone(h.status)} />
              </View>
            ))}
          </Section>
        ) : null}
      </ScrollView>
    </>
  );
}
