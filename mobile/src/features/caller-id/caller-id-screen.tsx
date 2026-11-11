import { Stack } from 'expo-router';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Platform, Switch, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { useCallerIdState } from './caller-id-settings';
import { callerIdNative } from './native';
import { useCallerId } from './store';
import { callerIdEligible, disableCallerId, enableCallerId, syncCallerId } from './sync';
import type { CallerIdStep } from './system';

/** How many numbered instructions each step has (`step_<id>_1` …). */
const INSTRUCTIONS: Record<CallerIdStep, number> = { ios_extension: 3, android_role: 2, android_overlay: 2 };

/** One system permission: done, or the instructions and the button that opens the right screen. */
function StepCard({ step, done, index }: { step: CallerIdStep; done: boolean; index: number }) {
  const { t } = useTranslation('callerid');
  const open = async () => {
    const opened = await callerIdNative.openStep(step);
    // The role dialog is not shown again after repeated refusals.
    if (!opened && step === 'android_role') await callerIdNative.openDefaultApps();
  };
  return (
    <View className="gap-3 rounded-lg border border-border bg-card p-4">
      <View className="flex-row items-center gap-3">
        {done ? (
          <Icon name="check" tone="primary" size={22} />
        ) : (
          <View className="size-6 items-center justify-center rounded-full bg-primary">
            <Text variant="footnote" tone="onPrimary" className="font-semibold">
              {index}
            </Text>
          </View>
        )}
        <Text variant="headline" className="flex-1">
          {t(`step_${step}_title`)}
        </Text>
      </View>
      {done ? (
        <Text variant="footnote" tone="muted">
          {t('step_done')}
        </Text>
      ) : (
        <>
          <View className="gap-1.5">
            {Array.from({ length: INSTRUCTIONS[step] }, (_, i) => (
              <Text key={i} variant="callout">
                {`${i + 1}. ${t(`step_${step}_${i + 1}`)}`}
              </Text>
            ))}
          </View>
          <Button title={t(`step_${step}_action`)} icon="externalLink" onPress={() => void open()} />
          <Text variant="footnote" tone="muted">
            {t(`step_${step}_manual`)}
          </Text>
        </>
      )}
    </View>
  );
}

export function CallerIdScreen() {
  const { t, i18n } = useTranslation('callerid');
  const me = useSession((s) => s.me);
  const state = useCallerIdState();
  const system = useCallerId((s) => s.system);
  const busy = useCallerId((s) => s.busy);
  const error = useCallerId((s) => s.error);
  const count = useCallerId((s) => s.count);
  const truncated = useCallerId((s) => s.truncated);
  const syncedAt = useCallerId((s) => s.syncedAt);
  const on = state !== 'off';
  const usable = callerIdNative.available && callerIdEligible(me) && system?.supported !== false;

  useEffect(() => {
    void useCallerId.getState().refreshSystem();
  }, []);

  const toggle = (value: boolean) => void (value ? enableCallerId() : disableCallerId());

  return (
    <>
      <Stack.Screen options={{ title: t('title') }} />
      <Screen>
        <Text variant="callout" tone="muted">
          {t('intro')}
        </Text>

        <List>
          <ListRow
            icon="phone"
            title={t('title')}
            subtitle={usable ? t(`state_${state}`) : t('unsupported')}
            trailing={
              usable ? (
                <Switch value={on} onValueChange={toggle} accessibilityLabel={t('title')} />
              ) : null
            }
            last
          />
        </List>

        {on && usable ? (
          <>
            <SectionHeader title={t('setup_title')} />
            {system?.ready ? (
              <View className="flex-row items-center gap-3 rounded-lg border border-border bg-card p-4">
                <Icon name="check" tone="primary" size={22} />
                <Text variant="callout" className="flex-1">
                  {t('setup_done')}
                </Text>
              </View>
            ) : null}
            {(system?.steps ?? [])
              .filter((s) => !system?.ready || !s.done)
              .map((s, i) => (
                <StepCard key={s.id} step={s.id} done={s.done} index={i + 1} />
              ))}

            <SectionHeader title={t('list_title')} />
            <List>
              <ListRow icon="customers" title={t('list_count', { count })} subtitle={syncedAt ? t('list_synced', { time: new Date(syncedAt).toLocaleString(i18n.language) }) : t('list_never')} trailing={busy ? <ActivityIndicator /> : null} last />
            </List>
            {truncated ? (
              <Text variant="footnote" tone="muted">
                {t('list_truncated', { count })}
              </Text>
            ) : null}
            {error ? (
              <Text variant="footnote" tone="danger">
                {t(`error_${error}`)}
              </Text>
            ) : null}
            <Button
              title={t('sync_now')}
              variant="outline"
              loading={busy}
              onPress={() => void syncCallerId({ force: true, full: true })}
            />
          </>
        ) : null}

        <View className="gap-2 pt-2">
          {Platform.OS === 'ios' ? (
            <Text variant="footnote" tone="muted">
              {t('note_ios_delay')}
            </Text>
          ) : (
            <>
              <Text variant="footnote" tone="muted">
                {t('note_android_contacts')}
              </Text>
              <Text variant="footnote" tone="muted">
                {t('note_android_battery')}
              </Text>
            </>
          )}
          <Text variant="footnote" tone="muted">
            {t('note_privacy')}
          </Text>
        </View>
      </Screen>
    </>
  );
}
