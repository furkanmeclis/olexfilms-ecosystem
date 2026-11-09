import { useMutation } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Linking, Modal, Pressable, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';
import { useModule } from '@/core/modules';
import { activeMembership, useCan } from '@/core/spaces';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { awaitingSignature, canCreateContract, type ContractState, contractState, contractWebUrl, showContract } from './contract';
import { type Service, WEB_ORIGIN } from './lib';
import { QrImage } from './qr-sheet';
import { errorText, Section } from './ui';

const TONE: Record<ContractState, StatusTone> = {
  none: 'problem',
  draft: 'neutral',
  pending: 'progress',
  executed: 'done',
  voided: 'problem',
};

/** Web wizard of a service in the active organization (contract signing). */
export function useContractUrl(serviceUuid: string): string | null {
  const slug = useSession((s) => activeMembership(s.me)?.slug);
  return slug ? contractWebUrl(WEB_ORIGIN, slug, serviceUuid) : null;
}

/**
 * The backend refused a status move for a missing signed contract
 * (CONTRACT_REQUIRED): say why and offer the signing page.
 */
export function useContractRequiredAlert(serviceUuid: string) {
  const { t } = useTranslation('dealer');
  const url = useContractUrl(serviceUuid);
  return () =>
    Alert.alert(t('contract.required_title'), t('contract.required_body'), [
      { text: t('common:close'), style: 'cancel' },
      ...(url ? [{ text: t('contract.sign_web'), onPress: () => void Linking.openURL(url) }] : []),
    ]);
}

/**
 * Intake contract of a service (TEC-1083): status (none / waiting for
 * signatures / signed), preparing the contract, and signing. The customer
 * OTP and both signatures are web only until /v1/contracts is on the mobile
 * mirror, so signing opens the web wizard or shows it as a QR for a tablet.
 */
export function ServiceContractCard({ s, onChanged }: { s: Service; onChanged: (s: Service) => void }) {
  const { t } = useTranslation('dealer');
  const on = useModule('contracts');
  const canWrite = useCan('contracts.write');
  const url = useContractUrl(s.uuid);
  const [qr, setQr] = useState(false);

  const create = useMutation({
    mutationFn: async () => {
      await unwrap(api.POST('/v1/services/{uuid}/contract', { params: { path: { uuid: s.uuid } }, body: {} }));
      return unwrap(api.GET('/v1/services/{uuid}', { params: { path: { uuid: s.uuid } } }));
    },
    onSuccess: onChanged,
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });

  if (!on || !showContract(s)) return null;
  const state = contractState(s.contract);
  const signing = awaitingSignature(s.contract);

  return (
    <Section title={t('contract.title')}>
      <View className="gap-3 rounded-2xl border border-border bg-card p-4" style={{ borderCurve: 'continuous' }}>
        <View className="flex-row items-center gap-3">
          <View className="size-9 items-center justify-center rounded-full bg-secondary">
            <Icon name="report" tone="primary" size={17} />
          </View>
          <View className="min-w-0 flex-1 gap-0.5">
            <Text variant="body" className="font-semibold">
              {s.contract ? t('contract.no', { no: s.contract.contract_no }) : t('contract.title')}
            </Text>
            {s.contract_required && state !== 'executed' ? (
              <Text variant="footnote" tone="muted">
                {t('contract.required')}
              </Text>
            ) : null}
          </View>
          <StatusPill label={t(`contract.status.${state}`)} tone={TONE[state]} />
        </View>

        {canWrite && canCreateContract(s) ? (
          <Button title={t('contract.create')} icon="add" loading={create.isPending} onPress={() => create.mutate()} />
        ) : null}

        {signing && url ? (
          <>
            <Text variant="footnote" tone="muted">
              {t('contract.sign_hint')}
            </Text>
            <View className="flex-row flex-wrap gap-2">
              <Button
                title={t('contract.sign_web')}
                icon="externalLink"
                onPress={() => void Linking.openURL(url)}
                className="grow basis-[45%]"
              />
              <Button
                title={t('contract.sign_qr')}
                variant="secondary"
                icon={<Icon name="qr" tone="primary" size={16} />}
                onPress={() => setQr(true)}
                className="grow basis-[45%]"
              />
            </View>
          </>
        ) : null}
      </View>

      {url ? (
        <Modal visible={qr} animationType="slide" presentationStyle="pageSheet" onRequestClose={() => setQr(false)}>
          <View className="flex-1 items-center gap-5 bg-background px-6 pt-4">
            <View className="w-full flex-row items-center justify-between">
              <Text variant="headline">{t('contract.title')}</Text>
              <Pressable accessibilityRole="button" accessibilityLabel={t('common:close')} hitSlop={10} onPress={() => setQr(false)}>
                <Icon name="close" />
              </Pressable>
            </View>
            <View className="overflow-hidden rounded-xl">
              <QrImage value={url} size={260} />
            </View>
            <Text variant="footnote" tone="muted" selectable className="text-center">
              {url}
            </Text>
            <Text variant="callout" tone="muted" className="text-center">
              {t('contract.qr_hint')}
            </Text>
          </View>
        </Modal>
      ) : null}
    </Section>
  );
}
