import { useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Platform, Pressable, RefreshControl, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { useCan } from '@/core/spaces';
import { CardRow, Chips, ErrorState, ListSkeleton, PagedList, useChildHref, useDebounced, useFormat } from '@/features/admin/kit';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { resolveScan, type ScanResult } from './api';
import { ContinuousScanner, type ScanOutcome } from './continuous-scanner';
import { alertError, confirmAction, isCode, whError } from './format';
import { Card, Field } from './parts';
import {
  createReclassification,
  decideReclassification,
  type ReclassDecision,
  type ReclassFilter,
  reclassKeys,
  reclassTone,
  useReclassification,
  useReclassifications,
} from './reclass-api';
import { type Product, ProductPicker } from './reserve-sheet';

/** Reclassification requests: the barcode stays, the unit moves to another product once a center approves. */
export function ReclassListScreen() {
  const { t } = useTranslation('warehouse');
  const f = useFormat();
  const href = useChildHref();
  const write = useCan('stock.write');
  const [filter, setFilter] = useState<ReclassFilter>('pending');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const list = useReclassifications(filter, query);

  return (
    <>
      <Stack.Screen
        options={{
          title: t('reclass.title'),
          headerLargeTitle: false,
          headerSearchBarOptions: {
            placeholder: t('reclass.search'),
            hideWhenScrolling: false,
            onChangeText: (e) => setQ(e.nativeEvent.text),
            onCancelButtonPress: () => setQ(''),
          },
          headerRight: write
            ? () => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('reclass.new')}
                  hitSlop={8}
                  onPress={() => router.push(href('new'))}
                  className="size-8 items-center justify-center">
                  <Icon name="add" tone="primary" />
                </Pressable>
              )
            : undefined,
        }}
      />
      <PagedList
        query={list}
        keyOf={(r) => r.uuid}
        header={
          <Chips
            options={(['pending', 'decided', 'all'] as ReclassFilter[]).map((k) => ({ key: k, label: t(`reclass.filter.${k}`) }))}
            value={filter}
            onChange={setFilter}
          />
        }
        empty={{
          icon: 'barcode',
          title: query ? t('reclass.no_results') : t('reclass.empty_title'),
          body: query ? undefined : t('reclass.empty_description'),
        }}
        renderItem={({ item }) => (
          <CardRow
            title={item.unit.barcode}
            subtitle={`${item.from_product.name} → ${item.to_product.name}`}
            meta={`${item.organization.name} · ${f.date(item.created_at)}`}
            trailing={<StatusPill label={t(`reclass.status.${item.status}`)} tone={reclassTone(item.status)} />}
            onPress={() => router.push(href(item.uuid))}
          />
        )}
      />
    </>
  );
}

export function ReclassDetailScreen() {
  const { t } = useTranslation('warehouse');
  const f = useFormat();
  const qc = useQueryClient();
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const me = useSession((s) => s.me);
  const write = useCan('stock.write');
  const decide = useCan('stock.reclassify');
  const q = useReclassification(uuid);
  const [busy, setBusy] = useState<ReclassDecision | null>(null);
  const r = q.data;

  const run = async (decision: ReclassDecision, note?: string) => {
    setBusy(decision);
    try {
      const out = await decideReclassification(uuid, decision, note);
      qc.setQueryData(reclassKeys.one(uuid), out);
      void qc.invalidateQueries({ queryKey: ['warehouse', 'reclass', 'list'] });
      void qc.invalidateQueries({ queryKey: ['stock'] });
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    } catch (e) {
      // Approval is a sensitive step that needs a fresh step-up, which only the web panel offers.
      if (isCode(e, 'STEP_UP_REQUIRED')) Alert.alert(t('reclass.step_up_title'), t('reclass.step_up_body'));
      else alertError(e);
    } finally {
      setBusy(null);
    }
  };

  const ask = (decision: ReclassDecision) => {
    // iOS asks for an optional note in the same dialog; Android has no text prompt.
    if (decision === 'reject' && Platform.OS === 'ios') {
      Alert.prompt(t('reclass.reject_title'), t('reclass.reject_description'), [
        { text: t('common:cancel'), style: 'cancel' },
        { text: t('reclass.reject'), style: 'destructive', onPress: (note?: string) => void run('reject', note?.trim() || undefined) },
      ]);
      return;
    }
    confirmAction({
      title: t(`reclass.${decision}_title`),
      body: t(`reclass.${decision}_description`),
      action: t(`reclass.${decision}`),
      destructive: decision !== 'approve',
      onConfirm: () => void run(decision),
    });
  };

  const pending = r?.status === 'pending';
  const own = !!r && !!me && r.requested_by?.uuid === me.user.uuid;

  return (
    <>
      <Stack.Screen options={{ title: r?.unit.barcode ?? t('reclass.title'), headerLargeTitle: false }} />
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {q.isPending ? (
          <ListSkeleton />
        ) : !r ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : (
          <>
            <Card>
              <View className="flex-row items-center justify-between gap-2">
                <Text variant="headline" className="font-mono" selectable>
                  {r.unit.barcode}
                </Text>
                <StatusPill label={t(`reclass.status.${r.status}`)} tone={reclassTone(r.status)} />
              </View>
              <Field label={t('reclass.from')} value={`${r.from_product.name} · ${r.from_product.sku}`} />
              <Field label={t('reclass.to')} value={`${r.to_product.name} · ${r.to_product.sku}`} />
              <Field label={t('reclass.organization')} value={r.organization.name} />
            </Card>

            <SectionHeader title={t('reclass.reason')} />
            <Card>
              <Text variant="callout" selectable>
                {r.reason}
              </Text>
            </Card>

            <SectionHeader title={t('reclass.history')} />
            <Card>
              <Field label={t('reclass.requested_by')} value={`${r.requested_by?.name ?? '–'} · ${f.dateTime(r.created_at)}`} />
              {r.decided_at ? (
                <Field label={t('reclass.decided_by')} value={`${r.decided_by?.name ?? '–'} · ${f.dateTime(r.decided_at)}`} />
              ) : null}
              <Field label={t('reclass.decision_note')} value={r.decision_note} />
            </Card>

            {pending && decide && !own ? (
              <View className="gap-2.5">
                <Card>
                  <Text variant="headline">{t('reclass.approve_on_web_title')}</Text>
                  <Text variant="footnote" tone="muted">
                    {t('reclass.approve_on_web_body')}
                  </Text>
                </Card>
                <Button
                  title={t('reclass.reject')}
                  variant="outline"
                  loading={busy === 'reject'}
                  disabled={!!busy}
                  onPress={() => ask('reject')}
                />
              </View>
            ) : null}
            {pending && decide && own ? (
              <Text variant="footnote" tone="muted">
                {t('reclass.self_approval_hint')}
              </Text>
            ) : null}
            {pending && write ? (
              <Button
                title={t('reclass.cancel')}
                variant="ghost"
                loading={busy === 'cancel'}
                disabled={!!busy}
                onPress={() => ask('cancel')}
              />
            ) : null}
          </>
        )}
      </Screen>
    </>
  );
}

type ScannedUnit = { barcode: string; product: NonNullable<ScanResult['product']> | null };

/** New request: scan the unit, pick the product it really is, say why. */
export function ReclassNewScreen() {
  const { t } = useTranslation('warehouse');
  const qc = useQueryClient();
  const [unit, setUnit] = useState<ScannedUnit | null>(null);
  const [manual, setManual] = useState('');
  const [product, setProduct] = useState<Product | null>(null);
  const [reason, setReason] = useState('');
  const [scanning, setScanning] = useState(false);
  const [errors, setErrors] = useState<{ unit?: string; product?: string; reason?: string }>({});
  const [busy, setBusy] = useState(false);

  const onScan = async (code: string): Promise<ScanOutcome> => {
    try {
      const res = await resolveScan(code, { expect: 'unit' });
      if (res.status !== 'ok' || !res.unit) return { ok: false, title: t(`scan_status.${res.status}`, { defaultValue: t('scan.failed') }) };
      setUnit({ barcode: res.unit.barcode, product: res.product });
      setErrors((e) => ({ ...e, unit: undefined }));
      return { ok: true, title: res.unit.barcode, detail: res.product?.name };
    } catch (e) {
      return { ok: false, title: whError(e) };
    }
  };

  const barcode = unit?.barcode ?? manual.trim();

  const submit = async () => {
    const next = {
      unit: barcode ? undefined : t('reclass.validation.unit'),
      product: product ? undefined : t('validation.product'),
      reason: reason.trim() ? undefined : t('reclass.validation.reason'),
    };
    setErrors(next);
    if (next.unit || next.product || next.reason || !product) return;
    setBusy(true);
    try {
      const out = await createReclassification({ barcode, to_product_uuid: product.uuid, reason: reason.trim() });
      qc.setQueryData(reclassKeys.one(out.uuid), out);
      void qc.invalidateQueries({ queryKey: ['warehouse', 'reclass', 'list'] });
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      router.replace(`/depo/reclass/${out.uuid}` as never);
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Stack.Screen options={{ title: t('reclass.new'), headerLargeTitle: false }} />
      <Screen keyboardShouldPersistTaps="handled">
        <Text variant="callout" tone="muted">
          {t('reclass.new_description')}
        </Text>

        <SectionHeader title={t('reclass.unit')} />
        {unit ? (
          <Card>
            <View className="flex-row items-center justify-between gap-2">
              <Text variant="headline" className="font-mono">
                {unit.barcode}
              </Text>
              <Pressable accessibilityRole="button" hitSlop={8} onPress={() => setUnit(null)}>
                <Text variant="callout" tone="primary">
                  {t('reclass.change')}
                </Text>
              </Pressable>
            </View>
            {unit.product ? <Field label={t('reclass.from')} value={`${unit.product.name} · ${unit.product.sku}`} /> : null}
          </Card>
        ) : (
          <View className="gap-2.5">
            <Button title={t('reclass.scan_unit')} icon="scan" variant="secondary" onPress={() => setScanning(true)} />
            <Input
              label={t('reclass.barcode')}
              icon="barcode"
              value={manual}
              onChangeText={setManual}
              placeholder={t('reclass.barcode_placeholder')}
              autoCapitalize="characters"
              autoCorrect={false}
              error={errors.unit}
            />
          </View>
        )}

        <SectionHeader title={t('reclass.to')} />
        {product ? (
          <Card>
            <View className="flex-row items-center justify-between gap-2">
              <View className="flex-1 gap-0.5">
                <Text variant="headline">{product.name}</Text>
                <Text variant="footnote" tone="muted" className="font-mono">
                  {product.sku}
                </Text>
              </View>
              <Pressable accessibilityRole="button" hitSlop={8} onPress={() => setProduct(null)}>
                <Text variant="callout" tone="primary">
                  {t('reclass.change')}
                </Text>
              </Pressable>
            </View>
          </Card>
        ) : (
          <>
            <ProductPicker onPick={(p) => setProduct(p)} />
            {errors.product ? (
              <Text variant="footnote" tone="danger">
                {errors.product}
              </Text>
            ) : null}
          </>
        )}

        <Input
          label={t('reclass.reason')}
          icon="edit"
          value={reason}
          onChangeText={setReason}
          placeholder={t('reclass.reason_placeholder')}
          multiline
          maxLength={500}
          error={errors.reason}
        />
        <Button title={t('reclass.submit')} size="lg" loading={busy} onPress={() => void submit()} />
      </Screen>
      <ContinuousScanner
        visible={scanning}
        single
        onClose={() => setScanning(false)}
        title={t('reclass.scan_unit')}
        onCode={onScan}
      />
    </>
  );
}
