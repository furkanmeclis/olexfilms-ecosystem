import { Image, type ImageSourcePropType, View } from 'react-native';
import { useTranslation } from 'react-i18next';

import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { deviceModelName } from './curve';
import { lastGauge } from './manager';
import { useGauge } from './store';
import { useGaugeUserKey } from './use-gauge-session';

const FALLBACK: ImageSourcePropType = require('../../../assets/images/devices/fallback.png');

/** Model image per curve device type (NexPTGConfig.DeviceType), as in the old app; Economic and unknown use the fallback. */
const IMAGES: Record<number, ImageSourcePropType> = {
  0: require('../../../assets/images/devices/advanced.png'),
  1: require('../../../assets/images/devices/standard.png'),
  2: require('../../../assets/images/devices/professional.png'),
  5: require('../../../assets/images/devices/standard-plus.png'),
  6: require('../../../assets/images/devices/pro-carbon.png'),
};

export function gaugeImage(deviceType: number | null | undefined): ImageSourcePropType {
  return (deviceType != null && IMAGES[deviceType]) || FALLBACK;
}

/** Battery percent at or below which the low-battery glyph and the danger tone are used. */
export const LOW_BATTERY_PERCENT = 20;

/**
 * The gauge to show: the connected one, else the user's last (default) gauge.
 * `connected` is false for a known gauge that is not linked right now.
 */
export function useShownGauge() {
  const userKey = useGaugeUserKey();
  const g = useGauge();
  const saved = lastGauge(userKey);
  const connected = g.status === 'connected';
  if (!connected && !saved) return null;
  const deviceType = (connected ? g.deviceType : null) ?? saved?.deviceType ?? null;
  return {
    connected,
    deviceType,
    model: (connected ? g.model : null) ?? saved?.model ?? deviceModelName(deviceType),
    batteryPercent: connected ? g.batteryPercent : null,
  };
}

/** Device image with a red dot while the gauge is not connected. Height is ~1.8 × width (portrait PNGs). */
export function GaugeDeviceImage({
  deviceType,
  connected,
  width,
}: {
  deviceType: number | null | undefined;
  connected: boolean;
  width: number;
}) {
  const { t } = useTranslation('measure');
  const height = Math.round(width * 1.8);
  const dot = Math.max(8, Math.round(width * 0.32));
  return (
    <View style={{ width, height }} accessibilityLabel={connected ? t('ble.badge.connected') : t('ble.status.idle')}>
      <Image source={gaugeImage(deviceType)} style={{ width, height, opacity: connected ? 1 : 0.55 }} resizeMode="contain" />
      {!connected ? (
        <View
          className="absolute rounded-full border-2 border-background bg-destructive"
          style={{ width: dot, height: dot, top: 0, right: -dot / 4 }}
        />
      ) : null}
    </View>
  );
}

/** Battery glyph and percent of the connected gauge. */
export function GaugeBattery({ percent }: { percent: number | null }) {
  if (percent == null) return null;
  const low = percent <= LOW_BATTERY_PERCENT;
  return (
    <View className="flex-row items-center gap-1">
      <Icon name={low ? 'batteryLow' : 'battery'} tone={low ? 'destructive' : 'muted-foreground'} size={16} />
      <Text variant="footnote" tone={low ? 'danger' : 'muted'} className="tabular-nums">
        {`${percent}%`}
      </Text>
    </View>
  );
}

/**
 * Header badge of the measuring screens: the gauge image (red dot when not
 * connected) and its battery. Without a known gauge it falls back to the gauge icon.
 */
export function GaugeHeaderBadge() {
  const shown = useShownGauge();
  if (!shown) return <Icon name="gauge" tone="primary" />;
  return (
    <View className="flex-row items-center gap-1.5 pl-2 pr-0.5">
      <GaugeDeviceImage deviceType={shown.deviceType} connected={shown.connected} width={17} />
      {shown.connected ? <GaugeBattery percent={shown.batteryPercent} /> : null}
    </View>
  );
}
