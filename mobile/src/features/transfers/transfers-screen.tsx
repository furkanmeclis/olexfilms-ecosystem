import { useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, KeyboardAvoidingView, Modal, Platform, Pressable, ScrollView, TextInput, View } from 'react-native';

import { hasAmount } from '@/core/price-visibility';
import { usePricesVisible } from '@/core/use-prices-visible';
import { isApiError } from '@/core/api/errors';
import { errorMessage, i18n } from '@/core/i18n';
import { useSession } from '@/core/auth/session';
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
import { RefRow } from '@/features/entity/ref-row';
import { PickerField, PickerSheet } from '@/features/picker-sheet';
import { newestFirst } from '@/features/orders/order-summary';
import { ContinuousScanner, type ScanOutcome } from '@/features/warehouse/continuous-scanner';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { StepTimeline } from '@/ui/step-timeline';
import { Text } from '@/ui/text';
import { PullRefresh } from '@/ui/pull-refresh';

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
  useReturnOrders,
  useReturnOrderUnits,
  useReturnSources,
  useTransfer,
  useTransfers,
  useTransferTargets,
} from './api';
import {
  buildCreateBody,
  lineFromOrderUnit,
  lineQuantityValid,
  type ReturnOrderUnit,
  toggleLine,
  type TransferLine,
} from './returns';

export { validQuantity } from './returns';

const BASE = '/bayi/menu/transfers';
/** Flow order of the action buttons (the server decides which are allowed). */
const FLOW: TransferStatus[] = ['approved', 'shipped', 'received', 'rejected', 'cancelled'];

/** The detail code first (e.g. TRANSFER_ORDER_MISMATCH under VALIDATION_ERROR), then the error code (web transferErrorMessage). */
function failMessage(e: unknown) {
  if (!isApiError(e)) return i18n.t('common:error_generic');
  for (const code of [e.details?.[0]?.code, e.code]) {
    if (code && i18n.exists(`transfers:errors.${code}`)) return i18n.t(`transfers:errors.${code}`);
  }
  return errorMessage(e.code);
}

function alertError(e: unknown) {
  void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
  Alert.alert(failMessage(e));
}

function counterparty(tr: Transfer) {
  return tr.role === 'sender' ? tr.receiver : tr.sender;
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
  // TEC-1220: a parent with transfers.approve opens returns in a child's name.
  const approve = useCan('transfers.approve');
  const [filter, setFilter] = useState<TransferFilter>('open');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const list = useTransfers(filter, query);

  const newReturn = () => router.push({ pathname: `${BASE}/new` as never, params: { kind: 'return' } });
  // A sibling transfer needs transfers.request; without it only a return for a child is left.
  const askNew = () =>
    request
      ? Alert.alert(t('new_title'), undefined, [
          { text: t('kind.sibling'), onPress: () => router.push({ pathname: `${BASE}/new` as never, params: { kind: 'sibling' } }) },
          { text: t('kind.return'), onPress: newReturn },
          { text: t('common:cancel'), style: 'cancel' },
        ])
      : newReturn();

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
          headerRight: request || approve
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
  const prices = usePricesVisible();
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
      <Screen refreshControl={<PullRefresh busy={q.isRefetching} onRefresh={() => void q.refetch()} />}>
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
              <RefRow kind="organization" label={t('sender')} value={tr.sender} />
              <RefRow kind="organization" label={t('receiver')} value={tr.receiver} />
              {/* TEC-1085: the order the return refers to and a return the parent opened in the giver's name. */}
              {tr.order ? <InfoRow label={t('return.order')} value={tr.order.order_no} /> : null}
              {tr.opened_by_parent ? (
                <InfoRow label={t('return.opened_by')} value={t('return.opened_by_parent', { parent: tr.parent.name })} />
              ) : null}
              <InfoRow label={t('created')} value={f.dateTime(tr.created_at)} />
              {tr.shipped_at ? <InfoRow label={t('shipped_at')} value={f.dateTime(tr.shipped_at)} /> : null}
              {tr.received_at ? <InfoRow label={t('received_at')} value={f.dateTime(tr.received_at)} /> : null}
              {/* TEC-1051: no amount for a warehouse role without pricing permissions. */}
              {prices ? <InfoRow label={t('total')} value={hasAmount(tr.total) ? f.money(tr.total, tr.currency) : '–'} last /> : null}
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

            {/* TEC-1220 (revize 5.4.1): who took each step of the transfer. */}
            <StepTimeline
              title={t('timeline')}
              steps={newestFirst(tr.timeline).map((s, i) => ({
                key: `${s.action}-${s.at}-${i}`,
                user: s.user?.name ?? t('orders:timeline.system'),
                label: t(`status.${s.action}`),
                when: f.dateTime(s.at),
              }))}
            />
          </>
        )}
      </Screen>
      {reasonFor ? (
        <ReasonSheet status={reasonFor} onClose={() => setReasonFor(null)} onConfirm={(r) => void run(reasonFor, r)} />
      ) : null}
    </>
  );
}

const OWN = 'own';
const NO_ORDER = 'none';

/**
 * New request: pick the target (a sibling, or the parent for a return), scan the units, send.
 *
 * TEC-1220 (web TEC-1085): a parent with transfers.approve may open the return in the
 * name of a direct child (it starts approved), and either side may link it to the order
 * the parent sold the units on; the order's units are offered for picking. The `from`
 * and `order` params preselect both (the order page's "Create return").
 */
export function TransferNewScreen() {
  const { t } = useTranslation('transfers');
  const qc = useQueryClient();
  const params = useLocalSearchParams<{ kind?: string; from?: string; order?: string }>();
  const canRequest = useCan('transfers.request');
  const canApprove = useCan('transfers.approve');
  const activeOrg = useSession((s) => s.me?.active_organization_uuid ?? null);
  // Only the parent opens a return without transfers.request; a sibling transfer needs it.
  const kind: TransferKind = params.kind === 'return' || !canRequest ? 'return' : 'sibling';
  const isReturn = kind === 'return';
  const canOpenForChild = isReturn && canApprove;
  const targets = useTransferTargets(kind, canRequest);
  const sources = useReturnSources(canOpenForChild);
  const children = sources.data ?? [];
  const ownParent = isReturn ? targets.data?.[0] : undefined;
  const [fromOrg, setFromOrg] = useState(isReturn ? (params.from ?? '') : '');
  const [orderUuid, setOrderUuid] = useState(isReturn ? (params.order ?? '') : '');
  const forChild = isReturn && fromOrg !== '';
  const [target, setTarget] = useState<string | null>(null);
  const [picking, setPicking] = useState<'target' | 'source' | 'order' | null>(null);
  const [lines, setLines] = useState<TransferLine[]>([]);
  const [manual, setManual] = useState('');
  const [note, setNote] = useState('');
  const [scanning, setScanning] = useState(false);
  const [touched, setTouched] = useState(false);
  const [busy, setBusy] = useState(false);

  // A return has one possible target: the direct parent, or the active organization
  // itself for a return opened for a child.
  const chosen = isReturn ? (forChild ? activeOrg : (ownParent?.uuid ?? null)) : target;
  const orderPicker = isReturn && (forChild || !!ownParent);
  const orders = useReturnOrders(fromOrg, orderPicker && (forChild ? canOpenForChild : canRequest));
  const units = useReturnOrderUnits(orderUuid, fromOrg, orderPicker && (forChild ? canOpenForChild : canRequest));
  const orderList = orders.data ?? [];
  const pickedOrder = orderList.find((o) => o.uuid === orderUuid);

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
  const toggleUnit = (u: ReturnOrderUnit) => {
    const line = lineFromOrderUnit(u);
    if (seen.current.has(line.barcode)) seen.current.delete(line.barcode);
    else seen.current.add(line.barcode);
    setLines((ls) => toggleLine(ls, line));
  };
  const changeSource = (v: string) => {
    // Another giver: its orders and units differ.
    setFromOrg(v);
    setOrderUuid('');
    seen.current.clear();
    setLines([]);
  };

  const onScan = async (code: string): Promise<ScanOutcome> =>
    add(code) ? { ok: true, title: code.trim() } : { ok: false, title: t('already_added') };

  const submit = async () => {
    setTouched(true);
    const body = buildCreateBody(chosen, lines, note, kind, {
      fromOrgUuid: forChild ? fromOrg : undefined,
      orderUuid: isReturn ? orderUuid : undefined,
    });
    if (!body) return;
    setBusy(true);
    try {
      const out = await createTransfer(body);
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

  const sourceName = forChild
    ? (children.find((o) => o.uuid === fromOrg)?.name ?? null)
    : canRequest && ownParent
      ? t('return.source_own', { parent: ownParent.name })
      : null;
  const sourceOptions = [
    ...(canRequest && ownParent ? [{ key: OWN, title: t('return.source_own', { parent: ownParent.name }), icon: 'building' as const }] : []),
    ...children.map((o) => ({ key: o.uuid, title: o.name, icon: 'building' as const })),
  ];
  const showSource = canOpenForChild && children.length > 0;
  const loadingTarget = isReturn ? (canRequest && targets.isPending) || (canOpenForChild && sources.isPending) : targets.isPending;

  return (
    <>
      <Stack.Screen options={{ title: t(`new_${kind}`), headerLargeTitle: false }} />
      <Screen keyboardShouldPersistTaps="handled">
        <Text variant="callout" tone="muted">
          {t(forChild ? 'return.hint_parent' : `new_${kind}_hint`)}
        </Text>

        <SectionHeader title={t(showSource ? 'return.source' : isReturn ? 'parent' : 'target')} />
        {loadingTarget ? (
          <ListSkeleton rows={2} inset={false} />
        ) : targets.isError ? (
          <ErrorState error={targets.error} onRetry={() => void targets.refetch()} />
        ) : showSource ? (
          // TEC-1085: our own return to the parent, or a child's return opened in its name.
          <>
            <PickerField
              icon="building"
              value={sourceName}
              placeholder={t('return.source_placeholder')}
              error={touched && !chosen ? t('validation.target') : undefined}
              onPress={() => setPicking('source')}
            />
            <Text variant="footnote" tone="muted">
              {t('return.source_hint')}
            </Text>
            <PickerSheet
              visible={picking === 'source'}
              title={t('return.source')}
              options={sourceOptions}
              selectedKey={forChild ? fromOrg : canRequest && ownParent ? OWN : null}
              onPick={(o) => changeSource(o.key === OWN ? '' : o.key)}
              onClose={() => setPicking(null)}
            />
          </>
        ) : !targets.data?.length ? (
          <Text variant="callout" tone="muted">
            {t('no_targets')}
          </Text>
        ) : isReturn ? (
          // A return goes to the direct parent: shown, nothing to pick.
          <List>
            {targets.data.map((o, i, arr) => (
              <ListRow
                key={o.uuid}
                icon="building"
                title={o.name}
                trailing={chosen === o.uuid ? <Icon name="check" tone="primary" size={18} /> : undefined}
                last={i === arr.length - 1}
              />
            ))}
          </List>
        ) : (
          <>
            <PickerField
              icon="building"
              value={targets.data.find((o) => o.uuid === chosen)?.name}
              placeholder={t('validation.target')}
              error={touched && !chosen ? t('validation.target') : undefined}
              onPress={() => setPicking('target')}
            />
            <PickerSheet
              visible={picking === 'target'}
              title={t('target')}
              options={targets.data.map((o) => ({ key: o.uuid, title: o.name, icon: 'building' as const }))}
              selectedKey={chosen}
              onPick={(o) => setTarget(o.key)}
              onClose={() => setPicking(null)}
            />
          </>
        )}
        {touched && !chosen && isReturn && !showSource ? (
          <Text variant="footnote" tone="danger">
            {t('validation.target')}
          </Text>
        ) : null}

        {orderPicker ? (
          <>
            <SectionHeader title={t('return.order')} />
            <PickerField
              icon="order"
              // A preselected order (from the order page) shows its uuid until the list loads.
              value={orderUuid === '' ? t('return.order_none') : (pickedOrder?.order_no ?? orderUuid)}
              placeholder={t('return.order_none')}
              onPress={() => setPicking('order')}
            />
            <Text variant="footnote" tone="muted">
              {!orders.isPending && !orders.isError && orderList.length === 0 ? t('return.no_orders') : t('return.order_hint')}
            </Text>
            <PickerSheet
              visible={picking === 'order'}
              title={t('return.order')}
              options={[
                { key: NO_ORDER, title: t('return.order_none') },
                ...orderList.map((o) => ({ key: o.uuid, title: o.order_no, icon: 'order' as const, keywords: o.order_no })),
              ]}
              selectedKey={orderUuid === '' ? NO_ORDER : orderUuid}
              selectedLabel={pickedOrder?.order_no ?? null}
              loading={orders.isPending}
              error={orders.error}
              onRetry={() => void orders.refetch()}
              onPick={(o) => setOrderUuid(o.key === NO_ORDER ? '' : o.key)}
              onClose={() => setPicking(null)}
            />
            {orderUuid !== '' ? (
              <>
                <SectionHeader title={t('return.order_units')} />
                {units.isPending ? (
                  <ListSkeleton rows={2} inset={false} />
                ) : units.isError ? (
                  <ErrorState error={units.error} onRetry={() => void units.refetch()} />
                ) : !units.data?.length ? (
                  <Text variant="callout" tone="muted">
                    {t('return.order_units_empty')}
                  </Text>
                ) : (
                  <List>
                    {units.data.map((u, i, arr) => {
                      // A unit the giver no longer holds is shown but cannot be picked.
                      const held = u.returnable > 0;
                      const on = lines.some((l) => l.barcode === u.barcode.trim());
                      return (
                        <ListRow
                          key={u.unit_uuid}
                          icon="barcode"
                          title={u.product.name}
                          subtitle={held ? u.barcode : `${u.barcode} · ${t('return.not_held')}`}
                          trailing={
                            <Icon name={on ? 'check' : 'add'} tone={held ? 'primary' : 'muted-foreground'} size={18} />
                          }
                          onPress={held ? () => toggleUnit(u) : undefined}
                          last={i === arr.length - 1}
                        />
                      );
                    })}
                  </List>
                )}
              </>
            ) : null}
          </>
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
              const qtyError = !lineQuantityValid(l);
              return (
                <View key={l.barcode} className="gap-1.5 rounded-lg border border-border bg-card px-3.5 py-2.5">
                  <View className="flex-row items-center gap-3">
                    <Icon name="barcode" tone="primary" size={18} />
                    <View className="flex-1">
                      <Text variant="callout" className="font-mono" numberOfLines={1}>
                        {l.barcode}
                      </Text>
                      {l.label ? (
                        <Text variant="footnote" tone="muted" numberOfLines={1}>
                          {l.label}
                        </Text>
                      ) : null}
                    </View>
                    {/* A picked serial piece or roll moves whole: no quantity. */}
                    {l.fixed === false ? null : (
                      <TextInput
                        value={l.quantity}
                        onChangeText={(v) => setLines((ls) => ls.map((x) => (x.barcode === l.barcode ? { ...x, quantity: v } : x)))}
                        accessibilityLabel={t('quantity')}
                        placeholder={l.fixed ? '1' : t('quantity_placeholder')}
                        placeholderTextColorClassName="accent-muted-foreground"
                        keyboardType="number-pad"
                        className={`min-h-9 w-20 rounded-md border bg-background px-2 text-right text-[15px] text-foreground tabular-nums ${qtyError ? 'border-destructive' : 'border-border'}`}
                      />
                    )}
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
                      {l.fixed ? t('return.quantity_max', { max: l.max ?? 1 }) : t('validation.quantity')}
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
        <Button title={t(forChild ? 'return.submit_parent' : 'send')} size="lg" loading={busy} onPress={() => void submit()} />
      </Screen>
      <ContinuousScanner visible={scanning} onClose={() => setScanning(false)} title={t('scan_units')} onCode={onScan} />
    </>
  );
}
