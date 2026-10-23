import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { useCan } from '@/core/spaces';
import { Chips, PagedList, usePagedQuery, useFormat } from '@/features/admin/kit';
import { Sheet } from '@/features/services/sheet';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { Input } from '@/ui/input';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { decideWarning, listCertificates, listWarnings, rejectCertificate, verifyCertificate } from './api';
import {
  type Certificate,
  certificateTone,
  type CertificateWarning,
  hasReason,
  isPendingCertificate,
  isPendingWarning,
  localizedName,
  personName,
  warningTone,
} from './lib';

/** A filtered view with fewer rows than this keeps loading the next page. */
const MIN_FILTERED = 10;

type Tab = 'queue' | 'approvals';
type Filter = 'pending' | 'all';

const fail = (e: unknown) => Alert.alert(errorMessage(isApiError(e) ? e.code : undefined));

/** A reason or note typed in a sheet; the decision runs on Save. */
function ReasonSheet({
  title,
  label,
  hint,
  required,
  busy,
  onSave,
  onClose,
}: {
  title: string;
  label: string;
  hint?: string;
  required: boolean;
  busy: boolean;
  onSave: (text: string) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation('governance');
  const [text, setText] = useState('');
  return (
    <Sheet title={title} busy={busy} canSave={!required || hasReason(text)} onSave={() => onSave(text.trim())} onClose={onClose}>
      <Input
        label={label}
        placeholder={t('certificates.reason_placeholder')}
        hint={hint}
        value={text}
        onChangeText={setText}
        multiline
        className="min-h-24 py-3"
        autoFocus
      />
    </Sheet>
  );
}

function CertificateCard({
  c,
  canDecide,
  onVerify,
  onReject,
  busy,
}: {
  c: Certificate;
  canDecide: boolean;
  onVerify: () => void;
  onReject: () => void;
  busy: boolean;
}) {
  const { t } = useTranslation('governance');
  const f = useFormat();
  return (
    <View className="mx-4 gap-2 rounded-lg border border-border bg-card px-4 py-3">
      <View className="flex-row items-start justify-between gap-3">
        <View className="flex-1 gap-0.5">
          <Text variant="headline" numberOfLines={2}>
            {localizedName(c.type_name, f.locale) || t('certificates.unknown_type')}
          </Text>
          <Text variant="callout">{personName(c) || '–'}</Text>
          {c.organization_name ? (
            <Text variant="footnote" tone="muted">
              {c.organization_name}
            </Text>
          ) : null}
        </View>
        <StatusPill label={t(`certificates.status.${c.status}`, { defaultValue: c.status })} tone={certificateTone(c.status)} />
      </View>
      <Text variant="footnote" tone="muted">
        {t('certificates.dates', { issued: f.date(c.issued_at), expires: f.date(c.expires_at) })}
      </Text>
      {c.reject_reason ? <Text variant="footnote">{t('certificates.reject_reason', { reason: c.reject_reason })}</Text> : null}
      {canDecide && isPendingCertificate(c) ? (
        <View className="flex-row gap-2 pt-1">
          <Button className="flex-1" title={t('certificates.verify')} disabled={busy} onPress={onVerify} />
          <Button className="flex-1" variant="outline" title={t('certificates.reject')} disabled={busy} onPress={onReject} />
        </View>
      ) : null}
    </View>
  );
}

function WarningCard({
  w,
  canDecide,
  onApprove,
  onReject,
  busy,
}: {
  w: CertificateWarning;
  canDecide: boolean;
  onApprove: () => void;
  onReject: () => void;
  busy: boolean;
}) {
  const { t } = useTranslation('governance');
  const f = useFormat();
  return (
    <View className="mx-4 gap-2 rounded-lg border border-border bg-card px-4 py-3">
      <View className="flex-row items-start justify-between gap-3">
        <View className="flex-1 gap-0.5">
          <Text variant="headline" numberOfLines={1}>
            {w.service_no ? t('certificates.service_no', { no: w.service_no }) : t('certificates.warning')}
          </Text>
          <Text variant="callout">
            {personName(w) || '–'} · {localizedName(w.type_name, f.locale) || t('certificates.unknown_type')}
          </Text>
          {w.organization_name ? (
            <Text variant="footnote" tone="muted">
              {w.organization_name}
            </Text>
          ) : null}
        </View>
        <StatusPill label={t(`certificates.decision.${w.decision}`, { defaultValue: w.decision })} tone={warningTone(w.decision)} />
      </View>
      {w.reason ? <Text variant="footnote">{w.reason}</Text> : null}
      {w.note ? <Text variant="footnote" tone="muted">{t('certificates.note', { note: w.note })}</Text> : null}
      <Text variant="footnote" tone="muted">
        {f.dateTime(w.created_at)}
      </Text>
      {canDecide && isPendingWarning(w) ? (
        <View className="flex-row gap-2 pt-1">
          <Button className="flex-1" title={t('certificates.approve')} disabled={busy} onPress={onApprove} />
          <Button className="flex-1" variant="outline" title={t('certificates.reject')} disabled={busy} onPress={onReject} />
        </View>
      ) : null}
    </View>
  );
}

type Sheeted = { kind: 'cert-reject'; uuid: string } | { kind: 'warn'; uuid: string; approve: boolean };

/**
 * Staff certificates of the network: the verification queue (verify / reject a
 * pending upload) and the service warnings waiting for an approval. Each tab
 * shows with its permission, as on the web (certificates.verify,
 * certificates.approve_service).
 */
export function CertificatesScreen() {
  const { t } = useTranslation('governance');
  const qc = useQueryClient();
  const f = useFormat();
  const canRead = useCan('certificates.read');
  const canVerify = useCan('certificates.verify');
  const canApprove = useCan('certificates.approve_service');
  const [tab, setTab] = useState<Tab>('queue');
  const [filter, setFilter] = useState<Filter>('pending');
  const [sheet, setSheet] = useState<Sheeted | null>(null);

  const showQueue = canRead || canVerify;
  const active: Tab = tab === 'approvals' && canApprove ? 'approvals' : showQueue ? 'queue' : 'approvals';

  const certs = usePagedQuery<Certificate>(['certificates', 'list'], listCertificates, showQueue && active === 'queue');
  const warns = usePagedQuery<CertificateWarning>(['certificates', 'warnings'], listWarnings, canApprove && active === 'approvals');

  const queueItems = filter === 'pending' ? certs.items.filter(isPendingCertificate) : certs.items;
  const warnItems = filter === 'pending' ? warns.items.filter(isPendingWarning) : warns.items;

  // The API cannot filter by status: keep loading pages while the pending view is thin.
  const cur = active === 'queue' ? certs : warns;
  const curCount = active === 'queue' ? queueItems.length : warnItems.length;
  const loadMore = filter === 'pending' && cur.hasNextPage && !cur.isFetchingNextPage && !cur.isError && curCount < MIN_FILTERED;
  useEffect(() => {
    if (loadMore) void cur.fetchNextPage();
  }, [loadMore, cur]);

  const done = () => {
    setSheet(null);
    void qc.invalidateQueries({ queryKey: ['certificates'] });
  };
  const verify = useMutation({ mutationFn: verifyCertificate, onSuccess: done, onError: fail });
  const reject = useMutation({
    mutationFn: (v: { uuid: string; reason: string }) => rejectCertificate(v.uuid, v.reason),
    onSuccess: done,
    onError: fail,
  });
  const decide = useMutation({
    mutationFn: (v: { uuid: string; approve: boolean; note: string }) => decideWarning(v.uuid, v.approve, v.note),
    onSuccess: done,
    onError: fail,
  });
  const busy = verify.isPending || reject.isPending || decide.isPending;

  const askVerify = (c: Certificate) =>
    Alert.alert(t('certificates.confirm_verify'), `${localizedName(c.type_name, f.locale)} · ${personName(c)}`, [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t('certificates.verify'), onPress: () => verify.mutate(c.uuid) },
    ]);
  const askApprove = (w: CertificateWarning) =>
    Alert.alert(t('certificates.confirm_approve'), w.service_no ?? '', [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t('certificates.approve'), onPress: () => decide.mutate({ uuid: w.uuid, approve: true, note: '' }) },
    ]);

  const tabs = [
    ...(showQueue ? [{ key: 'queue' as const, label: t('certificates.tab_queue') }] : []),
    ...(canApprove ? [{ key: 'approvals' as const, label: t('certificates.tab_approvals') }] : []),
  ];
  const header = (
    <View>
      {tabs.length > 1 ? <Chips options={tabs} value={active} onChange={setTab} /> : null}
      <Chips
        options={[
          { key: 'pending', label: t('certificates.filter_pending') },
          { key: 'all', label: t('certificates.filter_all') },
        ]}
        value={filter}
        onChange={setFilter}
      />
    </View>
  );

  if (!showQueue && !canApprove) {
    return (
      <>
        <Stack.Screen options={{ title: t('certificates.title') }} />
        <EmptyState icon="blocked" title={t('no_access')} />
      </>
    );
  }

  return (
    <>
      <Stack.Screen options={{ title: t('certificates.title') }} />
      {active === 'queue' ? (
        <PagedList
          query={{ ...certs, items: queueItems }}
          keyOf={(c) => c.uuid}
          header={header}
          empty={{ icon: 'warranty', title: t('certificates.empty') }}
          renderItem={({ item }) => (
            <CertificateCard
              c={item}
              canDecide={canVerify}
              busy={busy}
              onVerify={() => askVerify(item)}
              onReject={() => setSheet({ kind: 'cert-reject', uuid: item.uuid })}
            />
          )}
        />
      ) : (
        <PagedList
          query={{ ...warns, items: warnItems }}
          keyOf={(w) => w.uuid}
          header={header}
          empty={{ icon: 'warranty', title: t('certificates.empty_warnings') }}
          renderItem={({ item }) => (
            <WarningCard
              w={item}
              canDecide={canApprove}
              busy={busy}
              onApprove={() => askApprove(item)}
              onReject={() => setSheet({ kind: 'warn', uuid: item.uuid, approve: false })}
            />
          )}
        />
      )}
      {sheet?.kind === 'cert-reject' ? (
        <ReasonSheet
          title={t('certificates.reject')}
          label={t('certificates.reason')}
          required
          busy={reject.isPending}
          onClose={() => setSheet(null)}
          onSave={(reason) => reject.mutate({ uuid: sheet.uuid, reason })}
        />
      ) : null}
      {sheet?.kind === 'warn' ? (
        <ReasonSheet
          title={t('certificates.reject')}
          label={t('certificates.note_label')}
          required
          busy={decide.isPending}
          onClose={() => setSheet(null)}
          onSave={(note) => decide.mutate({ uuid: sheet.uuid, approve: sheet.approve, note })}
        />
      ) : null}
    </>
  );
}
