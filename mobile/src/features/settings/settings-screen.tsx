import { reloadAppAsync } from 'expo';
import { router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Alert, Platform } from 'react-native';

import { useSession } from '@/core/auth/session';
import { API_URL, APP_VERSION } from '@/core/config';
import { type Language, LANGUAGES, setLanguage } from '@/core/i18n';
import { type ThemePreference, type ThicknessUnit, usePreferences } from '@/core/preferences';
import { getUpdateInfo } from '@/core/updates';
import { PlateRecognitionSettings } from '@/features/plate-scan/plate-settings';
import { Icon } from '@/ui/icon';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

const THEMES: ThemePreference[] = ['system', 'light', 'dark'];
const UNITS: ThicknessUnit[] = ['um', 'mil'];

function Checked({ on }: { on: boolean }) {
  return on ? <Icon name="check" tone="primary" size={20} /> : null;
}

export function SettingsScreen() {
  const { t, i18n } = useTranslation('settings');
  const theme = usePreferences((s) => s.theme);
  const unit = usePreferences((s) => s.unit);
  const ota = getUpdateInfo();

  const pickLanguage = async (lang: Language) => {
    const needsRestart = await setLanguage(lang);
    if (!needsRestart) return;
    // `t` still holds the previous language from this render; ask in the new one.
    const tl = (key: string) => i18n.t(`settings:${key}`, { lng: lang });
    // iOS reads the app language at process start; a JS reload is not enough
    // and an app cannot relaunch itself there.
    if (Platform.OS === 'ios') {
      Alert.alert(tl('restart_title'), tl('restart_reopen_body'), [{ text: tl('restart_ok') }]);
      return;
    }
    Alert.alert(tl('restart_title'), tl('restart_body'), [
      { text: tl('restart_now'), onPress: () => void reloadAppAsync() },
    ]);
  };

  return (
    <>
      <Stack.Screen options={{ title: t('title') }} />
      <Screen>
        <SectionHeader title={t('language')} />
        <List>
          {LANGUAGES.map((l, i) => (
            <ListRow
              key={l.code}
              title={l.name}
              onPress={() => void pickLanguage(l.code)}
              trailing={<Checked on={i18n.language === l.code} />}
              last={i === LANGUAGES.length - 1}
            />
          ))}
        </List>

        <SectionHeader title={t('theme')} />
        <List>
          {THEMES.map((th, i) => (
            <ListRow
              key={th}
              title={t(`theme.${th}`)}
              onPress={() => usePreferences.getState().setTheme(th)}
              trailing={<Checked on={theme === th} />}
              last={i === THEMES.length - 1}
            />
          ))}
        </List>

        <SectionHeader title={t('unit')} />
        <List>
          {UNITS.map((u, i) => (
            <ListRow
              key={u}
              title={t(`unit.${u}`)}
              onPress={() => usePreferences.getState().setUnit(u)}
              trailing={<Checked on={unit === u} />}
              last={i === UNITS.length - 1}
            />
          ))}
        </List>

        <PlateRecognitionSettings />

        <SectionHeader title={t('about')} />
        <List>
          <ListRow icon="network" title={API_URL.replace(/^https?:\/\//, '')} subtitle={t('api_server')} />
          <ListRow icon="info" title={APP_VERSION} subtitle={t('app_version')} />
          {ota.enabled ? (
            <>
              <ListRow icon="download" title={ota.channel ?? '-'} subtitle={t('ota_channel')} />
              <ListRow icon="info" title={ota.runtimeVersion ?? '-'} subtitle={t('ota_runtime')} />
              <ListRow
                icon="info"
                title={ota.embedded ? t('ota_embedded') : (ota.updateId ?? '-')}
                subtitle={t('ota_update_id')}
                last
              />
            </>
          ) : (
            <ListRow icon="download" title={t('ota_off')} subtitle={t('ota_channel')} last />
          )}
        </List>

        <List className="mt-4">
          <ListRow icon="intro" title={t('watch_intro')} onPress={() => router.push('/intro')} />
          <ListRow icon="spaces" title={t('switch_space')} onPress={() => router.push('/spaces')} />
          <ListRow icon="signOut" title={t('sign_out')} tone="danger" onPress={() => void useSession.getState().signOut()} last />
        </List>

        <Text variant="footnote" tone="muted" className="text-center">
          {t('version', { version: APP_VERSION })}
        </Text>
      </Screen>
    </>
  );
}
