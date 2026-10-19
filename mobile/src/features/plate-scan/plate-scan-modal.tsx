import { CameraView, useCameraPermissions } from 'expo-camera';
import { File } from 'expo-file-system';
import * as Haptics from 'expo-haptics';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, KeyboardAvoidingView, Linking, Modal, Platform, Pressable, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { extractPlateCandidates, isValidTrPlate, normalizePlateInput, type PlateCandidate, PlateConsensus, previewRectToImage } from './lib';
import { getPlateRecognizer } from './recognizer';

/** Guide frame: TR plate is 520 × 110 mm. */
const GUIDE_ASPECT = 520 / 110;
const GUIDE_WIDTH = 0.84;
/** Pause between automatic frames. */
const FRAME_GAP_MS = 250;

type Size = { width: number; height: number };

function guideRect(view: Size) {
  const width = view.width * GUIDE_WIDTH;
  const height = width / GUIDE_ASPECT;
  return { x: (view.width - width) / 2, y: view.height * 0.38 - height / 2, width, height };
}

function confidenceTone(c: number): StatusTone {
  if (c >= 0.8) return 'done';
  if (c >= 0.5) return 'progress';
  return 'problem';
}

function discard(uri: string) {
  try {
    new File(uri).delete();
  } catch {
    // Temporary camera file; the OS cleans the cache anyway.
  }
}

/**
 * Full-screen plate scanner. Reads frames continuously while the plate sits
 * in the guide; two agreeing frames fill the result, or the shutter takes one
 * frame on demand. The result can always be edited before it is used.
 * A Modal (not a route) so it opens from inside page sheets as well.
 */
export function PlateScanModal({ visible, onClose, onResult }: { visible: boolean; onClose: () => void; onResult: (plate: string) => void }) {
  return (
    <Modal visible={visible} animationType="slide" presentationStyle="fullScreen" onRequestClose={onClose}>
      {visible ? <Scanner onClose={onClose} onResult={onResult} /> : null}
    </Modal>
  );
}

function Scanner({ onClose, onResult }: { onClose: () => void; onResult: (plate: string) => void }) {
  const { t } = useTranslation('plate');
  const insets = useSafeAreaInsets();
  const [permission, requestPermission] = useCameraPermissions();
  const camera = useRef<CameraView>(null);
  const consensus = useRef(new PlateConsensus());
  const [view, setView] = useState<Size | null>(null);
  const [ready, setReady] = useState(false);
  const [torch, setTorch] = useState(false);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const [result, setResult] = useState<{ candidates: PlateCandidate[]; value: string } | null>(null);
  const capturing = useRef(false);

  /** One frame: photo → OCR → candidates. Null when the camera is busy. */
  const readFrame = async (): Promise<PlateCandidate[] | null> => {
    if (!camera.current || !view || capturing.current) return null;
    capturing.current = true;
    let uri: string | null = null;
    try {
      const photo = await camera.current.takePictureAsync({ quality: 0.6, shutterSound: false });
      uri = photo.uri;
      const ocr = await getPlateRecognizer().recognize(photo.uri);
      const roi = previewRectToImage(view, { width: ocr.width || photo.width, height: ocr.height || photo.height }, guideRect(view));
      // A little slack around the guide: users rarely centre the plate exactly.
      const padded = { x: roi.x - roi.width * 0.08, y: roi.y - roi.height * 0.6, width: roi.width * 1.16, height: roi.height * 2.2 };
      return extractPlateCandidates(ocr.lines, { roi: padded });
    } finally {
      if (uri) discard(uri);
      capturing.current = false;
    }
  };

  const show = (candidates: PlateCandidate[]) => {
    const top = candidates[0];
    setResult({ candidates, value: top ? top.plate : '' });
    if (top) void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
  };

  // Automatic loop while the camera is on screen and nothing is shown yet.
  useEffect(() => {
    if (!ready || !view || result) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const tick = async () => {
      if (cancelled) return;
      try {
        const candidates = await readFrame();
        if (cancelled) return;
        setFailed(false);
        const accepted = candidates ? consensus.current.add(candidates) : null;
        if (accepted) {
          show([accepted, ...consensus.current.ranked().filter((c) => c.compact !== accepted.compact)]);
          return;
        }
      } catch {
        if (!cancelled) setFailed(true);
      }
      if (!cancelled) timer = setTimeout(() => void tick(), FRAME_GAP_MS);
    };
    timer = setTimeout(() => void tick(), 400);
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
    // readFrame/show read refs and the latest view; restart only on these.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, view, result]);

  const shutter = async () => {
    setBusy(true);
    try {
      // Wait for an in-flight automatic frame instead of failing the tap.
      for (let i = 0; i < 20 && capturing.current; i++) await new Promise((r) => setTimeout(r, 100));
      const frame = (await readFrame()) ?? [];
      const merged = new Map<string, PlateCandidate>();
      for (const c of [...frame, ...consensus.current.ranked()]) if (!merged.has(c.compact)) merged.set(c.compact, c);
      show([...merged.values()]);
    } catch {
      show(consensus.current.ranked());
    } finally {
      setBusy(false);
    }
  };

  const rescan = () => {
    consensus.current.reset();
    setResult(null);
  };

  const use = () => {
    if (!result) return;
    const value = normalizePlateInput(result.value);
    if (!value) return;
    onResult(value);
    onClose();
  };

  if (!permission) return <View className="flex-1 bg-black" />;

  if (!permission.granted) {
    return (
      <View className="flex-1 justify-center bg-background" style={{ paddingTop: insets.top }}>
        <EmptyState
          icon="camera"
          title={t('permission_title')}
          body={t('permission_body')}
          action={
            permission.canAskAgain
              ? { title: t('permission_button'), onPress: () => void requestPermission() }
              : { title: t('open_settings'), onPress: () => void Linking.openSettings() }
          }
        />
        <Button title={t('close')} variant="ghost" onPress={onClose} />
      </View>
    );
  }

  const guide = view ? guideRect(view) : null;
  const top = result?.candidates[0];
  const edited = result ? normalizePlateInput(result.value) : '';

  return (
    <View className="flex-1 bg-black">
      <View className="flex-1" onLayout={(e) => setView({ width: e.nativeEvent.layout.width, height: e.nativeEvent.layout.height })}>
        <CameraView
          ref={camera}
          style={{ flex: 1 }}
          facing="back"
          enableTorch={torch}
          animateShutter={false}
          onCameraReady={() => setReady(true)}
        />
        {guide ? (
          <View pointerEvents="none" className="absolute inset-0">
            <View className="absolute inset-x-0 top-0 bg-black/50" style={{ height: guide.y }} />
            <View className="absolute inset-x-0 bottom-0 bg-black/50" style={{ top: guide.y + guide.height }} />
            <View className="absolute left-0 bg-black/50" style={{ top: guide.y, height: guide.height, width: guide.x }} />
            <View className="absolute right-0 bg-black/50" style={{ top: guide.y, height: guide.height, width: guide.x }} />
            <View
              className="absolute rounded-xl border-[3px] border-white/90"
              style={{ left: guide.x, top: guide.y, width: guide.width, height: guide.height }}
            />
            <Text variant="callout" className="absolute inset-x-6 text-center text-white" style={{ top: guide.y + guide.height + 16 }}>
              {failed ? t('read_failed') : t('hint')}
            </Text>
          </View>
        ) : null}
      </View>

      <View className="absolute inset-x-0 flex-row items-center justify-between px-4" style={{ top: insets.top + 8 }}>
        <RoundButton icon="close" label={t('close')} onPress={onClose} />
        <Text variant="headline" className="text-white">
          {t('title')}
        </Text>
        <RoundButton icon="flashlight" label={t(torch ? 'torch_off' : 'torch_on')} active={torch} onPress={() => setTorch((v) => !v)} />
      </View>

      {!result ? (
        <View className="absolute inset-x-0 items-center gap-4" style={{ bottom: insets.bottom + 24 }}>
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t('capture')}
            disabled={busy || !ready}
            onPress={() => void shutter()}
            className="size-[72px] items-center justify-center rounded-full border-4 border-white/90 active:opacity-70"
          >
            {busy ? <ActivityIndicator color="#fff" /> : <View className="size-14 rounded-full bg-white" />}
          </Pressable>
          <Pressable accessibilityRole="button" hitSlop={8} onPress={onClose}>
            <Text variant="callout" className="text-white/90">
              {t('type_instead')}
            </Text>
          </Pressable>
        </View>
      ) : (
        <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="absolute inset-x-0 bottom-0">
          <View className="gap-4 rounded-t-sheet bg-card p-4" style={{ paddingBottom: insets.bottom + 16 }}>
            <View className="flex-row items-center justify-between">
              <Text variant="headline">{top ? t('result_title') : t('not_found_title')}</Text>
              {top ? <StatusPill tone={confidenceTone(top.confidence)} label={t('confidence', { value: Math.round(top.confidence * 100) })} /> : null}
            </View>
            {!top ? (
              <Text variant="footnote" tone="muted">
                {t('not_found_body')}
              </Text>
            ) : null}
            <Input
              label={t('plate')}
              placeholder={t('placeholder')}
              value={result.value}
              onChangeText={(value) => setResult({ ...result, value })}
              autoCapitalize="characters"
              autoCorrect={false}
              className="font-mono text-[22px]"
              hint={edited && !isValidTrPlate(edited) ? t('not_tr_format') : undefined}
            />
            {result.candidates.length > 1 ? (
              <View className="flex-row flex-wrap gap-2">
                {result.candidates.slice(1, 4).map((c) => (
                  <Pressable
                    key={c.compact}
                    accessibilityRole="button"
                    onPress={() => setResult({ ...result, value: c.plate })}
                    className="rounded-full border border-border px-3 py-1.5 active:bg-muted"
                  >
                    <Text variant="footnote" className="font-mono">
                      {c.plate}
                    </Text>
                  </Pressable>
                ))}
              </View>
            ) : null}
            <View className="flex-row gap-3">
              <Button title={t('rescan')} variant="secondary" size="lg" className="flex-1" onPress={rescan} />
              <Button title={t('use')} size="lg" className="flex-1" disabled={!edited} onPress={use} />
            </View>
          </View>
        </KeyboardAvoidingView>
      )}
    </View>
  );
}

function RoundButton({ icon, label, active, onPress }: { icon: 'close' | 'flashlight'; label: string; active?: boolean; onPress: () => void }) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      hitSlop={8}
      onPress={onPress}
      className={active ? 'size-11 items-center justify-center rounded-full bg-primary' : 'size-11 items-center justify-center rounded-full bg-black/50'}
    >
      <Icon name={icon} tone={active ? 'primary-foreground' : 'on-media'} size={20} />
    </Pressable>
  );
}
