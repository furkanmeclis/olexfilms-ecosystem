import { useQuery } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, View } from 'react-native';

import { useCan } from '@/core/spaces';
import { InfoRow, useFormat } from '@/features/admin/kit';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { List, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { calibrationSteps, newerCalibration, type StoredCalibration } from './calibration';
import { deviceModelName, type Material } from './curve';
import { GaugeBattery, GaugeDeviceImage, useShownGauge } from './gauge-badge';
import { DevicePickerSheet, GaugeLiveValue, useGaugeHref } from './gauge-panel';
import {
  cancelCalibration,
  disconnectByUser,
  fetchBackendCalibration,
  finishCalibration,
  forgetGauge,
  isGaugeAvailable,
  lastGauge,
  localCalibration,
  reconnectLast,
  startCalibrationStep,
} from './manager';
import { useGauge } from './store';
import { useGaugeHold, useGaugeUserKey } from './use-gauge-session';

/** Current calibration of a serial: the connected one, else the phone's copy and the organization's latest. */
function useCalibration(serial: number | null): { calibration: StoredCalibration | null; loading: boolean } {
  const connectedSerial = useGauge((s) => s.serial);
  const live = useGauge((s) => s.calibration);
  const remote = useQuery({
    queryKey: ['gauge-calibration', serial],
    queryFn: () => fetchBackendCalibration(serial!),
    enabled: serial != null && connectedSerial !== serial,
    retry: false,
  });
  if (serial == null) return { calibration: null, loading: false };
  if (connectedSerial === serial) return { calibration: live, loading: false };
  return { calibration: newerCalibration(localCalibration(serial), remote.data ?? null), loading: remote.isLoading };
}

function hz(v: number | null) {
  return v == null ? null : `${v > 0 ? '+' : ''}${v} Hz`;
}

/**
 * The gauge screen (the old app's device screen): the default gauge with its
 * model and serial, Default / Connected / Calibrated badges, the last
 * calibration, and Disconnect / Start calibration / Delete device.
 */
/** Shows the latest press of the gauge's button for a moment (device screen, live value box). */
function ButtonFlash() {
  const { t } = useTranslation('measure');
  const last = useGauge((s) => s.lastButton);
  // `now` only moves when the flash should end: a press shows until at + BUTTON_FLASH_MS.
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!last) return;
    const timer = setTimeout(() => setNow(Date.now()), Math.max(0, last.at + BUTTON_FLASH_MS - Date.now()));
    return () => clearTimeout(timer);
  }, [last]);
  const visible = last && now < last.at + BUTTON_FLASH_MS ? last : null;
  if (!visible) return null;
  return (
    <View className="flex-row items-center gap-1.5 rounded-full bg-primary px-3 py-1" accessibilityLiveRegion="polite">
      <Icon name="target" tone="primary-foreground" size={14} />
      <Text variant="footnote" tone="onPrimary" className="font-semibold">
        {t(`ble.button.${visible.kind}`)}
      </Text>
    </View>
  );
}

const BUTTON_FLASH_MS = 2_500;

/**
 * Gauge screen: the device (image, model, serial, link, battery, calibration
 * state) and its live value up top, the one next step (connect / calibrate)
 * as the primary action, then calibration and device details, and the rare
 * actions (disconnect, forget) at the end.
 */
export function GaugeDeviceScreen() {
  const { t } = useTranslation('measure');
  const f = useFormat();
  const userKey = useGaugeUserKey();
  const href = useGaugeHref();
  const canWrite = useCan('measurements.write');
  const g = useGauge();
  const [picking, setPicking] = useState(false);
  const [, setVersion] = useState(0);
  useGaugeHold({ pause: true });

  const saved = lastGauge(userKey);
  const shown = useShownGauge();
  const connected = g.status === 'connected';
  const busy = g.status === 'connecting' || g.status === 'scanning';
  const serial = connected ? g.serial : (saved?.serial ?? null);
  const model = (connected ? g.model : null) ?? saved?.model ?? deviceModelName(null);
  const { calibration, loading } = useCalibration(serial);
  const ready = connected && g.curve === 'ready' && g.serial != null;
  const bleName = (connected ? g.device?.name : saved?.name) ?? null;

  const onDelete = () =>
    Alert.alert(t('ble.device.delete_confirm_title'), t('ble.device.delete_confirm_body'), [
      { text: t('common:cancel'), style: 'cancel' },
      {
        text: t('ble.device.delete'),
        style: 'destructive',
        onPress: () => void forgetGauge(userKey).then(() => setVersion((v) => v + 1)),
      },
    ]);

  if (!isGaugeAvailable()) {
    return (
      <>
        <Stack.Screen options={{ title: t('ble.device_title'), headerLargeTitle: false }} />
        <EmptyState icon="bluetooth" title={t('ble_title')} body={t('ble_body')} />
      </>
    );
  }

  const statusPill = connected ? (
    <StatusPill label={t('ble.badge.connected')} tone="done" />
  ) : busy ? (
    <StatusPill label={t(`ble.status.${g.status}`)} tone="progress" />
  ) : (
    <StatusPill label={t('ble.status.idle')} tone="problem" />
  );

  return (
    <>
      <Stack.Screen options={{ title: t('ble.device_title'), headerLargeTitle: false }} />
      <Screen className="pb-16">
        {!saved && !connected ? (
          <View className="items-center gap-3 rounded-2xl border border-border bg-card px-5 py-8">
            <Icon name="gauge" tone="muted-foreground" size={36} />
            <Text variant="headline">{t('ble.device.none')}</Text>
            <Text variant="footnote" tone="muted" className="text-center">
              {t('ble.device.none_body')}
            </Text>
            <Button
              title={t('ble.connect')}
              loading={busy}
              icon={<Icon name="bluetooth" tone="primary-foreground" size={18} />}
              onPress={() => setPicking(true)}
            />
          </View>
        ) : (
          <View className="items-center gap-4 rounded-2xl border border-border bg-card px-5 pb-5 pt-6">
            <GaugeDeviceImage deviceType={shown?.deviceType} connected={connected} width={64} />
            <View className="items-center gap-0.5">
              <Text variant="title" className="text-center" numberOfLines={1}>
                {model}
              </Text>
              <Text variant="callout" tone="muted" selectable>
                {serial != null ? t('ble.serial', { serial }) : t('ble.device.serial_unknown')}
              </Text>
            </View>
            <View className="flex-row flex-wrap items-center justify-center gap-2">
              {statusPill}
              {serial != null ? (
                <StatusPill
                  label={calibration ? t('ble.badge.calibrated') : t('ble.badge.not_calibrated')}
                  tone={calibration ? 'done' : 'progress'}
                />
              ) : null}
              {connected ? <GaugeBattery percent={g.batteryPercent} /> : null}
            </View>

            {connected ? (
              <View className="w-full items-center gap-1 rounded-xl bg-secondary px-4 py-4">
                <Text variant="caption" tone="muted">
                  {t('ble.device.live')}
                </Text>
                <ButtonFlash />
                {g.reading ? (
                  <GaugeLiveValue />
                ) : (
                  <Text variant="callout" tone="muted" className="py-3 text-center">
                    {t('ble.device.probe_hint')}
                  </Text>
                )}
              </View>
            ) : (
              <Text variant="footnote" tone="muted" className="text-center">
                {t('ble.device.disconnected_body')}
              </Text>
            )}
            {connected ? (
              <Text variant="footnote" tone="muted" className="text-center">
                {t('ble.device.button_hint')}
              </Text>
            ) : null}

            {connected && g.noData ? (
              <Text variant="footnote" tone="danger" className="text-center">
                {t('ble.no_data')}
              </Text>
            ) : null}
            {g.error && !connected ? (
              <Text variant="footnote" tone="danger" className="text-center">
                {t(g.error)}
              </Text>
            ) : null}

            {!connected && saved ? (
              <Button
                className="self-stretch"
                title={t('ble.reconnect', { name: saved.name })}
                loading={busy}
                icon={<Icon name="bluetooth" tone="primary-foreground" size={18} />}
                onPress={() => void reconnectLast(userKey)}
              />
            ) : null}
          </View>
        )}

        {canWrite && connected && serial != null && !calibration && !loading ? (
          <View className="gap-3 rounded-2xl border border-warning/40 bg-warning/10 px-4 py-4">
            <View className="flex-row items-center gap-2">
              <Icon name="alert" tone="foreground" size={18} />
              <Text variant="headline">{t('ble.device.calibrate_cta_title')}</Text>
            </View>
            <Text variant="footnote" tone="muted">
              {t('ble.device.calibrate_cta_body')}
            </Text>
            <Button title={t('ble.device.start_calibration')} disabled={!ready} onPress={() => router.push(href('calibrate'))} />
            {!ready ? (
              <Text variant="footnote" tone="muted">
                {t('ble.device.calibrate_needs_connection')}
              </Text>
            ) : null}
          </View>
        ) : null}

        {serial != null ? (
          <View className="gap-2">
            <SectionHeader title={t('ble.device.last_calibration')} />
            <List>
              {calibration ? (
                <>
                  <InfoRow label={t('ble.device.calibrated_at')} value={f.dateTime(calibration.calibratedAt)} />
                  <InfoRow label={t('ble.device.calibrated_by')} value={calibration.by} />
                  <InfoRow label={t('ble.device.foil')} value={`${calibration.targetUm} µm`} />
                  <InfoRow label={t('ble.device.fe_offset')} value={hz(calibration.fe)} />
                  <InfoRow label={t('ble.device.al_offset')} value={hz(calibration.al)} />
                  <InfoRow
                    label={t('ble.device.sync')}
                    value={calibration.synced ? t('ble.device.synced') : t('ble.device.pending_sync')}
                    last
                  />
                </>
              ) : (
                <InfoRow label={t('ble.device.calibrated_at')} value={loading ? '…' : t('ble.device.never_calibrated')} last />
              )}
            </List>
            {canWrite && calibration ? (
              <Button
                title={t('ble.device.recalibrate')}
                variant="outline"
                disabled={!ready}
                onPress={() => router.push(href('calibrate'))}
              />
            ) : null}
          </View>
        ) : null}

        {connected || serial != null ? (
          <View className="gap-2">
            <SectionHeader title={t('ble.device.info')} />
            <List>
              <InfoRow label={t('ble.device.model')} value={model} />
              <InfoRow label={t('ble.device.serial')} value={serial != null ? String(serial) : null} />
              <InfoRow label={t('ble.device.ble_name')} value={bleName} />
              {connected ? (
                <InfoRow
                  label={t('ble.device.battery')}
                  value={g.batteryPercent != null ? `${g.batteryPercent}% · ${g.battery?.toFixed(1)} V` : null}
                />
              ) : null}
              <InfoRow label={t('ble.device.curve')} value={connected ? t(`ble.curve_state.${g.curve}`) : null} last />
            </List>
          </View>
        ) : null}

        {connected || saved ? (
          <View className="gap-1 pt-2">
            {connected ? <Button title={t('ble.disconnect')} variant="outline" onPress={() => void disconnectByUser()} /> : null}
            {saved ? (
              <Pressable accessibilityRole="button" onPress={onDelete} className="items-center py-3 active:opacity-60">
                <Text variant="callout" tone="danger">
                  {t('ble.device.delete')}
                </Text>
              </Pressable>
            ) : null}
          </View>
        ) : null}
      </Screen>
      {picking ? <DevicePickerSheet onClose={() => setPicking(false)} /> : null}
    </>
  );
}

/**
 * Fe / Al zero calibration wizard (REVERSE_ENGINEERING §6.1): a reference
 * foil (90 or 150 µm, from the curve) on the steel and then the aluminium
 * plate, 10 steady frames per channel, then the offsets are stored.
 */
export function GaugeCalibrationScreen() {
  const { t } = useTranslation('measure');
  const g = useGauge();
  const canWrite = useCan('measurements.write');
  const steps = calibrationSteps(g.deviceType ?? 2);
  const [index, setIndex] = useState(0);
  useGaugeHold({ pause: true });
  useEffect(() => cancelCalibration, []);

  const run = g.calibrationRun;
  const ready = g.status === 'connected' && g.curve === 'ready' && g.calibrationTargetUm != null;
  const material: Material | undefined = steps[index];
  const result = material ? run.results[material] : undefined;
  const allDone = steps.every((m) => run.results[m]);
  const um = g.calibrationTargetUm ?? 150;

  const save = () => {
    if (!finishCalibration()) return;
    Alert.alert(t('ble.cal.saved'), t('ble.cal.verify_hint', { um }));
    router.back();
  };

  return (
    <>
      <Stack.Screen options={{ title: t('ble.cal.title'), headerLargeTitle: false }} />
      <Screen>
        {!canWrite || !ready ? (
          <EmptyState icon="gauge" title={t('ble.cal.title')} body={t(!canWrite ? 'ble.cal.no_permission' : 'ble.cal.error.not_ready')} />
        ) : (
          <>
            <Text variant="callout">{t(steps.length > 1 ? 'ble.cal.intro' : 'ble.cal.intro_fe_only', { um })}</Text>

            <View className="flex-row gap-2">
              {steps.map((m, i) => (
                <StatusPill
                  key={m}
                  label={t(`ble.cal.step_${m}`, { n: i + 1 })}
                  tone={run.results[m] ? 'done' : i === index ? 'progress' : 'neutral'}
                />
              ))}
            </View>

            {material && !allDone ? (
              <View className="gap-3 rounded-lg border border-border bg-card px-4 py-4">
                <Text variant="headline">{t(`ble.cal.step_${material}`, { n: index + 1 })}</Text>
                <Text variant="callout">{t(`ble.cal.instruction_${material}`, { um })}</Text>
                <GaugeLiveValue />
                {run.phase === 'measuring' && run.material === material ? (
                  <View className="gap-1">
                    <View className="h-1.5 overflow-hidden rounded-full bg-secondary">
                      <View className="h-1.5 rounded-full bg-primary" style={{ width: `${Math.round(run.progress * 100)}%` }} />
                    </View>
                    <Text variant="footnote" tone="muted">
                      {t('ble.cal.measuring')}
                    </Text>
                  </View>
                ) : null}
                {run.phase === 'error' && run.error ? (
                  <Text variant="footnote" tone="danger">
                    {t(`ble.cal.error.${run.error}`)}
                  </Text>
                ) : null}
                {result ? (
                  <Text variant="callout" className="font-semibold">
                    {t('ble.cal.offset', { hz: hz(result.offset) })}
                  </Text>
                ) : null}
                {result ? (
                  <View className="flex-row gap-2">
                    <Button className="flex-1" title={t('ble.cal.retry')} variant="outline" onPress={() => startCalibrationStep(material)} />
                    <Button className="flex-1" title={t('ble.cal.next')} onPress={() => setIndex((i) => i + 1)} />
                  </View>
                ) : (
                  <Button
                    size="lg"
                    title={run.phase === 'error' ? t('ble.cal.retry') : t('ble.cal.measure')}
                    loading={run.phase === 'measuring'}
                    onPress={() => startCalibrationStep(material)}
                  />
                )}
              </View>
            ) : null}

            {allDone ? (
              <View className="gap-3 rounded-lg border border-border bg-card px-4 py-4">
                <Text variant="headline">{t('ble.cal.summary')}</Text>
                <List>
                  <InfoRow label={t('ble.device.foil')} value={`${um} µm`} />
                  <InfoRow label={t('ble.device.fe_offset')} value={hz(run.results.fe?.offset ?? null)} />
                  <InfoRow label={t('ble.device.al_offset')} value={hz(run.results.al?.offset ?? null)} last />
                </List>
                <Button size="lg" title={t('ble.cal.save')} onPress={save} />
              </View>
            ) : null}
          </>
        )}
      </Screen>
    </>
  );
}
