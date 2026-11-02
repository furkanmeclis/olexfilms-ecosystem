import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { BrandMark, PlateBadge } from '@/features/customer-space/home-parts';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { fullName, type Service, serviceTone, vehicleLabel } from './lib';

/**
 * Car, plate, status and customer of a service in one card: the head of the
 * detail screen and of the wizard's summary (dealer home's visual language).
 */
export function ServiceHero({ s, children }: { s: Service; children?: React.ReactNode }) {
  const { t } = useTranslation('dealer');
  const car = vehicleLabel(s.car_brand?.name, s.car_model?.name, s.model_year);
  return (
    <View className="gap-3 rounded-2xl border border-border bg-card p-4" style={{ borderCurve: 'continuous' }}>
      <View className="flex-row items-center gap-3">
        <BrandMark uuid={s.car_brand?.uuid} name={s.car_brand?.name} size={44} />
        <View className="min-w-0 flex-1 gap-1.5">
          <Text variant="headline" numberOfLines={2}>
            {car || s.service_no}
          </Text>
          <View className="flex-row flex-wrap items-center gap-1.5">
            {s.plate ? <PlateBadge plate={s.plate} country={s.plate_country} /> : null}
            <StatusPill label={t(`service_status.${s.status}`)} tone={serviceTone(s.status)} />
          </View>
        </View>
      </View>
      <View className="flex-row items-center gap-2 border-t border-border pt-3">
        <Icon name="customer" tone="muted-foreground" size={16} />
        <Text variant="callout" numberOfLines={1} className="min-w-0 flex-1">
          {fullName(s.customer) || '—'}
        </Text>
        <Text variant="footnote" tone="muted" numberOfLines={1} className="max-w-[45%]">
          {s.organization.name}
        </Text>
      </View>
      {children}
    </View>
  );
}
