import { useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, KeyboardAvoidingView, Modal, Platform, Pressable, RefreshControl, ScrollView, TextInput, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage, i18n } from '@/core/i18n';
import { useCan } from '@/core/spaces';
import {
  CardRow,
  Chips,
  ErrorState,
  InfoRow,
  ListSkeleton,
  PagedList,
  useChildHref,
  useDebounced,
  useFormat,
} from '@/features/admin/kit';
import { ContinuousScanner, type ScanOutcome } from '@/features/warehouse/continuous-scanner';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import {
  createTransfer,
  moveTransfer,
  NEEDS_REASON,
  type Transfer,
  type TransferFilter,
  type TransferKind,
  transferKeys,
  type TransferStatus,
  transferTone,
  useTransfer,
  useTransfers,
  useTransferTargets,
} from './api';

const BASE = '/bayi/menu/transfers';
/** Flow order of the action buttons (the server decides which are allowed). */
const FLOW: TransferStatus[] = ['approved', 'shipped', 'received', 'rejected', 'cancelled'];

function failMessage(e: unknown) {
  if (!isApiError(e)) return i18n.t('common:error_generic');
  const own = `transfers:errors.${e.code}`;
  return i18n.exists(own) ? i18n.t(own) : errorMessage(e.code);
}

function alertError(e: unknown) {
  void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
  Alert.alert(failMessage(e));
}

function counterparty(tr: Transfer) {
  return tr.role === 'sender' ? tr.receiver : tr.sender;
}

/** Fixed-barcode quantity: empty moves the whole unit, else a whole number 1..9,999,999 (web validQuantity). */
export function validQuantity(raw: string): boolean {
  const v = raw.trim();
  return v === '' || /^[1-9][0-9]{0,6}$/.test(v);
}

/** Sheet for a rejection or cancellation with an optional reason (iOS and Android alike). */
function ReasonSheet({
  status,
  onClose,
  onConfirm,
}: {
  status: TransferStatus;
  onClose: () => void;
  onConfirm: (reason?: string) => void;
}) {
  const { t } = useTranslation('transfers');
  const [reason, setReason] = useState('');
  return (
    <Modal visible animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="flex-1 bg-background">
        <View className="flex-row items-center justify-between px-4 pb-2 pt-5">
          <Text variant="title" className="flex-1" numberOfLines={2}>
            {t(`confirm.${status}`)}
          </Text>
          <Button title={t('common:cancel')} variant="ghost" onPress={onClose} />
        </View>
        <ScrollView contentContainerClassName="gap-4 px-4 pb-10" keyboardShouldPersistTaps="handled">
          <Input
            label={t('reason')}
            icon="edit"
            value={reason}
            onChangeText={setReason}
            placeholder={t('reason_placeholder')}
            hint={t('reason_hint')}
            multiline
            maxLength={500}
            autoFocus
          />
          <Button
            title={t(`action.${status}`)}
            variant="destructive"
            size="lg"
            onPress={() => {
              onClose();
              onConfirm(reason.trim() || undefined);
            }}
          />
        </ScrollView>
      </KeyboardAvoidingView>
    </Modal>
  );
}

/** Dealer transfers (TEC-197/223): requests to sibling dealers and returns to the parent. */
export function TransfersScreen() {
  const { t } = useTranslation('transfers');
  const f = useFormat();
  const href = useChildHref();
  const request = useCan('transfers.request');
  const [filter, setFilter] = useState<TransferFilter>('open');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const list = useTransfers(filter, query);

  const askNew = () =>
    Alert.alert(t('new_title'), undefined, [
      { text: t('kind.sibling'), onPress: () => router.push({ pathname: `${BASE}/new` as never, params: { kind: 'sibling' } }) },
      { text: t('kind.return'), onPress: () => router.push({ pathname: `${BASE}/new` as never, params: { kind: 'return' } }) },
      { text: t('common:cancel'), style: 'cancel' },
    ]);

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
          headerRight: request
            ? () => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('new_title')}
                  hitSlop={8}
                  onPress={askNew}
                  className="size-8 items-center justify-center">
                  <Icon name="add" tone="primary" />
                </Pressable>
              )
            : undefined,
        }}
      />
      <PagedList
        query={list}
        keyOf={(tr) => tr.uuid}
        header={
          <Chips
            options={(['open', 'done', 'closed', 'all'] as TransferFilter[]).map((k) => ({ key: k, label: t(`filter.${k}`) }))}
            value={filter}
            onChange={setFilter}
          />
        }
        empty={{ icon: 'transfer', title: query ? t('no_results') : t('empty_title'), body: query ? undefined : t('empty_body') }}
        renderItem={({ item }) => (
          <CardRow
            title={item.transfer_no}
            subtitle={t(item.role === 'sender' ? 'to' : 'from', { name: counterparty(item).name })}
            meta={`${t(`kind.${item.kind}`)} · ${t('items', { count: item.item_count })} · ${f.date(item.created_at)}`}
            trailing={<StatusPill label={t(`status.${item.status}`)} tone={transferTone(item.status)} />}
            onPress={() => router.push(href(item.uuid))}
          />
        )}
      />
    </>
  );
}

export function TransferDetailScreen() {
  const { t } = useTranslation('transfers');
  const f = useFormat();
  const qc = useQueryClient();
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const q = useTransfer(uuid);
  const [busy, setBusy] = useState<TransferStatus | null>(null);
  const tr = q.data;

  const run = async (status: TransferStatus, reason?: string) => {
    setBusy(status);
    try {
      const out = await moveTransfer(uuid, status, reason);
      qc.setQueryData(transferKeys.one(uuid), out);
      void qc.invalidateQueries({ queryKey: ['transfers', 'list'] });
      void qc.invalidateQueries({ queryKey: ['stock'] });
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(null);
    }
  };

  const [reasonFor, setReasonFor] = useState<TransferStatus | null>(null);
  const ask = (status: TransferStatus) => {
    if (NEEDS_REASON.includes(status)) return setReasonFor(status);
    Alert.alert(t(`confirm.${status}`), tr?.transfer_no, [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t(`action.${status}`), onPress: () => void run(status) },
    ]);
  };

  const actions = FLOW.filter((s) => tr?.available_transitions.includes(s));

  return (
    <>
      <Stack.Screen options={{ title: tr?.transfer_no ?? '', headerLargeTitle: false }} />
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {q.isPending ? (
          <ListSkeleton rows={4} inset={false} />
        ) : !tr ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : (
          <>
            <View className="flex-row items-center justify-between gap-3">
              <Text variant="title" selectable>
                {tr.transfer_no}
              </Text>
              <StatusPill label={t(`status.${tr.status}`)} tone={transferTone(tr.status)} />
            </View>

            {actions.length ? (
              <View className="gap-2">
                {actions.map((s) => (
                  <Button
                    key={s}
                    title={t(`action.${s}`)}
                    variant={NEEDS_REASON.includes(s) ? 'outline' : 'primary'}
                    loading={busy === s}
                    disabled={!!busy}
                    onPress={() => ask(s)}
                  />
                ))}
              </View>
            ) : null}

            <List>
              <InfoRow label={t('kind_label')} value={t(`kind.${tr.kind}`)} />
              <InfoRow label={t('sender')} value={tr.sender.name} />
              <InfoRow label={t('receiver')} value={tr.receiver.name} />
              <InfoRow label={t('created')} value={f.dateTime(tr.created_at)} />
              {tr.shipped_at ? <InfoRow label={t('shipped_at')} value={f.dateTime(tr.shipped_at)} /> : null}
              {tr.received_at ? <InfoRow label={t('received_at')} value={f.dateTime(tr.received_at)} /> : null}
              <InfoRow label={t('total')} value={tr.total ? f.money(tr.total, tr.currency) : '–'} last />
            </List>

            {tr.note || tr.decision_note || tr.cancel_reason ? (
              <View className="gap-1.5 rounded-lg border border-border bg-card px-4 py-3">
                {tr.note ? <Text variant="callout">{tr.note}</Text> : null}
                {tr.decision_note ? (
                  <Text variant="callout" tone="muted">
                    {tr.decision_note}
                  </Text>
                ) : null}
                {tr.cancel_reason ? (
                  <Text variant="callout" tone="danger">
                    {tr.cancel_reason}
                  </Text>
                ) : null}
              </View>
            ) : null}

            <SectionHeader title={t('items', { count: tr.items?.length ?? tr.item_count })} />
            {tr.items?.length ? (
              <List>
                {tr.items.map((it, i, arr) => (
                  <ListRow
                    key={it.uuid}
                    icon="barcode"
                    title={it.product.name}
                    subtitle={[
                      it.barcode,
                      it.meters ? `${f.number(it.meters, 2)} m` : it.quantity ? t('qty', { count: it.quantity }) : null,
                      it.received ? t('item_received') : it.shipped ? t('item_shipped') : null,
                    ]
                      .filter(Boolean)
                      .join(' · ')}
                    last={i === arr.length - 1}
                  />
                ))}
              </List>
            ) : null}
          </>
        )}
      </Screen>
      {reasonFor ? (
        <ReasonSheet status={reasonFor} onClose={() => setReasonFor(null)} onConfirm={(r) => void run(reasonFor, r)} />
      ) : null}
    </>
  );
}

type Line = { barcode: string; quantity: string };

/** New request: pick the target (a sibling, or the parent for a return), scan the units, send. */
export function TransferNewScreen() {
  const { t } = useTranslation('transfers');
  const qc = useQueryClient();
  const params = useLocalSearchParams<{ kind?: string }>();
  const kind: TransferKind = params.kind === 'return' ? 'return' : 'sibling';
  const targets = useTransferTargets(kind);
  const [target, setTarget] = useState<string | null>(null);
  const [lines, setLines] = useState<Line[]>([]);
  const [manual, setManual] = useState('');
  const [note, setNote] = useState('');
  const [scanning, setScanning] = useState(false);
  const [touched, setTouched] = useState(false);
  const [busy, setBusy] = useState(false);

  // A return has one possible target, the direct parent.
  const chosen = kind === 'return' ? (targets.data?.[0]?.uuid ?? null) : target;

  // The scanner calls back between renders: the ref sees codes added a moment ago.
  const seen = useRef(new Set<string>());
  const add = (code: string) => {
    const barcode = code.trim();
    if (!barcode || seen.current.has(barcode)) return false;
    seen.current.add(barcode);
    setLines((ls) => [...ls, { barcode, quantity: '' }]);
    return true;
  };
  const removeLine = (barcode: string) => {
    seen.current.delete(barcode);
    setLines((ls) => ls.filter((x) => x.barcode !== barcode));
  };

  const onScan = async (code: string): Promise<ScanOutcome> =>
    add(code) ? { ok: true, title: code.trim() } : { ok: false, title: t('already_added') };

  const submit = async () => {
    setTouched(true);
    if (!chosen || !lines.length || lines.some((l) => !validQuantity(l.quantity))) return;
    setBusy(true);
    try {
      const out = await createTransfer({
        kind,
        to_org_uuid: chosen,
        ...(note.trim() ? { note: note.trim() } : {}),
        items: lines.map((l) =>
          l.quantity.trim() ? { barcode: l.barcode, quantity: Number(l.quantity.trim()) } : { barcode: l.barcode },
        ),
      });
      qc.setQueryData(transferKeys.one(out.uuid), out);
      void qc.invalidateQueries({ queryKey: ['transfers', 'list'] });
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      router.replace(`${BASE}/${out.uuid}` as never);
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Stack.Screen options={{ title: t(`new_${kind}`), headerLargeTitle: false }} />
      <Screen keyboardShouldPersistTaps="handled">
        <Text variant="callout" tone="muted">
          {t(`new_${kind}_hint`)}
        </Text>

        <SectionHeader title={t(kind === 'return' ? 'parent' : 'target')} />
        {targets.isPending ? (
          <ListSkeleton rows={2} inset={false} />
        ) : targets.isError ? (
          <ErrorState error={targets.error} onRetry={() => void targets.refetch()} />
        ) : !targets.data?.length ? (
          <Text variant="callout" tone="muted">
            {t('no_targets')}
          </Text>
        ) : (
          <List>
            {targets.data.map((o, i, arr) => (
              <ListRow
                key={o.uuid}
                icon="building"
                title={o.name}
                onPress={kind === 'return' ? undefined : () => setTarget(o.uuid)}
                trailing={chosen === o.uuid ? <Icon name="check" tone="primary" size={18} /> : undefined}
                last={i === arr.length - 1}
              />
            ))}
          </List>
        )}
        {touched && !chosen ? (
          <Text variant="footnote" tone="danger">
            {t('validation.target')}
          </Text>
        ) : null}

        <SectionHeader title={t('units', { count: lines.length })} />
        <Button title={t('scan_units')} icon="scan" variant="secondary" onPress={() => setScanning(true)} />
        <Input
          label={t('barcode')}
          icon="barcode"
          value={manual}
          onChangeText={setManual}
          placeholder={t('barcode_placeholder')}
          autoCapitalize="characters"
          autoCorrect={false}
          returnKeyType="done"
          onSubmitEditing={() => {
            add(manual);
            setManual('');
          }}
        />
        {lines.length ? (
          <View className="gap-2">
            {lines.map((l) => {
              const qtyError = !validQuantity(l.quantity);
              return (
                <View key={l.barcode} className="gap-1.5 rounded-lg border border-border bg-card px-3.5 py-2.5">
                  <View className="flex-row items-center gap-3">
                    <Icon name="barcode" tone="primary" size={18} />
                    <Text variant="callout" className="flex-1 font-mono" numberOfLines={1}>
                      {l.barcode}
                    </Text>
                    <TextInput
                      value={l.quantity}
                      onChangeText={(v) => setLines((ls) => ls.map((x) => (x.barcode === l.barcode ? { ...x, quantity: v } : x)))}
                      accessibilityLabel={t('quantity')}
                      placeholder={t('quantity_placeholder')}
                      placeholderTextColorClassName="accent-muted-foreground"
                      keyboardType="number-pad"
                      className={`min-h-9 w-20 rounded-md border bg-background px-2 text-right text-[15px] text-foreground tabular-nums ${qtyError ? 'border-destructive' : 'border-border'}`}
                    />
                    <Pressable
                      accessibilityRole="button"
                      accessibilityLabel={t('remove')}
                      hitSlop={8}
                      onPress={() => removeLine(l.barcode)}
                      className="size-8 items-center justify-center rounded-md active:bg-muted">
                      <Icon name="close" tone="muted-foreground" size={16} />
                    </Pressable>
                  </View>
                  {qtyError ? (
                    <Text variant="footnote" tone="danger">
                      {t('validation.quantity')}
                    </Text>
                  ) : null}
                </View>
              );
            })}
            <Text variant="footnote" tone="muted">
              {t('quantity_hint')}
            </Text>
          </View>
        ) : touched ? (
          <Text variant="footnote" tone="danger">
            {t('validation.units')}
          </Text>
        ) : null}

        <Input
          label={t('note')}
          icon="edit"
          value={note}
          onChangeText={setNote}
          placeholder={t('note_placeholder')}
          multiline
          maxLength={1000}
        />
        <Button title={t('send')} size="lg" loading={busy} onPress={() => void submit()} />
      </Screen>
      <ContinuousScanner visible={scanning} onClose={() => setScanning(false)} title={t('scan_units')} onCode={onScan} />
    </>
  );
}
