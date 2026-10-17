import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CameraView, useCameraPermissions } from 'expo-camera';
import * as Haptics from 'expo-haptics';
import { Image } from 'expo-image';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Alert, Linking, Modal, Pressable, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { api, PANEL_MIRROR_PREFIX, unwrap, uploadForm } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import type { components } from '@/core/api/schema';
import { getTokens } from '@/core/auth/tokens';
import { API_URL, MOBILE_API_VERSION } from '@/core/config';
import { Button } from '@/ui/button';
import { Camera, Check, ImagePlus, X } from '@/ui/icons';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { errorText, Section } from './ui';

type Angle = components['schemas']['IntakePhotoAngle'];

export const intakeKey = (uuid: string) => ['services', 'intake-photos', uuid] as const;

/** Authenticated image source for an API file path (`/v1/...`), through the mobile mirror. */
export function apiImage(path: string) {
  const rel = path.startsWith('/v1/') ? PANEL_MIRROR_PREFIX + path.slice('/v1/'.length) : path;
  const t = getTokens();
  return {
    uri: `${API_URL}${rel}`,
    headers: { 'X-Mobile-Api-Version': MOBILE_API_VERSION, ...(t ? { Authorization: `Bearer ${t.access}` } : {}) },
  };
}

function label(map: Record<string, string> | undefined, lang: string, fallback: string) {
  if (!map) return fallback;
  return map[lang] ?? map[lang.split('-')[0]] ?? map.en ?? map.tr ?? Object.values(map)[0] ?? fallback;
}

/**
 * Intake photos of the photo standard (TEC-498/499): one photo per required
 * angle before the service leaves draft. Hidden when the organization does
 * not have the photo_standard module (the API answers FEATURE_DISABLED).
 */
export function IntakePhotos({ uuid, canCapture }: { uuid: string; canCapture: boolean }) {
  const { t, i18n } = useTranslation('dealer');
  const [angle, setAngle] = useState<Angle | null>(null);
  const q = useQuery({
    queryKey: intakeKey(uuid),
    queryFn: () => unwrap(api.GET('/v1/services/{uuid}/intake-photos', { params: { path: { uuid } } })),
    retry: false,
  });

  if (q.error && isApiError(q.error) && (q.error.code === 'FEATURE_DISABLED' || q.error.status === 403)) return null;
  if (!q.data) {
    if (q.isPending) return null;
    return (
      <Section title={t('photos')}>
        <Pressable accessibilityRole="button" onPress={() => void q.refetch()} className="px-4 py-3">
          <Text variant="callout" tone="danger">
            {errorText(q.error)}
          </Text>
        </Pressable>
      </Section>
    );
  }

  const angles = q.data.angles.filter((a) => !a.angle.hidden);
  if (angles.length === 0) return null;
  const required = angles.filter((a) => a.required);
  const done = required.filter((a) => !!a.photo).length;

  return (
    <>
      <Section
        title={t('photos')}
        right={
          <Text variant="footnote" tone={done === required.length ? 'primary' : 'muted'} className="font-semibold">
            {t('photos_progress', { done, total: required.length })}
          </Text>
        }>
        <View className="h-1 bg-muted">
          <View className="h-1 bg-primary" style={{ width: `${required.length ? (done / required.length) * 100 : 100}%` }} />
        </View>
        {angles.map((a, i) => (
          <Pressable
            key={a.angle.key}
            accessibilityRole="button"
            disabled={!canCapture}
            onPress={() => setAngle(a)}
            className={cn('flex-row items-center gap-3 px-4 py-2.5 active:bg-muted', i < angles.length - 1 && 'border-b border-border')}>
            {a.photo ? (
              <Image source={apiImage(a.photo.url)} style={{ width: 52, height: 40, borderRadius: 6 }} contentFit="cover" />
            ) : (
              <View className="h-10 w-[52px] items-center justify-center rounded-md bg-secondary">
                <Icon as={Camera} tone={a.required ? 'primary' : 'muted-foreground'} size={18} />
              </View>
            )}
            <View className="flex-1 gap-0.5">
              <Text variant="body" numberOfLines={1}>
                {label(a.angle.name, i18n.language, a.angle.key)}
              </Text>
              <Text variant="footnote" tone={a.missing ? 'danger' : 'muted'} numberOfLines={1}>
                {a.photo ? t('photo_taken') : a.required ? t('photo_required') : t('photo_optional')}
              </Text>
            </View>
            {a.photo ? <Icon as={Check} tone="primary" size={18} /> : null}
          </Pressable>
        ))}
      </Section>
      {angle ? (
        <CaptureModal
          serviceUuid={uuid}
          angle={angle}
          onClose={() => setAngle(null)}
          name={label(angle.angle.name, i18n.language, angle.angle.key)}
          hint={label(angle.angle.hint, i18n.language, '')}
        />
      ) : null}
    </>
  );
}

function CaptureModal({
  serviceUuid,
  angle,
  name,
  hint,
  onClose,
}: {
  serviceUuid: string;
  angle: Angle;
  name: string;
  hint: string;
  onClose: () => void;
}) {
  const { t } = useTranslation('dealer');
  const insets = useSafeAreaInsets();
  const qc = useQueryClient();
  const camera = useRef<CameraView>(null);
  const [permission, request] = useCameraPermissions();
  const [shot, setShot] = useState<string | null>(null);
  const [taking, setTaking] = useState(false);
  const [ready, setReady] = useState(false);

  const upload = useMutation({
    mutationFn: (uri: string) => {
      const form = new FormData();
      // React Native's FormData takes a { uri, name, type } file part; expo/fetch
      // cannot send those, so uploadForm goes through XMLHttpRequest.
      form.append('image', { uri, name: `${angle.angle.key}.jpg`, type: 'image/jpeg' } as unknown as Blob);
      return uploadForm(
        `/v1/services/${encodeURIComponent(serviceUuid)}/intake-photos/${encodeURIComponent(angle.angle.key)}`,
        form,
      );
    },
    onSuccess: () => {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      void qc.invalidateQueries({ queryKey: intakeKey(serviceUuid) });
      void qc.invalidateQueries({ queryKey: ['services', 'detail', serviceUuid] });
      onClose();
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });

  const take = async () => {
    if (!camera.current || taking) return;
    setTaking(true);
    try {
      const pic = await camera.current.takePictureAsync({ quality: 0.7, skipProcessing: false });
      if (pic?.uri) setShot(pic.uri);
    } catch (e) {
      Alert.alert(t('action_failed'), e instanceof Error ? e.message : String(e));
    } finally {
      setTaking(false);
    }
  };

  return (
    <Modal visible animationType="slide" presentationStyle="fullScreen" onRequestClose={onClose}>
      <View className="flex-1 bg-black" style={{ paddingTop: insets.top, paddingBottom: insets.bottom }}>
        <View className="flex-row items-center gap-3 px-4 py-3">
          <Pressable accessibilityRole="button" accessibilityLabel={t('common:close')} hitSlop={10} onPress={onClose}>
            <Icon as={X} tone="primary-foreground" />
          </Pressable>
          <View className="flex-1">
            <Text variant="headline" className="text-white">
              {name}
            </Text>
            {hint ? (
              <Text variant="footnote" className="text-white/70" numberOfLines={2}>
                {hint}
              </Text>
            ) : null}
          </View>
        </View>

        <View className="flex-1 overflow-hidden">
          {shot ? (
            <Image source={{ uri: shot }} style={{ flex: 1 }} contentFit="contain" />
          ) : !permission ? null : !permission.granted ? (
            <View className="flex-1 items-center justify-center gap-3 px-8">
              <Text variant="body" className="text-center text-white">
                {t('camera_needed')}
              </Text>
              <Button
                title={permission.canAskAgain ? t('camera_allow') : t('open_settings')}
                onPress={() => void (permission.canAskAgain ? request() : Linking.openSettings())}
              />
            </View>
          ) : (
            <View className="flex-1">
              <CameraView ref={camera} style={{ flex: 1 }} facing="back" onCameraReady={() => setReady(true)} />
              {angle.angle.example_url ? (
                <View className="absolute bottom-3 end-3 overflow-hidden rounded-md border border-white/60">
                  <Image source={apiImage(angle.angle.example_url)} style={{ width: 96, height: 72 }} contentFit="cover" />
                </View>
              ) : null}
            </View>
          )}
        </View>

        <View className="flex-row items-center justify-center gap-4 px-4 py-5">
          {shot ? (
            <>
              <Button title={t('photo_retake')} variant="secondary" onPress={() => setShot(null)} disabled={upload.isPending} className="grow" />
              <Button
                title={t('photo_use')}
                icon={<Icon as={ImagePlus} tone="primary-foreground" size={18} />}
                loading={upload.isPending}
                onPress={() => upload.mutate(shot)}
                className="grow"
              />
            </>
          ) : permission?.granted ? (
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={t('photo_take')}
              disabled={!ready || taking}
              onPress={() => void take()}
              className="size-[72px] items-center justify-center rounded-full border-4 border-white active:opacity-70">
              {taking ? <ActivityIndicator color="#fff" /> : <View className="size-14 rounded-full bg-white" />}
            </Pressable>
          ) : null}
        </View>
      </View>
    </Modal>
  );
}
