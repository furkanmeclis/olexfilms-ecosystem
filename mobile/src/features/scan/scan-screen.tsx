import { CameraView, useCameraPermissions } from 'expo-camera';
import * as Haptics from 'expo-haptics';
import { type Href, router, useIsFocused, usePathname } from 'expo-router';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Linking, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { useCan } from '@/core/spaces';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { Text } from '@/ui/text';

import { openWarranty } from '@/features/services/share';
import { WarehouseScan } from '@/features/warehouse/warehouse-scan';

import { classifyCode, scanAction, type ScannedCode } from './scan-codes';

const BARCODE_TYPES = ['qr', 'datamatrix', 'code128', 'code39', 'ean13', 'ean8'] as const;

/**
 * One camera for every code. The camera only runs while this tab is on screen.
 * In the warehouse space every code is resolved by the warehouse scan endpoint
 * and opens its result card with next actions.
 */
export function ScanScreen({ space }: { space?: 'depo' } = {}) {
  const { t } = useTranslation('scan');
  const insets = useSafeAreaInsets();
  const focused = useIsFocused();
  const [permission, requestPermission] = useCameraPermissions();
  const [result, setResult] = useState<ScannedCode | null>(null);
  const busy = useRef(false);
  const pathname = usePathname();
  const canStock = useCan('stock.read');
  const action = result ? scanAction(result, { space: pathname.startsWith('/admin') ? 'admin' : 'bayi', canStock }) : null;

  if (!permission) return <View className="flex-1 bg-black" />;

  if (!permission.granted) {
    return (
      <View className="flex-1 justify-center bg-background">
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
      </View>
    );
  }

  if (space === 'depo') return <WarehouseScan />;

  const onScanned = ({ data }: { data: string }) => {
    if (busy.current) return;
    busy.current = true;
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    const code = classifyCode(data);
    if (code.kind === 'qr_login') {
      router.push({ pathname: '/qr-login/[code]', params: { code: code.code } });
      setTimeout(() => (busy.current = false), 1500);
      return;
    }
    setResult(code);
  };

  return (
    <View className="flex-1 bg-black">
      {focused ? (
        <CameraView
          style={{ flex: 1 }}
          facing="back"
          barcodeScannerSettings={{ barcodeTypes: [...BARCODE_TYPES] }}
          onBarcodeScanned={result ? undefined : onScanned}
        />
      ) : null}
      <View pointerEvents="none" className="absolute inset-0 items-center justify-center">
        <View className="size-64 rounded-2xl border-[3px] border-white/90" />
        <Text variant="callout" className="mt-5 text-white/90">
          {t('hint')}
        </Text>
      </View>
      {result ? (
        <View className="absolute inset-x-3 gap-3 rounded-sheet bg-card p-4" style={{ bottom: insets.bottom + 90 }}>
          <Text variant="caption" tone="muted">
            {t(`result.${result.kind}`)}
          </Text>
          <Text variant="title" selectable className="font-mono">
            {result.code}
          </Text>
          {action ? (
            <Button
              title={t(action.kind === 'unit' ? 'open_unit' : 'open_warranty')}
              icon={action.kind === 'unit' ? 'barcode' : 'shield'}
              onPress={() => {
                if (action.kind === 'unit') router.push(action.path as Href);
                else void openWarranty(action.code);
              }}
            />
          ) : (
            <Text variant="callout" tone="muted">
              {t(`next_hint.${result.kind}`)}
            </Text>
          )}
          <Button
            title={t('scan_again')}
            variant="secondary"
            onPress={() => {
              setResult(null);
              busy.current = false;
            }}
          />
        </View>
      ) : null}
    </View>
  );
}
