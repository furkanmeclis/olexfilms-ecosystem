import { useQuery } from '@tanstack/react-query';
import { CameraView, useCameraPermissions } from 'expo-camera';
import * as Haptics from 'expo-haptics';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';

import { useDebounced } from './lib';
import { extractServiceNo, isValidServiceNo } from './service-no';

function CardScanner({ onCode }: { onCode: (code: string) => void }) {
  const { t } = useTranslation('dealer');
  const [permission, request] = useCameraPermissions();
  const done = useRef(false);
  if (!permission) return null;
  if (!permission.granted) {
    return <Button title={t('camera_allow')} variant="secondary" onPress={() => void request()} />;
  }
  return (
    <View className="h-56 overflow-hidden rounded-lg bg-black">
      <CameraView
        style={{ flex: 1 }}
        barcodeScannerSettings={{ barcodeTypes: ['qr'] }}
        onBarcodeScanned={({ data }) => {
          if (done.current) return;
          done.current = true;
          void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
          onCode(data);
        }}
      />
    </View>
  );
}

/**
 * Service number of a new service (TEC-1004): typed, pasted (the warranty
 * card URL is reduced to its number) or scanned from the card's QR; checked
 * as it changes. Empty means a generated number, or with `current` (an open
 * draft) that the current number stays.
 */
export function ServiceNoField({
  value,
  onChange,
  current,
}: {
  value: string;
  onChange: (v: string) => void;
  current?: string;
}) {
  const { t } = useTranslation('dealer');
  const [scanning, setScanning] = useState(false);
  const no = useDebounced(value.trim());
  const same = !!current && no === current;
  const valid = isValidServiceNo(no);
  const check = useQuery({
    queryKey: ['services', 'service-no', no],
    queryFn: () => unwrap(api.GET('/v1/services/service-no-availability', { params: { query: { service_no: no } } })),
    enabled: valid && !same,
    staleTime: 10_000,
  });
  const error = !no || same
    ? undefined
    : !valid
      ? t('service_no_invalid')
      : check.data && !check.data.available
        ? t('service_no_taken')
        : undefined;
  const hint = !no
    ? current
      ? t('service_no_keep', { no: current })
      : t('service_no_auto')
    : same
      ? undefined
      : check.data?.available
        ? t('service_no_available')
        : undefined;

  return (
    <View className="gap-2">
      <Input
        label={t('service_no_label')}
        placeholder={t('service_no_placeholder')}
        autoCapitalize="none"
        autoCorrect={false}
        value={value}
        onChangeText={(text) => onChange(text.includes('/') ? extractServiceNo(text) : text)}
        onEndEditing={(e) => onChange(extractServiceNo(e.nativeEvent.text))}
        error={error}
        hint={hint}
        trailing={
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t('service_no_scan')}
            onPress={() => setScanning((s) => !s)}
            className="min-h-[52px] w-[52px] items-center justify-center rounded-lg border border-border bg-card">
            <Icon name="qr" size={22} />
          </Pressable>
        }
      />
      {scanning ? (
        <CardScanner
          onCode={(code) => {
            setScanning(false);
            onChange(extractServiceNo(code));
          }}
        />
      ) : null}
    </View>
  );
}
