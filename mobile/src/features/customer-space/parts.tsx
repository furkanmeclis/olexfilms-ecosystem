import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { formatDate, formatDateTime, vehicleLabel, warrantyTone, type Warranty } from '@/features/services/lib';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import {
  appointmentTone,
  canCancelAppointment,
  daysLeft,
  nameOf,
  type PortalActiveWarranty,
  type PortalAppointment,
  type PortalServiceItem,
  portalServiceTone,
  type PortalVehicle,
} from './lib';
import type { IconName } from '@/ui/icons';

/** Localized service status; the backend label (or the raw code) when unknown. */
export function useServiceStatusLabel() {
  const { t, i18n } = useTranslation('customer');
  return (status: string | undefined, fallback?: string) => {
    const key = `customer:service_status.${status ?? ''}`;
    return status && i18n.exists(key) ? t(`service_status.${status}`) : (fallback ?? status ?? '');
  };
}

function Card({ onPress, children }: { onPress?: () => void; children: React.ReactNode }) {
  const cls = 'gap-1 rounded-lg border border-border bg-card px-4 py-3';
  if (!onPress) return <View className={cls}>{children}</View>;
  return (
    <Pressable accessibilityRole="button" onPress={onPress} className={`${cls} active:opacity-80`}>
      {children}
    </Pressable>
  );
}

export function VehicleCard({ v, onPress }: { v: PortalVehicle; onPress: () => void }) {
  const { t, i18n } = useTranslation('customer');
  const label = vehicleLabel(v.car_brand?.name, v.car_model?.name, v.model_year);
  const facts = [
    t('service_count', { count: v.service_count ?? 0 }),
    v.last_service_at ? t('last_service', { date: formatDate(v.last_service_at, i18n.language) }) : null,
  ].filter(Boolean);
  return (
    <Card onPress={onPress}>
      <View className="flex-row items-center gap-3">
        <View className="size-10 items-center justify-center rounded-md bg-secondary">
          <Icon name="vehicle" tone="primary" size={20} />
        </View>
        <View className="flex-1 gap-0.5">
          <Text variant="headline" numberOfLines={1}>
            {v.plate || label || t('vehicle')}
          </Text>
          {v.plate && label ? (
            <Text variant="callout" numberOfLines={1}>
              {label}
            </Text>
          ) : null}
        </View>
        {(v.active_warranty_count ?? 0) > 0 ? (
          <StatusPill label={t('active_warranties_short', { count: v.active_warranty_count })} tone="done" />
        ) : null}
      </View>
      <Text variant="footnote" tone="muted" numberOfLines={1}>
        {facts.join(' · ')}
      </Text>
    </Card>
  );
}

export function PortalServiceRow({ s, onPress }: { s: PortalServiceItem; onPress: () => void }) {
  const { i18n } = useTranslation('customer');
  const statusLabel = useServiceStatusLabel();
  return (
    <Card onPress={onPress}>
      <View className="flex-row items-center justify-between gap-3">
        <Text variant="headline" numberOfLines={1} className="flex-1">
          {s.plate || s.service_no}
        </Text>
        <StatusPill label={statusLabel(s.status)} tone={portalServiceTone(s.status)} />
      </View>
      <Text variant="callout" numberOfLines={1}>
        {[vehicleLabel(s.car_brand_name, s.car_model_name, s.model_year), s.package].filter(Boolean).join(' · ')}
      </Text>
      <Text variant="footnote" tone="muted" numberOfLines={1}>
        {[s.service_no, nameOf(s.organization), formatDate(s.completed_at ?? s.created_at, i18n.language)]
          .filter(Boolean)
          .join(' · ')}
      </Text>
    </Card>
  );
}

export function WarrantyRow({ w, onPress }: { w: Warranty; onPress: () => void }) {
  const { t, i18n } = useTranslation('customer');
  const left = w.status === 'active' ? daysLeft(w.end_at) : null;
  return (
    <Card onPress={onPress}>
      <View className="flex-row items-center justify-between gap-3">
        <Text variant="headline" numberOfLines={1} className="flex-1">
          {w.product?.name || w.public_code}
        </Text>
        <StatusPill label={t(`warranty_status.${w.status}`)} tone={warrantyTone(w.status)} />
      </View>
      <Text variant="callout" numberOfLines={1}>
        {[w.vehicle?.plate, vehicleLabel(w.vehicle?.brand_name, w.vehicle?.model_name, w.vehicle?.model_year)]
          .filter(Boolean)
          .join(' · ')}
      </Text>
      <Text variant="footnote" tone="muted" numberOfLines={1}>
        {[
          w.public_code,
          t('until', { date: formatDate(w.end_at, i18n.language) }),
          left !== null && left >= 0 ? t('days_left', { count: left }) : null,
        ]
          .filter(Boolean)
          .join(' · ')}
      </Text>
    </Card>
  );
}

export function ActiveWarrantyRow({ w, onPress }: { w: PortalActiveWarranty; onPress: () => void }) {
  const { t, i18n } = useTranslation('customer');
  return (
    <Card onPress={onPress}>
      <View className="flex-row items-center justify-between gap-3">
        <Text variant="headline" numberOfLines={1} className="flex-1">
          {w.product?.name || w.public_code}
        </Text>
        <StatusPill label={t('days_left', { count: Math.max(0, w.days_left ?? 0) })} tone="done" />
      </View>
      <Text variant="footnote" tone="muted" numberOfLines={1}>
        {[w.public_code, nameOf(w.organization), t('until', { date: formatDate(w.end_at, i18n.language) })]
          .filter(Boolean)
          .join(' · ')}
      </Text>
      {typeof w.percent_left === 'number' ? (
        <View className="mt-1 h-1.5 overflow-hidden rounded-full bg-secondary">
          <View
            className="h-full rounded-full bg-success"
            style={{ width: `${Math.min(100, Math.max(0, w.percent_left))}%` }}
          />
        </View>
      ) : null}
    </Card>
  );
}

export function AppointmentRow({
  a,
  onCancel,
  cancelling,
}: {
  a: PortalAppointment;
  onCancel?: () => void;
  cancelling?: boolean;
}) {
  const { t, i18n } = useTranslation('customer');
  return (
    <Card>
      <View className="flex-row items-center justify-between gap-3">
        <Text variant="headline" numberOfLines={1} className="flex-1">
          {formatDateTime(a.starts_at, i18n.language)}
        </Text>
        <StatusPill label={t(`appointment_status.${a.status}`)} tone={appointmentTone(a.status)} />
      </View>
      <Text variant="callout" numberOfLines={1}>
        {[a.dealer_name, a.vehicle_plate ?? a.vehicle_label].filter(Boolean).join(' · ')}
      </Text>
      {a.note ? (
        <Text variant="footnote" tone="muted" numberOfLines={2}>
          {a.note}
        </Text>
      ) : null}
      {a.status === 'cancelled' && a.cancel_reason ? (
        <Text variant="footnote" tone="muted" numberOfLines={2}>
          {a.cancel_reason}
        </Text>
      ) : null}
      {onCancel && canCancelAppointment(a) ? (
        <Button
          title={t('appointment_cancel')}
          variant="outline"
          loading={cancelling}
          onPress={onCancel}
          className="mt-2 self-start"
        />
      ) : null}
    </Card>
  );
}

/** Small stat tile used on the home screen. */
export function StatTile({ icon, label, value, onPress }: { icon: IconName; label: string; value: string; onPress?: () => void }) {
  const body = (
    <>
      <View className="flex-row items-center gap-2">
        <Icon name={icon} tone="primary" size={16} />
        <Text variant="footnote" tone="muted" numberOfLines={1} className="flex-1">
          {label}
        </Text>
      </View>
      <Text variant="title" className="tabular-nums">
        {value}
      </Text>
    </>
  );
  const cls = 'min-h-20 grow basis-[46%] justify-between gap-1 rounded-lg border border-border bg-card p-3';
  if (!onPress) return <View className={cls}>{body}</View>;
  return (
    <Pressable accessibilityRole="button" onPress={onPress} className={`${cls} active:opacity-80`}>
      {body}
    </Pressable>
  );
}
