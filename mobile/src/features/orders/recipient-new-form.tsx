import * as Haptics from 'expo-haptics';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, KeyboardAvoidingView, Platform, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { Chips } from '@/features/admin/kit';
import { DEFAULT_COUNTRY, parsePhone } from '@/features/auth/phone-country';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import {
  EMPTY_RECIPIENT,
  NAME_MAX,
  type RecipientErrors,
  type RecipientForm,
  type RecipientKind,
  toRecipientBody,
  validateRecipient,
} from './recipient-form';

/**
 * TEC-1063: the center opens a new order recipient (dealer candidate or retail /
 * delivery point) without leaving the order. The recipient lands in the waiting
 * pool (POST /v1/tenant/pending-recipients) and comes back selected.
 */
export function RecipientNewForm({
  initialName,
  onCreated,
  bottomInset,
}: {
  initialName?: string;
  onCreated: (r: { uuid: string; name: string }) => void;
  bottomInset: number;
}) {
  const { t } = useTranslation('orders');
  const [v, setV] = useState<RecipientForm>({ ...EMPTY_RECIPIENT, name: initialName ?? '' });
  const [errors, setErrors] = useState<RecipientErrors>({});
  const [busy, setBusy] = useState(false);
  const inFlight = useRef(false);
  const set = (k: keyof RecipientForm) => (s: string) => setV((p) => ({ ...p, [k]: s }));
  const err = (k: keyof RecipientErrors) => (errors[k] ? t(`recipient.errors.${errors[k]}`) : undefined);

  const save = async () => {
    if (inFlight.current) return;
    const raw = v.phone.trim();
    const phone = raw ? parsePhone(raw, DEFAULT_COUNTRY) : null;
    const found = validateRecipient(v, phone);
    setErrors(found);
    if (Object.keys(found).length || !phone) return;
    inFlight.current = true;
    setBusy(true);
    try {
      const r = await unwrap(api.POST('/v1/tenant/pending-recipients', { body: toRecipientBody(v, phone) }));
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      onCreated({ uuid: r.uuid, name: r.name });
    } catch (e) {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
      Alert.alert(t('recipient.save_failed'), errorMessage(isApiError(e) ? e.code : undefined));
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  };

  return (
    <KeyboardAvoidingView className="flex-1" behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
      <ScrollView
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        contentContainerClassName="gap-4 px-4 pt-1"
        contentContainerStyle={{ paddingBottom: bottomInset + 24 }}>
        <Text variant="footnote" tone="muted">
          {t('recipient.hint')}
        </Text>
        <Chips<RecipientKind>
          className="-mx-4"
          options={[
            { key: 'dealer_candidate', label: t('recipient.kind.dealer_candidate') },
            { key: 'retail_point', label: t('recipient.kind.retail_point') },
          ]}
          value={v.kind}
          onChange={(kind) => setV((p) => ({ ...p, kind }))}
        />
        <Input
          label={t('recipient.name')} placeholder={t('recipient.name_placeholder')}
          icon="building"
          value={v.name}
          onChangeText={set('name')}
          maxLength={NAME_MAX}
          autoFocus={!initialName}
          error={err('name')}
        />
        <Input
          label={t('recipient.phone')}
          icon="phone"
          value={v.phone}
          onChangeText={set('phone')}
          placeholder={t('recipient.phone_placeholder')}
          keyboardType="phone-pad"
          autoComplete="tel"
          error={err('phone')}
        />
        <View className="flex-row gap-3">
          <View className="flex-1">
            <Input label={t('recipient.city')} placeholder={t('recipient.city_placeholder')} icon="location" value={v.city} onChangeText={set('city')} />
          </View>
          <View className="flex-1">
            <Input label={t('recipient.district')} placeholder={t('recipient.district_placeholder')} value={v.district} onChangeText={set('district')} />
          </View>
        </View>
        <Input label={t('recipient.address')} placeholder={t('recipient.address_placeholder')} value={v.address} onChangeText={set('address')} multiline />
        <Input
          label={t('recipient.tax_no')} placeholder={t('recipient.tax_no_placeholder')}
          value={v.tax_no}
          onChangeText={set('tax_no')}
          keyboardType="number-pad"
          maxLength={14}
          error={err('tax_no')}
        />
        <Input label={t('recipient.tax_office')} placeholder={t('recipient.tax_office_placeholder')} value={v.tax_office} onChangeText={set('tax_office')} />
        <Button title={t('recipient.save')} size="lg" loading={busy} disabled={busy} onPress={() => void save()} />
      </ScrollView>
    </KeyboardAvoidingView>
  );
}
