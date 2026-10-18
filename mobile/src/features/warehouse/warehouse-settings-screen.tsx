import { useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RefreshControl } from 'react-native';

import { useCan } from '@/core/spaces';
import { ErrorState, ListSkeleton } from '@/features/admin/kit';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { eodKeys, saveWarehouseSettings, useWarehouseSettings } from './eod-api';
import { alertError } from './format';
import { Card, Field } from './parts';

const HHMM = /^([01]\d|2[0-3]):[0-5]\d$/;

/** Accepts "18", "1800", "18.00" and "18:00"; returns HH:MM or null. */
function normalizeTime(raw: string): string | null {
  const digits = raw.replace(/\D/g, '');
  if (!digits) return null;
  const padded = digits.length <= 2 ? `${digits.padStart(2, '0')}00` : digits.padStart(4, '0');
  const v = `${padded.slice(0, 2)}:${padded.slice(2, 4)}`;
  return HHMM.test(v) ? v : null;
}

/** Warehouse settings: the end-of-day cutoff of the business day. */
export function WarehouseSettingsScreen() {
  const { t } = useTranslation('warehouse');
  const qc = useQueryClient();
  const write = useCan('warehouse.write');
  const q = useWarehouseSettings();
  // null until the user types: the field shows the saved cutoff.
  const [draft, setDraft] = useState<string | null>(null);
  const value = draft ?? q.data?.eod_cutoff ?? '';
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const save = async (cutoff: string | null) => {
    setBusy(true);
    try {
      const out = await saveWarehouseSettings(cutoff);
      qc.setQueryData(eodKeys.settings(), out);
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      router.back();
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(false);
    }
  };

  const submit = () => {
    if (!value.trim()) return void save(null);
    const v = normalizeTime(value);
    if (!v) return setError(t('settings.cutoff_invalid'));
    setError(undefined);
    void save(v === '00:00' ? null : v);
  };

  return (
    <>
      <Stack.Screen options={{ title: t('settings.title'), headerLargeTitle: false }} />
      <Screen
        keyboardShouldPersistTaps="handled"
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {q.isPending ? (
          <ListSkeleton rows={2} />
        ) : !q.data ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : (
          <>
            <Card>
              <Field label={t('settings.timezone')} value={q.data.timezone} />
              <Field label={t('settings.current')} value={q.data.eod_cutoff ?? t('settings.calendar_day')} />
            </Card>
            {write ? (
              <>
                <Input
                  label={t('settings.cutoff')}
                  icon="clock"
                  value={value}
                  onChangeText={(s) => {
                    setDraft(s);
                    setError(undefined);
                  }}
                  placeholder={t('settings.cutoff_placeholder')}
                  keyboardType="numbers-and-punctuation"
                  maxLength={5}
                  error={error}
                  hint={t('settings.cutoff_hint')}
                  returnKeyType="done"
                  onSubmitEditing={submit}
                />
                <Button title={t('settings.save')} size="lg" loading={busy} onPress={submit} />
                {q.data.eod_cutoff ? (
                  <Button
                    title={t('settings.use_calendar_day')}
                    variant="ghost"
                    disabled={busy}
                    onPress={() => void save(null)}
                  />
                ) : null}
              </>
            ) : (
              <Text variant="callout" tone="muted">
                {t('settings.read_only')}
              </Text>
            )}
          </>
        )}
      </Screen>
    </>
  );
}
