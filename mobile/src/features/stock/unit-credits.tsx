import { useMutation, useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import type { components } from '@/core/api/schema';
import { useSession } from '@/core/auth/session';
import { errorMessage } from '@/core/i18n';
import { type ServiceStatus, serviceTone } from '@/features/services/lib';
import { formatDate } from '@/features/warehouse/format';
import { Card, Field } from '@/features/warehouse/parts';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { SectionHeader } from '@/ui/list';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { creditsText, parseCredits } from './credits';

type Credits = components['schemas']['StockUnitCredits'];

/**
 * TEC-1221 (kredili kod): the credit card of a serial piece unit: remaining /
 * total, how many services used it and those services. The platform super
 * admin may add credits with a reason.
 */
export function UnitCreditsSection({ barcode, credits }: { barcode: string; credits: Credits }) {
  const { t } = useTranslation('warehouse');
  const superAdmin = useSession((s) => !!s.me?.user.is_super_admin);
  const [adding, setAdding] = useState(false);

  return (
    <>
      <SectionHeader title={t('stock.credits.title')} />
      <Card>
        <Field label={t('stock.credits.remaining')} value={creditsText(credits)} />
        <Field label={t('stock.credits.used')} value={String(credits.used_count)} />
        {credits.credits_remaining === 0 ? (
          <Text variant="footnote" tone="danger">
            {t('stock.credits.exhausted')}
          </Text>
        ) : null}
        {superAdmin ? (
          adding ? (
            <IncreaseForm barcode={barcode} unitUuid={credits.unit_uuid} onDone={() => setAdding(false)} />
          ) : (
            <Button title={t('stock.credits.increase')} icon="add" variant="ghost" onPress={() => setAdding(true)} />
          )
        ) : null}
      </Card>

      {credits.services.length ? (
        <>
          <SectionHeader title={t('stock.credits.services')} />
          <View className="gap-2">
            {credits.services.map((s) => (
              <View key={s.uuid} className="gap-0.5 rounded-lg border border-border bg-card px-4 py-3">
                <View className="flex-row items-center gap-2">
                  <Text variant="callout" className="flex-1 font-semibold" selectable>
                    {s.service_no}
                  </Text>
                  <StatusPill
                    tone={serviceTone(s.status as ServiceStatus)}
                    label={t(`dealer:service_status.${s.status}`)}
                  />
                </View>
                <Text variant="footnote" tone="muted" numberOfLines={2}>
                  {[
                    formatDate(s.completed_at ?? s.created_at),
                    s.finished_product ? t('stock.credits.finished') : null,
                    t('stock.credits.taken', { value: s.credits_taken }),
                  ]
                    .filter(Boolean)
                    .join(' · ')}
                </Text>
              </View>
            ))}
          </View>
        </>
      ) : null}
    </>
  );
}

/** Super admin: add credits to the unit (amount 1..1000 and a reason). */
function IncreaseForm({ barcode, unitUuid, onDone }: { barcode: string; unitUuid: string; onDone: () => void }) {
  const { t } = useTranslation('warehouse');
  const qc = useQueryClient();
  const [amount, setAmount] = useState('');
  const [reason, setReason] = useState('');
  const [touched, setTouched] = useState(false);
  const n = parseCredits(amount);
  const reasonOk = reason.trim().length > 0;

  const increase = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST('/v1/platform/stock/units/{uuid}/credits', {
          params: { path: { uuid: unitUuid } },
          body: { amount: n!, reason: reason.trim() },
        }),
      ),
    onSuccess: () => {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      void qc.invalidateQueries({ queryKey: ['stock', 'unit', barcode] });
      void qc.invalidateQueries({ queryKey: ['stock', 'units'] });
      onDone();
    },
    onError: (e) => Alert.alert(t('stock.credits.increase_failed'), errorMessage(isApiError(e) ? e.code : undefined)),
  });

  const submit = () => {
    setTouched(true);
    if (n === null || !reasonOk || increase.isPending) return;
    increase.mutate();
  };

  return (
    <View className="gap-3 pt-2">
      <Input
        label={t('stock.credits.amount')}
        placeholder={t('stock.credits.amount_placeholder')}
        keyboardType="number-pad"
        value={amount}
        onChangeText={setAmount}
        error={touched && n === null ? t('stock.credits.amount_invalid') : undefined}
      />
      <Input
        label={t('stock.credits.reason')}
        placeholder={t('stock.credits.reason_placeholder')}
        value={reason}
        onChangeText={setReason}
        error={touched && !reasonOk ? t('stock.credits.reason_required') : undefined}
      />
      <Button title={t('stock.credits.increase')} loading={increase.isPending} onPress={submit} />
      <Button title={t('common:cancel')} variant="ghost" disabled={increase.isPending} onPress={onDone} />
    </View>
  );
}
