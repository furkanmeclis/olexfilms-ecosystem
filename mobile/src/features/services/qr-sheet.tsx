import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal, Pressable, View } from 'react-native';
import Svg, { Path, Rect } from 'react-native-svg';

import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { publicWarrantyUrl } from './lib';
import { encodeQr } from './qr';
import { shareWarranty } from './share';

/** QR of a text, always dark on white (scanners need the contrast in dark mode too). */
export function QrImage({ value, size = 240 }: { value: string; size?: number }) {
  const { d, n } = useMemo(() => {
    const m = encodeQr(value);
    let path = '';
    m.forEach((row, y) => row.forEach((dark, x) => dark && (path += `M${x + 4} ${y + 4}h1v1h-1z`)));
    return { d: path, n: m.length + 8 };
  }, [value]);
  return (
    <Svg width={size} height={size} viewBox={`0 0 ${n} ${n}`} accessibilityLabel={value}>
      <Rect width={n} height={n} fill="#ffffff" />
      <Path d={d} fill="#000000" />
    </Svg>
  );
}

/** Button that shows the public warranty page as a QR the customer scans from the screen. */
export function WarrantyQrButton({ code, variant = 'outline' }: { code: string; variant?: 'outline' | 'secondary' }) {
  const { t } = useTranslation('dealer');
  const [open, setOpen] = useState(false);
  const url = publicWarrantyUrl(code);
  return (
    <>
      <Button
        title={t('qr')}
        variant={variant}
        icon={<Icon name="qr" tone="primary" size={16} />}
        onPress={() => setOpen(true)}
        className="grow"
      />
      <Modal visible={open} animationType="slide" presentationStyle="pageSheet" onRequestClose={() => setOpen(false)}>
        <View className="flex-1 items-center gap-5 bg-background px-6 pt-4">
          <View className="w-full flex-row items-center justify-between">
            <Text variant="headline">{t('warranty')}</Text>
            <Pressable accessibilityRole="button" accessibilityLabel={t('common:close')} hitSlop={10} onPress={() => setOpen(false)}>
              <Icon name="close" />
            </Pressable>
          </View>
          <View className="overflow-hidden rounded-xl">
            <QrImage value={url} size={260} />
          </View>
          <Text variant="title" selectable>
            {code}
          </Text>
          <Text variant="footnote" tone="muted" selectable className="text-center">
            {url}
          </Text>
          <Text variant="callout" tone="muted" className="text-center">
            {t('qr_hint')}
          </Text>
          <Button
            title={t('share')}
            variant="secondary"
            icon={<Icon name="share" tone="primary" size={16} />}
            onPress={() => shareWarranty(code, t('warranty_share_message', { code }))}
            className="self-stretch"
          />
        </View>
      </Modal>
    </>
  );
}
