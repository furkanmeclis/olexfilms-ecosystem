import { CameraView, useCameraPermissions } from 'expo-camera';
import * as Haptics from 'expo-haptics';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, KeyboardAvoidingView, Linking, Modal, Platform, Pressable, TextInput, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { ScopedTheme } from 'uniwind';

import { Camera, Check, CircleAlert, Flashlight, Keyboard, X } from '@/ui/icons';
import { cn } from '@/ui/cn';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

export const BARCODE_TYPES = ['qr', 'datamatrix', 'code128', 'code39', 'ean13', 'ean8', 'upc_a', 'upc_e', 'itf14'] as const;

/** Same code again within this window is the camera seeing it twice, not a second scan. */
const REPEAT_MS = 2500;

/** Wall clock for the repeat window (scan callbacks only, never during render). */
const clock = () => Date.now();

export type ScanOutcome = { ok: boolean; title: string; detail?: string };

type FeedItem = ScanOutcome & { id: number; code: string };

/**
 * Full-screen camera that keeps scanning: each code is handed to `onCode`,
 * its outcome is shown in a short feed with haptics, and the camera stays
 * open for the next code. A keyboard entry covers damaged labels.
 */
export function ContinuousScanner({
  visible,
  onClose,
  title,
  hint,
  onCode,
  footer,
  single,
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
}) {
  const { t } = useTranslation('warehouse');
  const insets = useSafeAreaInsets();
  const [permission, requestPermission] = useCameraPermissions();
  const [feed, setFeed] = useState<FeedItem[]>([]);
  const [busy, setBusy] = useState(false);
  const [torch, setTorch] = useState(false);
  const [typing, setTyping] = useState(false);
  const [manual, setManual] = useState('');
  const lock = useRef(false);
  const last = useRef<{ code: string; at: number }>({ code: '', at: 0 });
  const seq = useRef(0);

  const handle = async (raw: string) => {
    const code = raw.trim();
    if (!code || lock.current) return;
    const now = clock();
    if (code === last.current.code && now - last.current.at < REPEAT_MS) return;
    last.current = { code, at: now };
    lock.current = true;
    setBusy(true);
    void Haptics.selectionAsync();
    let outcome: ScanOutcome;
    try {
      outcome = await onCode(code);
    } catch {
      outcome = { ok: false, title: t('scan.failed') };
    }
    void Haptics.notificationAsync(
      outcome.ok ? Haptics.NotificationFeedbackType.Success : Haptics.NotificationFeedbackType.Error,
    );
    seq.current += 1;
    setFeed((f) => [{ ...outcome, code, id: seq.current }, ...f].slice(0, 4));
    setBusy(false);
    lock.current = false;
    if (outcome.ok && single) close();
  };

  const close = () => {
    setFeed([]);
    setTorch(false);
    setTyping(false);
    setManual('');
    last.current = { code: '', at: 0 };
    onClose();
  };

  return (
    <Modal visible={visible} animationType="slide" presentationStyle="fullScreen" onRequestClose={close}>
      {/* Always dark, like a camera: the overlay (and callers' footers) use white text. */}
      <ScopedTheme theme="dark">
        <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="flex-1 bg-black">
        {!permission ? null : !permission.granted ? (
          <View className="flex-1 justify-center bg-black">
            <EmptyState
              icon={Camera}
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
            onBarcodeScanned={typing ? undefined : ({ data }) => void handle(data)}
          />
        ) : null}

        {/* Top bar */}
        <View className="absolute inset-x-0 top-0 flex-row items-center gap-3 px-4" style={{ paddingTop: insets.top + 8 }}>
          <RoundButton label={t('common:close')} onPress={close} icon={X} />
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
          <RoundButton label={t('m.torch')} onPress={() => setTorch((v) => !v)} icon={Flashlight} active={torch} />
          <RoundButton label={t('m.type_code')} onPress={() => setTyping((v) => !v)} icon={Keyboard} active={typing} />
        </View>

        {!typing && permission?.granted ? (
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
                  last.current = { code: '', at: 0 };
                  void handle(v);
                }}
                className="min-h-11 flex-1 px-2 font-mono text-[17px] text-foreground"
              />
              <Pressable
                accessibilityRole="button"
                accessibilityLabel={t('scan.submit')}
                onPress={() => {
                  const v = manual;
                  setManual('');
                  last.current = { code: '', at: 0 };
                  void handle(v);
                }}
                className="min-h-11 justify-center rounded-md bg-primary px-4">
                <Text variant="callout" tone="onPrimary" className="font-semibold">
                  {t('scan.submit')}
                </Text>
              </Pressable>
            </View>
          ) : null}
          {busy ? (
            <View className="flex-row items-center gap-2 self-center rounded-full bg-card/90 px-3 py-1.5">
              <ActivityIndicator size="small" />
              <Text variant="footnote">{t('m.checking')}</Text>
            </View>
          ) : null}
          {feed.map((f, i) => (
            <View
              key={f.id}
              className={cn('flex-row items-start gap-3 rounded-lg bg-card p-3', i > 0 && 'opacity-70')}>
              <View className={cn('mt-0.5 size-6 items-center justify-center rounded-full', f.ok ? 'bg-success/20' : 'bg-destructive/20')}>
                <Icon as={f.ok ? Check : CircleAlert} tone={f.ok ? 'primary' : 'destructive'} size={14} />
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
        </KeyboardAvoidingView>
      </ScopedTheme>
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
  icon: typeof X;
  active?: boolean;
}) {
  const Glyph = icon;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={{ selected: !!active }}
      hitSlop={6}
      onPress={onPress}
      className={cn('size-10 items-center justify-center rounded-full', active ? 'bg-white' : 'bg-black/50')}>
      {active ? (
        <Icon as={icon} tone="primary" size={20} />
      ) : (
        // Over the camera picture the glyph is always white, like the frame.
        <Glyph size={20} color="#ffffff" strokeWidth={2} />
      )}
    </Pressable>
  );
}
