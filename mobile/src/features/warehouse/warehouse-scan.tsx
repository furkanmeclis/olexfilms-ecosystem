import { useQueryClient } from '@tanstack/react-query';
import { CameraView } from 'expo-camera';
import * as Haptics from 'expo-haptics';
import { router, useIsFocused } from 'expo-router';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Alert, Pressable, ScrollView, TextInput, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Flashlight, Keyboard, X } from '@/ui/icons';
import { useCan } from '@/core/spaces';
import { Button } from '@/ui/button';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { classifyCode } from '@/features/scan/scan-codes';

import { countApi, resolveScan, type ScanResult, transferApi, useCounts, useTransfers, wk } from './api';
import { useCurrentWarehouse } from './context';
import { BARCODE_TYPES } from './continuous-scanner';
import { alertError, go, whError } from './format';
import { ScanResultCard } from './scan-result-card';
import { WarehouseChip } from './warehouse-picker';

const REPEAT_MS = 2500;

/** Wall clock for the repeat window (scan callbacks only, never during render). */
const clock = () => Date.now();

type State = { kind: 'idle' } | { kind: 'busy'; code: string } | { kind: 'ok'; result: ScanResult } | { kind: 'error'; code: string; message: string };

/**
 * Warehouse scan tab: the camera keeps running; every new code is resolved
 * (POST /v1/mobile/warehouse/scan) and shown in the card with what can be
 * done next (add to an open transfer or count, open stock or history).
 */
export function WarehouseScan() {
  const { t } = useTranslation('warehouse');
  const insets = useSafeAreaInsets();
  const focused = useIsFocused();
  const qc = useQueryClient();
  const write = useCan('warehouse.write');
  const { warehouse } = useCurrentWarehouse();
  const [state, setState] = useState<State>({ kind: 'idle' });
  const [torch, setTorch] = useState(false);
  const [typing, setTyping] = useState(false);
  const [manual, setManual] = useState('');
  const [acting, setActing] = useState<string | null>(null);
  const lock = useRef(false);
  const last = useRef({ code: '', at: 0 });

  // Open documents the scanned unit can go into (cached lists, refreshed in the background).
  const drafts = useTransfers('draft');
  const counting = useCounts('in_progress');
  const draftList = drafts.data?.pages.flatMap((p) => p.items) ?? [];
  const countList = counting.data?.pages.flatMap((p) => p.items) ?? [];

  const onCode = async (raw: string, force = false) => {
    const code = raw.trim();
    if (!code || lock.current) return;
    const now = clock();
    if (!force && code === last.current.code && now - last.current.at < REPEAT_MS) return;
    last.current = { code, at: now };

    const kind = classifyCode(code);
    if (kind.kind === 'qr_login') {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      router.push({ pathname: '/qr-login/[code]', params: { code: kind.code } });
      return;
    }
    lock.current = true;
    setState({ kind: 'busy', code });
    void Haptics.selectionAsync();
    try {
      const result = await resolveScan(code);
      void Haptics.notificationAsync(
        result.status === 'ok' ? Haptics.NotificationFeedbackType.Success : Haptics.NotificationFeedbackType.Warning,
      );
      setState({ kind: 'ok', result });
    } catch (e) {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
      setState({ kind: 'error', code, message: whError(e) });
    } finally {
      lock.current = false;
    }
  };

  const addToTransfer = (barcode: string) => {
    const pick = async (uuid: string, no: string) => {
      setActing('transfer');
      try {
        await transferApi.addLines(uuid, { barcodes: [barcode] });
        void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
        void qc.invalidateQueries({ queryKey: wk.transfer(uuid) });
        void qc.invalidateQueries({ queryKey: ['warehouse', 'transfers'] });
        Alert.alert(t('m.added_to', { doc: no }));
      } catch (e) {
        alertError(e);
      } finally {
        setActing(null);
      }
    };
    if (draftList.length === 1) return void pick(draftList[0].uuid, draftList[0].transfer_no);
    Alert.alert(t('m.pick_transfer'), undefined, [
      ...draftList.slice(0, 4).map((d) => ({
        text: `${d.transfer_no} · ${d.from_warehouse.code} → ${d.to_warehouse.code}`,
        onPress: () => void pick(d.uuid, d.transfer_no),
      })),
      { text: t('common:cancel'), style: 'cancel' as const },
    ]);
  };

  const addToCount = (code: string) => {
    const pick = async (uuid: string, label: string) => {
      setActing('count');
      try {
        await countApi.scan(uuid, { code });
        void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
        void qc.invalidateQueries({ queryKey: wk.count(uuid) });
        Alert.alert(t('m.added_to', { doc: label }));
      } catch (e) {
        alertError(e);
      } finally {
        setActing(null);
      }
    };
    const label = (c: (typeof countList)[number]) => `${c.warehouse.code} · ${t(`count_method.${c.method}`)}`;
    if (countList.length === 1) return void pick(countList[0].uuid, label(countList[0]));
    Alert.alert(t('m.pick_count'), undefined, [
      ...countList.slice(0, 4).map((c) => ({ text: label(c), onPress: () => void pick(c.uuid, label(c)) })),
      { text: t('common:cancel'), style: 'cancel' as const },
    ]);
  };

  const result = state.kind === 'ok' ? state.result : null;
  const ok = result?.status === 'ok';

  return (
    <View className="flex-1 bg-black">
      {focused ? (
        <CameraView
          style={{ flex: 1 }}
          facing="back"
          enableTorch={torch}
          barcodeScannerSettings={{ barcodeTypes: [...BARCODE_TYPES] }}
          onBarcodeScanned={typing ? undefined : ({ data }) => void onCode(data)}
        />
      ) : null}

      <View className="absolute inset-x-0 top-0 flex-row items-center gap-3 px-4" style={{ paddingTop: insets.top + 8 }}>
        <View className="flex-1 flex-row">
          <WarehouseChip />
        </View>
        <OverlayButton label={t('m.torch')} active={torch} onPress={() => setTorch((v) => !v)} icon={Flashlight} />
        <OverlayButton label={t('m.type_code')} active={typing} onPress={() => setTyping((v) => !v)} icon={Keyboard} />
      </View>

      {state.kind === 'idle' && !typing ? (
        <View pointerEvents="none" className="absolute inset-0 items-center justify-center">
          <View className="h-44 w-72 rounded-2xl border-[3px] border-white/90" />
          <Text variant="callout" className="mt-5 px-8 text-center text-white/90">
            {t('scan.description')}
          </Text>
        </View>
      ) : null}

      <View className="absolute inset-x-3 gap-2" style={{ bottom: insets.bottom + 90 }}>
        {typing ? (
          <View className="flex-row items-center gap-2 rounded-lg bg-card p-2">
            <TextInput
              autoFocus
              value={manual}
              onChangeText={setManual}
              autoCapitalize="characters"
              autoCorrect={false}
              returnKeyType="search"
              placeholder={t('scan.placeholder')}
              placeholderTextColorClassName="accent-muted-foreground"
              onSubmitEditing={() => {
                void onCode(manual, true);
                setManual('');
              }}
              className="min-h-11 flex-1 px-2 font-mono text-[17px] text-foreground"
            />
            <Button
              title={t('scan.submit')}
              onPress={() => {
                void onCode(manual, true);
                setManual('');
              }}
            />
          </View>
        ) : null}

        {state.kind !== 'idle' ? (
          <View className="max-h-[420px] rounded-sheet bg-card">
            <ScrollView contentContainerClassName="gap-3 p-4">
              <View className="flex-row items-start justify-between gap-2">
                <View className="flex-1">
                  {state.kind === 'busy' ? (
                    <View className="flex-row items-center gap-2">
                      <ActivityIndicator />
                      <Text variant="callout" className="font-mono">
                        {state.code}
                      </Text>
                    </View>
                  ) : state.kind === 'error' ? (
                    <View className="gap-1">
                      <StatusPill tone="problem" label={state.message} />
                      <Text variant="title" selectable className="font-mono">
                        {state.code}
                      </Text>
                    </View>
                  ) : result ? (
                    <ScanResultCard result={result} />
                  ) : null}
                </View>
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('common:close')}
                  hitSlop={8}
                  onPress={() => {
                    setState({ kind: 'idle' });
                    last.current = { code: '', at: 0 };
                  }}
                  className="size-8 items-center justify-center rounded-full bg-secondary">
                  <Icon as={X} size={16} />
                </Pressable>
              </View>

              {ok && result ? (
                <View className="flex-row flex-wrap gap-2">
                  {result.type === 'unit' && result.unit ? (
                    <>
                      {write && draftList.length > 0 && result.unit.unit_kind === 'serial' ? (
                        <ActionButton
                          title={t('m.add_to_transfer')}
                          loading={acting === 'transfer'}
                          onPress={() => addToTransfer(result.unit!.barcode)}
                        />
                      ) : null}
                      {write && countList.length > 0 ? (
                        <ActionButton
                          title={t('m.add_to_count')}
                          loading={acting === 'count'}
                          onPress={() => addToCount(result.unit!.barcode)}
                        />
                      ) : null}
                      <ActionButton
                        title={t('m.unit_history')}
                        variant="outline"
                        onPress={() => go(`/depo/stock/unit/${encodeURIComponent(result.unit!.barcode)}`)}
                      />
                    </>
                  ) : null}
                  {result.type === 'location' && result.location ? (
                    <>
                      <ActionButton
                        title={t('m.location_stock')}
                        onPress={() =>
                          go(
                            `/depo/stock/location/${result.location!.uuid}?name=${encodeURIComponent(result.location!.full_code)}`,
                          )
                        }
                      />
                      {write && countList.length > 0 ? (
                        <ActionButton
                          title={t('m.count_here')}
                          variant="outline"
                          loading={acting === 'count'}
                          onPress={() => addToCount(result.code)}
                        />
                      ) : null}
                    </>
                  ) : null}
                  {result.product && result.type !== 'location' ? (
                    <ActionButton
                      title={t('m.product_stock')}
                      variant="outline"
                      onPress={() =>
                        go(`/depo/stock/${result.product!.uuid}?name=${encodeURIComponent(result.product!.name)}`)
                      }
                    />
                  ) : null}
                </View>
              ) : null}
              {warehouse && result?.type === 'location' && result.location && !result.location.path.some((p) => p.uuid === warehouse.uuid) ? (
                <Text variant="footnote" tone="muted">
                  {t('m.other_warehouse')}
                </Text>
              ) : null}
            </ScrollView>
          </View>
        ) : null}
      </View>
    </View>
  );
}

function ActionButton(props: { title: string; onPress: () => void; loading?: boolean; variant?: 'primary' | 'outline' }) {
  return <Button {...props} variant={props.variant ?? 'secondary'} className="grow" />;
}

function OverlayButton({
  label,
  active,
  onPress,
  icon: Glyph,
}: {
  label: string;
  active: boolean;
  onPress: () => void;
  icon: typeof X;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={{ selected: active }}
      hitSlop={6}
      onPress={onPress}
      className={cn('size-10 items-center justify-center rounded-full', active ? 'bg-white' : 'bg-black/50')}>
      {active ? <Icon as={Glyph} tone="primary" size={20} /> : <Glyph size={20} color="#ffffff" strokeWidth={2} />}
    </Pressable>
  );
}
