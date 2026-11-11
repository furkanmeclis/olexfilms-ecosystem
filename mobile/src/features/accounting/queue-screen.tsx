import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, FlatList, KeyboardAvoidingView, Modal, Platform, Pressable, ScrollView, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { api, unwrap } from '@/core/api/client';
import { orderTone } from '@/features/orders/orders-screen';
import { formatDateTime, formatNumber, PAGE_SIZE, usePagedList } from '@/features/services/lib';
import { Chips, errorText, ListBody, ListFooter } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { KpiCard, KpiGrid } from '@/ui/kpi';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { PullRefresh } from '@/ui/pull-refresh';

import { accountingKey, useAccountingAccess, useAccountingLinks } from './hooks';
import { formatMoney } from './lib';
import {
  canMarkInvoiced,
  type InvoiceErrors,
  type InvoiceForm,
  invoiceInput,
  invoiceStatusTone,
  processInput,
  QUEUE_NOTE_MAX,
  QUEUE_TABS,
  type QueueReturn,
  type QueueTab,
  queueCount,
  queueOrdersQuery,
  todayISO,
  validateInvoice,
} from './queue';

const TAB_ICONS = { orders: 'accounting', transit: 'truck', returns: 'undo' } as const;

/**
 * TEC-1071: accounting queue on mobile (web TEC-1070, legacy olexfilms-warehouse
 * "Muhasebe kuyruğu"). Counters on top; the tabs list received sales to invoice
 * (mark "Faturalandı" with einvoice.manage), sales on the road and received
 * returns to mark processed (accounting.write).
 */
export function AccountingQueueScreen() {
  const { t, i18n } = useTranslation('accounting');
  const access = useAccountingAccess();
  const key = [...accountingKey(access.orgUuid), 'queue'] as const;
  const [tab, setTab] = useState<QueueTab>('orders');

  const summary = useQuery({
    queryKey: [...key, 'summary'],
    queryFn: () => unwrap(api.GET('/v1/accounting/queue/summary')),
  });

  const header = (
    <View className="gap-3 pb-1 pt-2">
      <Text variant="footnote" tone="muted">
        {t('queue.description')}
      </Text>
      <KpiGrid>
        {QUEUE_TABS.map((id) => {
          const n = queueCount(summary.data, id);
          return (
            <KpiCard
              key={id}
              icon={TAB_ICONS[id]}
              label={t(`queue.tabs.${id}`)}
              value={n === undefined ? '—' : formatNumber(n, i18n.language)}
              loading={summary.isPending}
              onPress={() => setTab(id)}
            />
          );
        })}
      </KpiGrid>
      <Chips<QueueTab> value={tab} onChange={setTab} options={QUEUE_TABS.map((id) => ({ value: id, label: t(`queue.tabs.${id}`) }))} />
      {tab === 'transit' ? (
        <Text variant="footnote" tone="muted">
          {t('queue.transit_hint')}
        </Text>
      ) : null}
    </View>
  );

  return (
    <>
      <Stack.Screen options={{ title: t('queue.title') }} />
      {tab === 'returns' ? (
        <QueueReturns header={header} onChanged={() => void summary.refetch()} />
      ) : (
        <QueueOrders key={tab} tab={tab} header={header} onChanged={() => void summary.refetch()} />
      )}
    </>
  );
}

function QueueOrders({ tab, header, onChanged }: { tab: 'orders' | 'transit'; header: React.ReactElement; onChanged: () => void }) {
  const { t, i18n } = useTranslation('accounting');
  const access = useAccountingAccess();
  const links = useAccountingLinks();
  const qc = useQueryClient();
  const key = [...accountingKey(access.orgUuid), 'queue', tab] as const;
  const canInvoice = canMarkInvoiced(tab, access.canManageEinvoices);
  const [invoicing, setInvoicing] = useState<{ uuid: string; no: string } | null>(null);

  const list = usePagedList(key, (offset) =>
    unwrap(api.GET('/v1/orders', { params: { query: { ...queueOrdersQuery(tab), limit: PAGE_SIZE, offset } } })),
  );

  return (
    <>
      <FlatList
        data={list.items}
        keyExtractor={(o) => o.uuid}
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2 px-4 pb-10"
        ListHeaderComponent={header}
        ListEmptyComponent={
          <ListBody
            isPending={list.isPending}
            error={list.error}
            count={list.items.length}
            onRetry={() => void list.refetch()}
            empty={{ icon: 'inbox', title: t('queue.orders_empty') }}
          />
        }
        renderItem={({ item: o }) => {
          const at = tab === 'orders' ? o.received_at : (o.shipped_at ?? o.created_at);
          return (
            <Pressable
              accessibilityRole="button"
              onPress={() => router.push(links.order(o.uuid))}
              className="gap-1.5 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
              <View className="flex-row items-center justify-between gap-3">
                <Text variant="headline" numberOfLines={1} className="flex-1 font-mono">
                  {o.order_no}
                </Text>
                {o.total != null ? (
                  <Text variant="callout" className="font-semibold tabular-nums">
                    {formatMoney(o.total, o.currency, i18n.language)}
                  </Text>
                ) : null}
              </View>
              <Text variant="callout" numberOfLines={1}>
                {o.buyer.name}
              </Text>
              <View className="flex-row flex-wrap items-center gap-2">
                <StatusPill label={o.status_label} tone={orderTone(o.status)} />
                <StatusPill label={t(`queue.invoice_status.${o.invoice_status}`)} tone={invoiceStatusTone(o.invoice_status)} />
              </View>
              <Text variant="footnote" tone="muted" numberOfLines={1}>
                {`${t(tab === 'orders' ? 'queue.received_at' : 'queue.shipped_at')}: ${at ? formatDateTime(at, i18n.language) : '—'}`}
              </Text>
              {canInvoice ? (
                <Button
                  title={t('queue.mark_invoiced')}
                  icon="check"
                  variant="outline"
                  className="mt-1"
                  onPress={() => setInvoicing({ uuid: o.uuid, no: o.order_no })}
                />
              ) : null}
            </Pressable>
          );
        }}
        onEndReachedThreshold={0.5}
        onEndReached={() => {
          if (list.hasNextPage && !list.isFetchingNextPage) void list.fetchNextPage();
        }}
        ListFooterComponent={<ListFooter loading={list.isFetchingNextPage} />}
        refreshControl={
          <PullRefresh
            busy={list.isRefetching && !list.isFetchingNextPage}
            onRefresh={() => {
              onChanged();
              void list.refetch();
            }}
          />
        }
      />
      <InvoiceSheet
        order={invoicing}
        onClose={() => setInvoicing(null)}
        onDone={() => {
          setInvoicing(null);
          onChanged();
          void qc.invalidateQueries({ queryKey: key });
        }}
      />
    </>
  );
}

function QueueReturns({ header, onChanged }: { header: React.ReactElement; onChanged: () => void }) {
  const { t, i18n } = useTranslation('accounting');
  const access = useAccountingAccess();
  const qc = useQueryClient();
  const [processed, setProcessed] = useState(false);
  const [marking, setMarking] = useState<QueueReturn | null>(null);
  const key = [...accountingKey(access.orgUuid), 'queue', 'returns', { processed }] as const;

  const list = usePagedList(key, (offset) =>
    unwrap(api.GET('/v1/accounting/queue/returns', { params: { query: { processed, limit: PAGE_SIZE, offset } } })),
  );

  const changed = () => {
    onChanged();
    void qc.invalidateQueries({ queryKey: [...accountingKey(access.orgUuid), 'queue', 'returns'] });
  };

  const unmark = useMutation({
    mutationFn: (uuid: string) =>
      unwrap(api.DELETE('/v1/accounting/queue/returns/{uuid}/processed', { params: { path: { uuid } } })),
    onSuccess: () => {
      changed();
      Alert.alert(t('queue.unmarked'));
    },
    onError: (e) => Alert.alert(t('queue.mark_failed'), errorText(e)),
  });

  const confirmUnmark = (r: QueueReturn) =>
    Alert.alert(t('queue.unmark'), r.transfer_no, [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t('queue.unmark'), onPress: () => unmark.mutate(r.uuid) },
    ]);

  return (
    <>
      <FlatList
        data={list.items}
        keyExtractor={(r) => r.uuid}
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2 px-4 pb-10"
        ListHeaderComponent={
          <View className="gap-3 pb-1">
            {header}
            <Chips<'waiting' | 'processed'>
              value={processed ? 'processed' : 'waiting'}
              onChange={(v) => setProcessed(v === 'processed')}
              options={[
                { value: 'waiting', label: t('queue.filter_waiting') },
                { value: 'processed', label: t('queue.filter_processed') },
              ]}
            />
          </View>
        }
        ListEmptyComponent={
          <ListBody
            isPending={list.isPending}
            error={list.error}
            count={list.items.length}
            onRetry={() => void list.refetch()}
            empty={{ icon: 'inbox', title: t('queue.returns_empty') }}
          />
        }
        renderItem={({ item: r }) => (
          <View className="gap-1.5 rounded-lg border border-border bg-card px-4 py-3">
            <View className="flex-row items-center justify-between gap-3">
              <Text variant="headline" numberOfLines={1} className="flex-1 font-mono">
                {r.transfer_no}
              </Text>
              {r.processed_at ? (
                <StatusPill label={t('queue.processed')} tone="done" />
              ) : (
                <StatusPill label={t('queue.waiting')} tone="progress" />
              )}
            </View>
            <Text variant="callout" numberOfLines={1}>
              {r.sender.name}
            </Text>
            <Text variant="footnote" tone="muted" numberOfLines={2}>
              {[
                t('queue.item_count', { count: r.item_count }),
                r.total ? formatMoney(r.total, r.currency, i18n.language) : null,
                r.received_at ? `${t('queue.received_at')}: ${formatDateTime(r.received_at, i18n.language)}` : null,
              ]
                .filter(Boolean)
                .join(' · ')}
            </Text>
            {r.processed_at ? (
              <Text variant="footnote" tone="muted" numberOfLines={3}>
                {[`${t('queue.processed')}: ${formatDateTime(r.processed_at, i18n.language)}`, r.processed_by, r.note]
                  .filter(Boolean)
                  .join(' · ')}
              </Text>
            ) : null}
            {access.canWrite ? (
              r.processed_at ? (
                <Button
                  title={t('queue.unmark')}
                  icon="undo"
                  variant="ghost"
                  loading={unmark.isPending && unmark.variables === r.uuid}
                  onPress={() => confirmUnmark(r)}
                />
              ) : (
                <Button title={t('queue.mark_processed')} icon="check" variant="outline" className="mt-1" onPress={() => setMarking(r)} />
              )
            ) : null}
          </View>
        )}
        onEndReachedThreshold={0.5}
        onEndReached={() => {
          if (list.hasNextPage && !list.isFetchingNextPage) void list.fetchNextPage();
        }}
        ListFooterComponent={<ListFooter loading={list.isFetchingNextPage} />}
        refreshControl={
          <PullRefresh
            busy={list.isRefetching && !list.isFetchingNextPage}
            onRefresh={() => {
              onChanged();
              void list.refetch();
            }}
          />
        }
      />
      <ProcessSheet
        item={marking}
        onClose={() => setMarking(null)}
        onDone={() => {
          setMarking(null);
          changed();
        }}
      />
    </>
  );
}

/** Page sheet with a title and a close button; the body mounts only while open. */
function FormSheet({ title, subtitle, visible, onClose, children }: {
  title: string;
  subtitle?: string;
  visible: boolean;
  onClose: () => void;
  children: React.ReactNode;
}) {
  const { t } = useTranslation('common');
  const insets = useSafeAreaInsets();
  return (
    <Modal visible={visible} animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      {visible ? (
        <KeyboardAvoidingView
          behavior={Platform.OS === 'ios' ? 'padding' : undefined}
          className="flex-1 bg-background"
          style={{ paddingTop: Platform.OS === 'ios' ? 0 : insets.top }}>
          <View className="flex-row items-center justify-between gap-2 pb-2 pe-2 ps-5 pt-4">
            <View className="flex-1">
              <Text variant="title" numberOfLines={1}>
                {title}
              </Text>
              {subtitle ? (
                <Text variant="footnote" tone="muted" numberOfLines={1} className="font-mono">
                  {subtitle}
                </Text>
              ) : null}
            </View>
            <Button title={t('close')} variant="ghost" onPress={onClose} />
          </View>
          <ScrollView keyboardShouldPersistTaps="handled" contentContainerClassName="gap-4 px-4 pb-10 pt-2">
            {children}
          </ScrollView>
        </KeyboardAvoidingView>
      ) : null}
    </Modal>
  );
}

/** "Faturalandı": invoice number, date and note; every line not invoiced yet goes on the record. */
function InvoiceSheet({ order, onClose, onDone }: { order: { uuid: string; no: string } | null; onClose: () => void; onDone: () => void }) {
  const { t } = useTranslation('accounting');
  return (
    <FormSheet title={t('queue.mark_invoiced')} subtitle={order?.no} visible={!!order} onClose={onClose}>
      {order ? <InvoiceFields uuid={order.uuid} onDone={onDone} /> : null}
    </FormSheet>
  );
}

function InvoiceFields({ uuid, onDone }: { uuid: string; onDone: () => void }) {
  const { t } = useTranslation('accounting');
  const [form, setForm] = useState<InvoiceForm>({ invoice_no: '', invoice_date: todayISO(), note: '' });
  const [errors, setErrors] = useState<InvoiceErrors>({});
  const set = <K extends keyof InvoiceForm>(k: K, v: InvoiceForm[K]) => setForm((f) => ({ ...f, [k]: v }));

  const mark = useMutation({
    mutationFn: () => unwrap(api.POST('/v1/orders/{uuid}/invoices', { params: { path: { uuid } }, body: invoiceInput(form) })),
    onSuccess: () => {
      Alert.alert(t('queue.invoiced'));
      onDone();
    },
    onError: (e) => Alert.alert(t('queue.invoice_failed'), errorText(e)),
  });

  const submit = () => {
    const errs = validateInvoice(form);
    setErrors(errs);
    if (!Object.keys(errs).length) mark.mutate();
  };

  return (
    <>
      <Text variant="footnote" tone="muted">
        {t('queue.invoice_hint')}
      </Text>
      <Input
        label={t('queue.invoice_no')}
        placeholder={t('queue.invoice_no_placeholder')}
        value={form.invoice_no}
        onChangeText={(v) => set('invoice_no', v)}
        autoCapitalize="characters"
        autoCorrect={false}
        error={errors.invoice_no ? t('validation.required') : undefined}
      />
      <Input
        label={t('queue.invoice_date')}
        placeholder={t('queue.date_placeholder')}
        value={form.invoice_date}
        onChangeText={(v) => set('invoice_date', v)}
        keyboardType="numbers-and-punctuation"
        autoCorrect={false}
        error={errors.invoice_date ? t('queue.date_invalid') : undefined}
      />
      <Input
        label={t('queue.note')}
        placeholder={t('queue.invoice_note_placeholder')}
        value={form.note}
        onChangeText={(v) => set('note', v)}
        multiline
        maxLength={QUEUE_NOTE_MAX}
        className="min-h-20 py-3"
        error={errors.note ? t('validation.too_long', { max: QUEUE_NOTE_MAX }) : undefined}
      />
      <Button title={t('queue.mark_invoiced')} loading={mark.isPending} onPress={submit} />
    </>
  );
}

/** "İşlendi": an optional note (e.g. the credit note number). */
function ProcessSheet({ item, onClose, onDone }: { item: QueueReturn | null; onClose: () => void; onDone: () => void }) {
  const { t } = useTranslation('accounting');
  return (
    <FormSheet title={t('queue.mark_processed')} subtitle={item?.transfer_no} visible={!!item} onClose={onClose}>
      {item ? <ProcessFields uuid={item.uuid} onDone={onDone} /> : null}
    </FormSheet>
  );
}

function ProcessFields({ uuid, onDone }: { uuid: string; onDone: () => void }) {
  const { t } = useTranslation('accounting');
  const [note, setNote] = useState('');
  const mark = useMutation({
    mutationFn: () =>
      unwrap(api.POST('/v1/accounting/queue/returns/{uuid}/processed', { params: { path: { uuid } }, body: processInput(note) })),
    onSuccess: () => {
      Alert.alert(t('queue.marked'));
      onDone();
    },
    onError: (e) => Alert.alert(t('queue.mark_failed'), errorText(e)),
  });
  return (
    <>
      <Input
        label={t('queue.note')}
        placeholder={t('queue.note_placeholder')}
        value={note}
        onChangeText={setNote}
        multiline
        maxLength={QUEUE_NOTE_MAX}
        className="min-h-20 py-3"
      />
      <Button title={t('queue.mark_processed')} loading={mark.isPending} onPress={() => mark.mutate()} />
    </>
  );
}
