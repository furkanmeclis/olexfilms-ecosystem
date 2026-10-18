import { FlashList } from '@shopify/flash-list';
import * as Haptics from 'expo-haptics';
import type { CountryCode } from 'libphonenumber-js/min';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal, Platform, Pressable, TextInput, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Button } from '@/ui/button';
import { fontFamily } from '@/ui/fonts';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { type Country, filterCountries, listCountries } from './phone-country';

/** Countries in the UI language; the default country first, then alphabetical. */
export function useCountries() {
  const { i18n } = useTranslation();
  return useMemo(() => listCountries(i18n.language), [i18n.language]);
}

/** Flag + calling code button; opens a searchable sheet of every country. */
export function CountryPicker({ value, onChange }: { value: CountryCode; onChange: (c: CountryCode) => void }) {
  const { t } = useTranslation('auth');
  const countries = useCountries();
  const [open, setOpen] = useState(false);
  const current = countries.find((c) => c.code === value) ?? countries[0];

  return (
    <>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={t('country_picker_a11y', { country: current.name, dial: current.dial })}
        onPress={() => setOpen(true)}
        className="min-h-[52px] flex-row items-center gap-1.5 rounded-lg border border-border bg-background px-3 active:opacity-70">
        <Text className="text-[22px] leading-[26px]">{current.flag}</Text>
        <Text variant="body" className="font-semibold tabular-nums">
          {current.dial}
        </Text>
        <Icon name="chevronDown" tone="muted-foreground" size={14} weight="medium" />
      </Pressable>
      <CountrySheet
        visible={open}
        countries={countries}
        value={value}
        onClose={() => setOpen(false)}
        onPick={(c) => {
          onChange(c);
          setOpen(false);
        }}
      />
    </>
  );
}

function CountrySheet({
  visible,
  countries,
  value,
  onClose,
  onPick,
}: {
  visible: boolean;
  countries: Country[];
  value: CountryCode;
  onClose: () => void;
  onPick: (c: CountryCode) => void;
}) {
  const { t } = useTranslation('auth');
  const insets = useSafeAreaInsets();
  const [query, setQuery] = useState('');
  const results = useMemo(() => filterCountries(countries, query), [countries, query]);
  const family = fontFamily('sans');

  const close = () => {
    setQuery('');
    onClose();
  };

  return (
    <Modal visible={visible} animationType="slide" presentationStyle="pageSheet" onRequestClose={close}>
      {/* iOS presents a page sheet below the status bar; Android shows it full screen. */}
      <View className="flex-1 bg-background" style={{ paddingTop: Platform.OS === 'ios' ? 0 : insets.top }}>
        <View className="flex-row items-center justify-between ps-5 pe-2 pb-2 pt-4">
          <Text variant="title">{t('country_title')}</Text>
          <Button title={t('common:close')} variant="ghost" onPress={close} />
        </View>
        <View className="px-4 pb-3">
          <View className="min-h-11 flex-row items-center gap-2 rounded-xl bg-secondary px-3">
            <Icon name="search" tone="muted-foreground" size={17} />
            <TextInput
              value={query}
              onChangeText={setQuery}
              placeholder={t('country_search_placeholder')}
              placeholderTextColorClassName="accent-muted-foreground"
              accessibilityLabel={t('country_search_placeholder')}
              autoCorrect={false}
              autoCapitalize="none"
              clearButtonMode="while-editing"
              returnKeyType="search"
              className="min-h-11 flex-1 text-[17px] text-foreground"
              style={family ? { fontFamily: family } : undefined}
            />
          </View>
        </View>
        <FlashList
          data={results}
          keyExtractor={(c) => c.code}
          keyboardShouldPersistTaps="handled"
          keyboardDismissMode="on-drag"
          contentContainerStyle={{ paddingBottom: insets.bottom + 16 }}
          ItemSeparatorComponent={() => <View className="ms-14 h-px bg-border" />}
          ListEmptyComponent={
            <Text variant="callout" tone="muted" className="px-5 py-8 text-center">
              {t('country_no_results')}
            </Text>
          }
          renderItem={({ item }) => (
            <Pressable
              accessibilityRole="button"
              accessibilityState={{ selected: item.code === value }}
              onPress={() => {
                void Haptics.selectionAsync();
                setQuery('');
                onPick(item.code);
              }}
              className="min-h-[52px] flex-row items-center gap-3 px-4 active:bg-secondary">
              <Text className="w-7 text-center text-[24px] leading-[30px]">{item.flag}</Text>
              <Text variant="body" className="flex-1" numberOfLines={1}>
                {item.name}
              </Text>
              <Text variant="callout" tone="muted" className="tabular-nums">
                {item.dial}
              </Text>
              <View className="w-5 items-center">
                {item.code === value ? <Icon name="check" tone="primary" size={17} weight="semibold" /> : null}
              </View>
            </Pressable>
          )}
        />
      </View>
    </Modal>
  );
}
