import { CameraView, useCameraPermissions } from 'expo-camera';
import * as Haptics from 'expo-haptics';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, KeyboardAvoidingView, Linking, Modal, Platform, Pressable, TextInput, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { ScopedTheme } from 'uniwind';

import { localCount } from '@/core/digits';
import { useScannerInput } from '@/features/scanner/input-mode';
import { createScanQueue, type ScanQueue, ScanTimeoutError } from '@/features/scanner/scan-queue';
import { WedgeInput } from '@/features/scanner/wedge-input';
import { cn } from '@/ui/cn';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';
import type { IconName } from '@/ui/icons';
import { ToastHost } from '@/ui/toast';

export const BARCODE_TYPES = ['qr', 'datamatrix', 'code128', 'code39', 'ean13', 'ean8', 'upc_a', 'upc_e', 'itf14'] as const;

export type ScanOutcome = { ok: boolean; title: string; detail?: string };

type FeedItem = ScanOutcome & { id: number; code: string };

/**
 * Full-screen camera that keeps scanning: each code is handed to `onCode`,
 * its outcome is shown in a short feed with haptics, and the camera stays
 * open for the next code. A keyboard entry covers damaged labels.
 *
 * TEC-1255: codes that arrive while one is being handled wait in a FIFO
 * queue (the count is shown) instead of being dropped; a code that gets no
 * answer within 10 s is reported as timed out and the queue moves on.
 */
export function ContinuousScanner({
  visible,
  onClose,
  title,
  hint,
  onCode,
  footer,
  single,
  complete,
  overlay,
}: {
  visible: boolean;
  onClose: () => void;
  title: string;
  hint?: string;
  onCode: (code: string) => Promise<ScanOutcome>;
  /** Extra controls under the feed (quantity, context). */
  footer?: React.ReactNode;
  /** Close after the first successful code. */
  single?: boolean;
  /** The job is done (e.g. every order line is covered): a success row with a close button. */
  complete?: { title: string; action: string };
  /**
   * TEC-1241: a confirmation over the feed (e.g. the cut of a closed box).
   * Scanning pauses while it is shown; it takes the reader's Enter.
   */
  overlay?: React.ReactNode;
}) {
  const { t, i18n } = useTranslation('warehouse');
  const insets = useSafeAreaInsets();
  const [permission, requestPermission] = useCameraPermissions();
  // TEC-1230: a hardware reader (keyboard wedge) instead of the camera.
  const wedge = useScannerInput((s) => s.mode) === 'wedge';
  const [feed, setFeed] = useState<FeedItem[]>([]);
  const [queue, setQueue] = useState<{ active: string | null; pending: number }>({ active: null, pending: 0 });
  const [torch, setTorch] = useState(false);
  const [typing, setTyping] = useState(false);
  const [manual, setManual] = useState('');
  const seq = useRef(0);
  // The latest props for the queue's callbacks (the queue lives across renders).
  const latest = useRef({ onCode, single, t, close: () => {} });

  // One queue per scanner; its callbacks read the latest props through `latest`.
  // Created on the first code (never during render).
  const queueRef = useRef<ScanQueue | null>(null);
  const getQueue = () =>
    (queueRef.current ??= createScanQueue<ScanOutcome>({
      run: (code) => latest.current.onCode(code),
      onStart: () => void Haptics.selectionAsync(),
      onChange: setQueue,
      onSettled: (code, r) => {
        const outcome: ScanOutcome = r.ok
          ? r.value
          : { ok: false, title: latest.current.t(r.error instanceof ScanTimeoutError ? 'scan.timeout' : 'scan.failed') };
        void Haptics.notificationAsync(
          outcome.ok ? Haptics.NotificationFeedbackType.Success : Haptics.NotificationFeedbackType.Error,
        );
        seq.current += 1;
        const id = seq.current;
        setFeed((f) => [{ ...outcome, code, id }, ...f].slice(0, 4));
        if (outcome.ok && latest.current.single) latest.current.close();
      },
    }));

  // TEC-1241: a confirmation over the feed waits for the user, not the network.
  useEffect(() => {
    if (overlay) queueRef.current?.holdTimeout();
  }, [overlay]);
  // Codes left in the queue never outlive the scanner.
  useEffect(() => {
    if (!visible) queueRef.current?.clear();
  }, [visible]);
  useEffect(() => () => queueRef.current?.clear(), []);

  // A reader trigger is deliberate: only the camera sees a code twice.
  const handle = (raw: string, repeat = !wedge) => {
    getQueue().push(raw, { repeat });
  };

  const close = () => {
    queueRef.current?.clear();
    setFeed([]);
    setTorch(false);
    setTyping(false);
    setManual('');
    onClose();
  };
  useEffect(() => {
    latest.current = { onCode, single, t, close };
  });

  return (
    <Modal visible={visible} animationType="slide" presentationStyle="fullScreen" onRequestClose={close}>
      {/* Always dark, like a camera: the overlay (and callers' footers) use white text. */}
      <ScopedTheme theme="dark">
        <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="flex-1 bg-black">
        {wedge ? (
          <View className="flex-1 items-center justify-center gap-3 bg-black px-8">
            <Icon name="barcode" tone="on-media" size={56} />
            <Text variant="headline" className="text-center text-white">
              {t('scan:input.ready')}
            </Text>
            <Text variant="footnote" className="text-center text-white/70">
              {t('scan:input.ready_hint')}
            </Text>
            {visible ? <WedgeInput paused={typing || !!overlay} onCode={(c) => handle(c)} /> : null}
          </View>
        ) : !permission ? null : !permission.granted ? (
          <View className="flex-1 justify-center bg-black">
            <EmptyState
              icon="camera"
              title={t('scan:permission_title')}
              body={t('scan:permission_body')}
              action={
                permission.canAskAgain
                  ? { title: t('scan:permission_button'), onPress: () => void requestPermission() }
                  : { title: t('scan:open_settings'), onPress: () => void Linking.openSettings() }
              }
            />
          </View>
        ) : visible ? (
          <CameraView
            style={{ flex: 1 }}
            facing="back"
            enableTorch={torch}
            barcodeScannerSettings={{ barcodeTypes: [...BARCODE_TYPES] }}
            onBarcodeScanned={typing || overlay ? undefined : ({ data }) => handle(data)}
          />
        ) : null}

        {/* Top bar */}
        <View className="absolute inset-x-0 top-0 flex-row items-center gap-3 px-4" style={{ paddingTop: insets.top + 8 }}>
          <RoundButton label={t('common:close')} onPress={close} icon="close" />
          <View className="flex-1">
            <Text variant="headline" className="text-white" numberOfLines={1}>
              {title}
            </Text>
            {hint ? (
              <Text variant="footnote" className="text-white/80" numberOfLines={2}>
                {hint}
              </Text>
            ) : null}
          </View>
          <RoundButton
            label={t(wedge ? 'scan:input.camera' : 'scan:input.wedge')}
            onPress={() => useScannerInput.getState().setMode(wedge ? 'camera' : 'wedge')}
            icon={wedge ? 'camera' : 'barcode'}
          />
          {wedge ? null : (
            <RoundButton label={t('m.torch')} onPress={() => setTorch((v) => !v)} icon="flashlight" active={torch} />
          )}
          <RoundButton label={t('m.type_code')} onPress={() => setTyping((v) => !v)} icon="keyboard" active={typing} />
        </View>

        {!wedge && !typing && permission?.granted ? (
          <View pointerEvents="none" className="absolute inset-0 items-center justify-center">
            <View className="h-44 w-72 rounded-2xl border-[3px] border-white/90" />
          </View>
        ) : null}

        {/* Bottom: manual entry, feed, footer */}
        <View className="absolute inset-x-3 gap-2" style={{ bottom: insets.bottom + 12 }}>
          {typing ? (
            <View className="flex-row items-center gap-2 rounded-lg bg-card p-2">
              <TextInput
                autoFocus
                value={manual}
                onChangeText={setManual}
                autoCapitalize="characters"
                autoCorrect={false}
                returnKeyType="send"
                placeholder={t('scan.placeholder')}
                placeholderTextColorClassName="accent-muted-foreground"
                onSubmitEditing={() => {
                  const v = manual;
                  setManual('');
                  handle(v, false);
                }}
                className="min-h-11 flex-1 px-2 font-mono text-[17px] text-foreground"
              />
              <Pressable
                accessibilityRole="button"
                accessibilityLabel={t('scan.submit')}
                onPress={() => {
                  const v = manual;
                  setManual('');
                  handle(v, false);
                }}
                className="min-h-11 justify-center rounded-md bg-primary px-4">
                <Text variant="callout" tone="onPrimary" className="font-semibold">
                  {t('scan.submit')}
                </Text>
              </Pressable>
            </View>
          ) : null}
          {queue.active !== null ? (
            <View className="flex-row items-center gap-2 self-center rounded-full bg-card/90 px-3 py-1.5">
              <ActivityIndicator size="small" />
              <Text variant="footnote">{t('m.checking')}</Text>
              {queue.pending > 0 ? (
                <Text variant="footnote" className="font-semibold">
                  {`· ${t('m.queued', { n: localCount(queue.pending, i18n.language) })}`}
                </Text>
              ) : null}
            </View>
          ) : null}
          {complete ? (
            <View className="flex-row items-center gap-3 rounded-lg bg-card p-3">
              <View className="size-7 items-center justify-center rounded-full bg-success/20">
                <Icon name="check" tone="primary" size={16} weight="semibold" />
              </View>
              <Text variant="callout" className="flex-1 font-semibold">
                {complete.title}
              </Text>
              <Pressable accessibilityRole="button" onPress={close} className="min-h-11 justify-center rounded-md bg-primary px-4">
                <Text variant="callout" tone="onPrimary" className="font-semibold">
                  {complete.action}
                </Text>
              </Pressable>
            </View>
          ) : null}
          {feed.map((f, i) => (
            <View
              key={f.id}
              className={cn('flex-row items-start gap-3 rounded-lg bg-card p-3', i > 0 && 'opacity-70')}>
              <View className={cn('mt-0.5 size-6 items-center justify-center rounded-full', f.ok ? 'bg-success/20' : 'bg-destructive/20')}>
                <Icon name={f.ok ? 'check' : 'error'} tone={f.ok ? 'primary' : 'destructive'} size={14} />
              </View>
              <View className="flex-1 gap-0.5">
                <Text variant="callout" className="font-semibold" numberOfLines={2}>
                  {f.title}
                </Text>
                <Text variant="footnote" tone="muted" numberOfLines={2}>
                  {f.detail ? `${f.detail} · ` : ''}
                  <Text variant="footnote" tone="muted" className="font-mono">
                    {f.code}
                  </Text>
                </Text>
              </View>
            </View>
          ))}
          {footer}
        </View>
        {overlay}
        </KeyboardAvoidingView>
      </ScopedTheme>
      <ToastHost inModal />
    </Modal>
  );
}

function RoundButton({
  label,
  onPress,
  icon,
  active,
}: {
  label: string;
  onPress: () => void;
  icon: IconName;
  active?: boolean;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={{ selected: !!active }}
      hitSlop={6}
      onPress={onPress}
      className={cn('size-10 items-center justify-center rounded-full', active ? 'bg-white' : 'bg-black/50')}>
      <Icon name={icon} tone={active ? 'primary' : 'on-media'} size={20} />
    </Pressable>
  );
}
