import { useMutation, useQueryClient } from '@tanstack/react-query';
import { CameraView, useCameraPermissions } from 'expo-camera';
import * as Haptics from 'expo-haptics';
import { Image } from 'expo-image';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Alert, Linking, Modal, Pressable, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { uploadForm } from '@/core/api/client';
import { errorText } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

export const claimKey = (uuid: string) => ['warranty-claims', 'detail', uuid] as const;

/** Full-screen camera for one claim photo: take, review, upload (JPEG). */
export function ClaimPhotoCapture({ claimUuid, onClose }: { claimUuid: string; onClose: () => void }) {
  const { t } = useTranslation('claims');
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
      // React Native's FormData takes a { uri, name, type } file part; uploadForm sends it with XMLHttpRequest.
      form.append('file', { uri, name: 'claim.jpg', type: 'image/jpeg' } as unknown as Blob);
      return uploadForm(`/v1/warranty-claims/${encodeURIComponent(claimUuid)}/photos`, form);
    },
    onSuccess: () => {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      void qc.invalidateQueries({ queryKey: claimKey(claimUuid) });
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
    <Modal
      visible
      animationType="slide"
      presentationStyle="fullScreen"
      onRequestClose={() => {
        if (!upload.isPending) onClose();
      }}>
      <View className="flex-1 bg-black" style={{ paddingTop: insets.top, paddingBottom: insets.bottom }}>
        <View className="flex-row items-center gap-3 px-4 py-3">
          <Pressable accessibilityRole="button" accessibilityLabel={t('common:close')} hitSlop={10} disabled={upload.isPending} onPress={onClose}>
            <Icon name="close" tone="primary-foreground" />
          </Pressable>
          <Text variant="headline" className="flex-1 text-white">
            {t('photo_add')}
          </Text>
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
            <CameraView ref={camera} style={{ flex: 1 }} facing="back" onCameraReady={() => setReady(true)} />
          )}
        </View>

        <View className="flex-row items-center justify-center gap-4 px-4 py-5">
          {shot ? (
            <>
              <Button title={t('photo_retake')} variant="secondary" onPress={() => setShot(null)} disabled={upload.isPending} className="grow" />
              <Button
                title={t('photo_use')}
                icon={<Icon name="addPhoto" tone="primary-foreground" size={18} />}
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
              {taking ? <ActivityIndicator colorClassName="accent-on-media" /> : <View className="size-14 rounded-full bg-white" />}
            </Pressable>
          ) : null}
        </View>
      </View>
    </Modal>
  );
}
