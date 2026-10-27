import { Image } from 'expo-image';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';
import Animated, { FadeInDown } from 'react-native-reanimated';
import Svg, { Defs, RadialGradient, Rect, Stop } from 'react-native-svg';
import { useCSSVariable } from 'uniwind';

import type { components } from '@/core/api/schema';
import { API_URL } from '@/core/config';
import { formatDigits, intlLocale } from '@/core/digits';
import { formatDate } from '@/features/services/lib';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { Text } from '@/ui/text';

type PortalVehicle = components['schemas']['PortalVehicle'];
type ActiveWarranty = components['schemas']['PortalActiveWarranty'];

/** Model hero → brand hero → default (backend fallback chain, public and cacheable). */
export function heroUrl(v: Pick<PortalVehicle, 'car_brand' | 'car_model'>) {
  if (v.car_model?.uuid) return `${API_URL}/v1/public/vehicle-heroes/models/${v.car_model.uuid}`;
  if (v.car_brand?.uuid) return `${API_URL}/v1/public/vehicle-heroes/brands/${v.car_brand.uuid}`;
  return `${API_URL}/v1/public/vehicle-heroes/default`;
}

export function brandLogoUrl(brandUuid: string) {
  return `${API_URL}/v1/public/brand-logos/${brandUuid}`;
}

/** Cards rise in one after another; Reanimated skips it under Reduce Motion. */
export function Rise({ index, children, className }: { index: number; children: React.ReactNode; className?: string }) {
  return (
    <Animated.View entering={FadeInDown.delay(index * 70).springify().damping(18)} className={className}>
      {children}
    </Animated.View>
  );
}

/** Brand logo in a rounded tile; the brand's initial when there is no logo. */
export function BrandMark({ uuid, name, size = 40 }: { uuid?: string | null; name?: string | null; size?: number }) {
  const [failed, setFailed] = useState(false);
  return (
    <View
      className="items-center justify-center overflow-hidden rounded-xl border border-border bg-card"
      style={{ width: size, height: size, borderCurve: 'continuous' }}>
      {uuid && !failed ? (
        <Image
          source={brandLogoUrl(uuid)}
          contentFit="contain"
          style={{ width: size * 0.7, height: size * 0.7 }}
          onError={() => setFailed(true)}
          accessibilityIgnoresInvertColors
        />
      ) : (
        <Text variant="headline" tone="primary">
          {(name ?? '?').charAt(0).toLocaleUpperCase()}
        </Text>
      )}
    </View>
  );
}

/** EU-style plate: blue country strip, black characters on white, in both themes. */
export function PlateBadge({ plate, country }: { plate: string; country?: string | null }) {
  return (
    <View className="flex-row overflow-hidden rounded-md border border-black/70 bg-white" style={{ direction: 'ltr' }}>
      <View className="items-center justify-center px-1.5" style={{ backgroundColor: '#1f4fb4' }}>
        <Text className="text-[10px] font-bold text-white">{(country || 'TR').toUpperCase()}</Text>
      </View>
      <Text className="px-2.5 py-1 text-[15px] font-bold tracking-[1.5px] text-black">{plate}</Text>
    </View>
  );
}

/** Soft light behind the car, in the brand color of the theme. */
function Glow({ width, height }: { width: number; height: number }) {
  const primary = useCSSVariable('--color-primary');
  const color = typeof primary === 'string' ? primary : '#003d25';
  return (
    <Svg width={width} height={height} style={{ position: 'absolute', left: 0, top: 0 }} pointerEvents="none">
      <Defs>
        <RadialGradient id="glow" cx="0.6" cy="0.62" r="0.55">
          <Stop offset="0" stopColor={color} stopOpacity="0.22" />
          <Stop offset="1" stopColor={color} stopOpacity="0" />
        </RadialGradient>
      </Defs>
      <Rect width={width} height={height} fill="url(#glow)" />
    </Svg>
  );
}

/** The selected car: name, plate and the cut-out car running off the right edge. */
export function VehiclePage({ v, width, onPress }: { v: PortalVehicle; width: number; onPress: () => void }) {
  const { t, i18n } = useTranslation('customer');
  const carWidth = width * 0.96;
  const carHeight = carWidth / 2.4;
  const pageHeight = carHeight + 132;
  return (
    // Clipped at the screen edge: the car runs off the right side without widening the page.
    <Pressable accessibilityRole="button" onPress={onPress} style={{ width, height: pageHeight }} className="overflow-hidden active:opacity-90">
      <Glow width={width} height={pageHeight} />
      <View className="flex-row items-center gap-3 px-5">
        <BrandMark uuid={v.car_brand?.uuid} name={v.car_brand?.name} size={38} />
        <View className="flex-1">
          <Text variant="headline" numberOfLines={1}>
            {[v.car_brand?.name, v.car_model?.name].filter(Boolean).join(' ')}
          </Text>
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {[v.model_year ? formatDigits(v.model_year, i18n.language) : null, v.last_service_at ? t('home_last_service', { date: formatDate(v.last_service_at, i18n.language) }) : null]
              .filter(Boolean)
              .join(' · ')}
          </Text>
        </View>
        {v.plate ? <PlateBadge plate={v.plate} country={v.plate_country} /> : null}
      </View>
      <Image
        source={heroUrl(v)}
        contentFit="contain"
        transition={250}
        style={{ position: 'absolute', width: carWidth, height: carHeight, right: -width * 0.07, bottom: 20 }}
        accessibilityIgnoresInvertColors
      />
      <View className="absolute bottom-0 start-5 flex-row gap-2">
        <Chip icon="warranty" label={t('home_active_warranties', { count: v.active_warranty_count })} />
        <Chip icon="service" label={t('home_service_count', { count: v.service_count })} />
      </View>
    </Pressable>
  );
}

function Chip({ icon, label }: { icon: IconName; label: string }) {
  return (
    <View className="flex-row items-center gap-1.5 rounded-full border border-border bg-card/90 px-3 py-1.5">
      <Icon name={icon} tone="primary" size={13} weight="medium" />
      <Text variant="footnote" className="font-semibold">
        {label}
      </Text>
    </View>
  );
}

/** Remaining time as years / months / days, the largest units only. */
export function useTimeLeft() {
  const { t } = useTranslation('customer');
  return (days: number) => {
    if (days <= 0) return t('left_ended');
    const years = Math.floor(days / 365);
    const months = Math.floor((days % 365) / 30);
    if (years > 0 && months > 0) return t('left_years_months', { years, months });
    if (years > 0) return t('left_years', { count: years });
    if (months > 0) return t('left_months', { count: months });
    return t('left_days', { count: days });
  };
}

/** Warranty with a bar of the time still covered; amber in its last 10 %. */
export function WarrantyProgressCard({ w, onPress }: { w: ActiveWarranty; onPress: () => void }) {
  const { t, i18n } = useTranslation('customer');
  const left = useTimeLeft();
  const pct = Math.max(0, Math.min(1, w.percent_left > 1 ? w.percent_left / 100 : w.percent_left));
  const ending = pct < 0.1;
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      className="gap-3 rounded-2xl border border-border bg-card p-4 active:opacity-85"
      style={{ borderCurve: 'continuous' }}>
      <View className="flex-row items-start gap-3">
        <View className="size-10 items-center justify-center rounded-full bg-primary/10">
          <Icon name="warranty" tone="primary" size={19} />
        </View>
        <View className="flex-1 gap-0.5">
          <Text variant="caption" tone="muted" className="font-semibold">
            {w.product.name}
          </Text>
          <Text variant="headline">{left(w.days_left)}</Text>
        </View>
        <Icon name="forward" tone="muted-foreground" size={14} />
      </View>
      <View className="h-1.5 overflow-hidden rounded-full bg-muted">
        <View className={cn('h-full rounded-full', ending ? 'bg-brand' : 'bg-primary')} style={{ width: `${pct * 100}%` }} />
      </View>
      <View className="flex-row justify-between">
        <Text variant="footnote" tone="muted">
          {formatDate(w.start_at, i18n.language)}
        </Text>
        <Text variant="footnote" tone="muted">
          {t('home_until', { date: formatDate(w.end_at, i18n.language) })}
        </Text>
      </View>
    </Pressable>
  );
}

/** The one next step, on the brand-green card. */
export function NextStepCard({
  icon,
  eyebrow,
  title,
  body,
  cta,
  onPress,
}: {
  icon: IconName;
  eyebrow: string;
  title: string;
  body?: string;
  cta: string;
  onPress: () => void;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      className="gap-3 rounded-[22px] p-5 active:opacity-90"
      style={{ backgroundColor: '#003d25', borderCurve: 'continuous' }}>
      <View className="flex-row items-center gap-2">
        <Icon name={icon} tone="brand" size={15} weight="semibold" />
        <Text variant="caption" className="font-semibold text-brand">
          {eyebrow}
        </Text>
      </View>
      <View className="gap-1">
        <Text variant="title" className="text-on-media">
          {title}
        </Text>
        {body ? (
          <Text variant="callout" className="text-on-media/75">
            {body}
          </Text>
        ) : null}
      </View>
      <View className="flex-row items-center gap-1.5 self-start rounded-full bg-on-media px-4 py-2">
        <Text variant="footnote" className="font-semibold" style={{ color: '#003d25' }}>
          {cta}
        </Text>
      </View>
    </Pressable>
  );
}

/** Round shortcut with its label underneath. */
export function QuickAction({ icon, label, onPress }: { icon: IconName; label: string; onPress: () => void }) {
  return (
    <Pressable accessibilityRole="button" onPress={onPress} className="flex-1 items-center gap-2 active:opacity-70">
      <View className="size-14 items-center justify-center rounded-2xl border border-border bg-card" style={{ borderCurve: 'continuous' }}>
        <Icon name={icon} tone="primary" size={22} />
      </View>
      <Text variant="footnote" className="text-center font-medium" numberOfLines={2}>
        {label}
      </Text>
    </Pressable>
  );
}

/** Day and month stacked, like a ticket stub. */
export function DateStub({ iso }: { iso: string }) {
  const { i18n } = useTranslation();
  const d = new Date(iso);
  const day = new Intl.DateTimeFormat(intlLocale(i18n.language), { day: '2-digit' }).format(d);
  const month = new Intl.DateTimeFormat(intlLocale(i18n.language), { month: 'short' }).format(d);
  return (
    <View className="w-12 items-center rounded-xl bg-muted py-1.5">
      <Text variant="headline" className="tabular-nums">
        {day}
      </Text>
      <Text variant="caption" tone="muted">
        {month}
      </Text>
    </View>
  );
}
