import { router, usePathname } from 'expo-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Modal, Pressable, ScrollView, View } from 'react-native';

import { usePreferences } from '@/core/preferences';
import { formatThickness } from '@/core/units';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { List, ListRow } from '@/ui/list';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { SAPL_COLORS, saplLevel } from './interpretation';
import { clearError, connect, disconnect, lastGauge, reconnectLast, retryCurve, startScan, stopScan } from './manager';
import { type GaugeStatus, useGauge } from './store';
import { useGaugeUserKey } from './use-gauge-session';

const STATUS_TONE: Record<GaugeStatus, StatusTone> = {
  idle: 'neutral',
  scanning: 'progress',
  connecting: 'progress',
  connected: 'done',
  error: 'problem',
};

/** Connection status chip of the gauge. */
export function GaugeStatusChip() {
  const { t } = useTranslation('measure');
  const status = useGauge((s) => s.status);
  return <StatusPill label={t(`ble.status.${status}`)} tone={STATUS_TONE[status]} />;
}

/** Route of a gauge screen (`device`, `calibrate`) in the current measuring stack. */
export function useGaugeHref() {
  const path = usePathname().replace(/\/$/, '');
  const base = path.match(/^(.*\/measure)(?:\/|$)/u)?.[1] ?? path;
  return (screen: 'device' | 'calibrate') => `${base}/${screen}` as never;
}

/** Model · SN · battery line of a connected gauge. */
export function useGaugeSummary(): string | null {
  const { t } = useTranslation('measure');
  const g = useGauge();
  const parts = [
    g.model,
    g.serial != null ? t('ble.serial', { serial: g.serial }) : null,
    g.batteryPercent != null ? t('ble.battery', { percent: g.batteryPercent }) : null,
  ].filter(Boolean);
  return parts.length ? parts.join(' · ') : null;
}

/** Live value with its substrate and SAPL interpretation (general mode, outside). */
export function GaugeLiveValue() {
  const { t, i18n } = useTranslation('measure');
  const unit = usePreferences((s) => s.unit);
  const reading = useGauge((s) => s.reading);
  const um = reading?.um ?? null;
  const level = um != null ? saplLevel(um) : null;
  return (
    <View className="gap-1">
      <Text variant="reading" accessibilityLiveRegion="polite">
        {um != null ? formatThickness(um, unit, i18n.language) : '—'}
      </Text>
      {um != null && reading?.material ? (
        <View className="flex-row items-center gap-2">
          <View className="size-2.5 rounded-full" style={{ backgroundColor: SAPL_COLORS[level!] }} />
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {`${t(`ble.material.${reading.material}`)} · ${t(`ble.sapl.${level}`)}`}
          </Text>
        </View>
      ) : null}
    </View>
  );
}

/**
 * Gauge card of the measuring form: connection state, the live value and the
 * part the next steady reading goes to. Render only when isGaugeAvailable().
 */
export function GaugePanel({ targetLabel }: { targetLabel: string | null }) {
  const { t } = useTranslation('measure');
  const g = useGauge();
  const userKey = useGaugeUserKey();
  const href = useGaugeHref();
  const summary = useGaugeSummary();
  const [picking, setPicking] = useState(false);
  const last = g.status === 'idle' || g.status === 'error' ? lastGauge(userKey) : null;
  const connected = g.status === 'connected';

  return (
    <View className="gap-3 rounded-lg border border-border bg-card px-4 py-3">
      <View className="flex-row items-center gap-3">
        <View className="size-9 items-center justify-center rounded-md bg-secondary">
          <Icon name="bluetooth" tone={connected ? 'primary' : 'muted-foreground'} size={20} />
        </View>
        <View className="flex-1 gap-0.5">
          <Text variant="callout" className="font-semibold" numberOfLines={1}>
            {g.device?.name ?? t('ble.title')}
          </Text>
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {connected ? summary || t('ble.waiting') : t('ble.hint')}
          </Text>
        </View>
        <GaugeStatusChip />
      </View>

      {connected ? (
        <View className="gap-1">
          <GaugeLiveValue />
          <View className="h-1 overflow-hidden rounded-full bg-secondary">
            <View className="h-1 rounded-full bg-primary" style={{ width: `${Math.round(g.progress * 100)}%` }} />
          </View>
          <Text variant="footnote" tone="muted">
            {g.curve === 'loading'
              ? t('ble.curve_loading')
              : targetLabel
                ? t('ble.target', { part: targetLabel })
                : t('ble.pick_part')}
          </Text>
          {g.noData ? (
            <Text variant="footnote" tone="danger">
              {t('ble.no_data')}
            </Text>
          ) : g.curve === 'ready' && !g.calibration ? (
            <Text variant="footnote" tone="muted">
              {t('ble.not_calibrated_hint')}
            </Text>
          ) : null}
        </View>
      ) : null}

      {g.error ? (
        <View className="flex-row items-center gap-2">
          <Text variant="footnote" tone="danger" className="flex-1">
            {t(g.error)}
          </Text>
          {g.error === 'ble.error.curve' ? (
            <Button title={t('common:retry')} variant="ghost" onPress={retryCurve} />
          ) : (
            <Button title={t('common:close')} variant="ghost" onPress={clearError} />
          )}
        </View>
      ) : null}

      <View className="flex-row gap-2">
        {connected ? (
          <>
            <Button className="flex-1" title={t('ble.device_open')} variant="outline" onPress={() => router.push(href('device'))} />
            <Button className="flex-1" title={t('ble.disconnect')} variant="ghost" onPress={() => void disconnect()} />
          </>
        ) : (
          <>
            <Button
              className="flex-1"
              title={t('ble.connect')}
              loading={g.status === 'connecting'}
              icon={<Icon name="bluetooth" tone="primary-foreground" size={18} />}
              onPress={() => setPicking(true)}
            />
            {last ? (
              <Button className="flex-1" title={t('ble.reconnect', { name: last.name })} variant="outline" onPress={() => void reconnectLast(userKey)} />
            ) : null}
          </>
        )}
      </View>

      {picking ? <DevicePickerSheet onClose={() => setPicking(false)} /> : null}
    </View>
  );
}

function signal(rssi: number | null) {
  if (rssi == null) return '';
  return `${rssi} dBm`;
}

/** Scans on open and lists nearby gauges; tapping one connects and closes. */
export function DevicePickerSheet({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation('measure');
  const userKey = useGaugeUserKey();
  const devices = useGauge((s) => s.devices);
  const status = useGauge((s) => s.status);
  const error = useGauge((s) => s.error);

  useEffect(() => {
    void startScan();
    return stopScan;
  }, []);

  const close = onClose;

  return (
    <Modal visible animationType="slide" presentationStyle="pageSheet" onRequestClose={close}>
      <View className="flex-1 bg-background">
        <View className="flex-row items-center justify-between px-4 pb-2 pt-5">
          <Text variant="title" className="flex-1" numberOfLines={1}>
            {t('ble.pick_title')}
          </Text>
          <Button title={t('common:cancel')} variant="ghost" onPress={close} />
        </View>
        <ScrollView contentContainerClassName="gap-4 px-4 pb-10">
          <Text variant="footnote" tone="muted">
            {t('ble.pick_hint')}
          </Text>
          {error ? (
            <Text variant="footnote" tone="danger">
              {t(error)}
            </Text>
          ) : null}
          {devices.length ? (
            <List>
              {devices.map((d, i) => (
                <ListRow
                  key={d.id}
                  icon="gauge"
                  title={d.name}
                  subtitle={signal(d.rssi)}
                  last={i === devices.length - 1}
                  trailing={<Icon name="forward" tone="muted-foreground" size={16} />}
                  onPress={() => {
                    void connect(d.id, userKey);
                    onClose();
                  }}
                />
              ))}
            </List>
          ) : null}
          {status === 'scanning' ? (
            <View className="flex-row items-center justify-center gap-2 py-4">
              <ActivityIndicator colorClassName="accent-muted-foreground" />
              <Text variant="callout" tone="muted">
                {t('ble.scanning')}
              </Text>
            </View>
          ) : (
            <Pressable accessibilityRole="button" onPress={() => void startScan()} className="items-center py-4 active:opacity-70">
              <Text variant="callout" tone="primary" className="font-semibold">
                {devices.length ? t('ble.scan_again') : t('ble.none_found')}
              </Text>
            </Pressable>
          )}
        </ScrollView>
      </View>
    </Modal>
  );
}
