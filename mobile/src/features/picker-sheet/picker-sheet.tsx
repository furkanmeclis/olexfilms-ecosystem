import * as Haptics from 'expo-haptics';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, FlatList, KeyboardAvoidingView, Modal, Platform, Pressable, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { errorText, SearchField } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';
import type { IconName } from '@/ui/icons';

import { filterOptions, needsSearch, pickerRows, type SearchableOption } from './search';

export type PickerOption = SearchableOption & {
  icon?: IconName;
  disabled?: boolean;
};

export type PickerSheetProps<T extends PickerOption> = {
  visible: boolean;
  title: string;
  options: readonly T[];
  selectedKey?: string | null;
  /** Shown in the "Selected" strip when the picked option is not in the loaded page. */
  selectedLabel?: string | null;
  /** Called with the tapped option; the sheet then closes itself (onClose). */
  onPick: (option: T) => void;
  onClose: () => void;
  searchPlaceholder?: string;
  /**
   * Caller-owned search (server search or a custom filter): the box is always
   * shown and `options` are used as given. Without it the sheet filters
   * locally once there are more than SEARCH_THRESHOLD options.
   */
  search?: { value: string; onChange: (v: string) => void };
  autoFocusSearch?: boolean;
  loading?: boolean;
  error?: unknown;
  /** Feature-specific text for `error` (default: the shared error catalog). */
  errorMessage?: string;
  onRetry?: () => void;
  /** Next page of a paged list (infinite scroll). */
  onEndReached?: () => void;
  fetchingMore?: boolean;
  emptyText?: string;
  /** Extra content under the list (a note, a "create new" button). */
  footer?: React.ReactNode;
};

/**
 * The one single-choice picker of the app: a page sheet with the current
 * value on top, search for long or server lists, infinite scroll and
 * loading / empty / error states. Replaces lists that open inline and stretch
 * the screen.
 */
export function PickerSheet<T extends PickerOption>(props: PickerSheetProps<T>) {
  const { visible, onClose } = props;
  return (
    <Modal visible={visible} animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      {/* Mounted only while open, so the local search starts empty every time. */}
      {visible ? <SheetBody {...props} /> : null}
    </Modal>
  );
}

function SheetBody<T extends PickerOption>({
  title,
  options,
  selectedKey,
  selectedLabel,
  onPick,
  onClose,
  searchPlaceholder,
  search,
  autoFocusSearch,
  loading,
  error,
  errorMessage,
  onRetry,
  onEndReached,
  fetchingMore,
  emptyText,
  footer,
}: PickerSheetProps<T>) {
  const { t } = useTranslation('common');
  const insets = useSafeAreaInsets();
  const [local, setLocal] = useState('');
  const serverSearch = !!search;
  const showSearch = needsSearch(options.length, serverSearch);
  const shown = useMemo(() => (serverSearch ? [...options] : filterOptions(options, local)), [serverSearch, options, local]);
  const rows = useMemo(() => pickerRows(shown), [shown]);
  const current = selectedLabel ?? options.find((o) => o.key === selectedKey)?.title ?? null;

  const pick = (o: T) => {
    void Haptics.selectionAsync().catch(() => undefined);
    onPick(o);
    onClose();
  };

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
      className="flex-1 bg-background"
      // iOS presents a page sheet below the status bar; Android shows it full screen.
      style={{ paddingTop: Platform.OS === 'ios' ? 0 : insets.top }}>
      <View className="flex-row items-center justify-between gap-2 pb-2 pe-2 ps-5 pt-4">
        <Text variant="title" numberOfLines={1} className="flex-1">
          {title}
        </Text>
        <Button title={t('close')} variant="ghost" onPress={onClose} />
      </View>

      {current ? (
        <View
          accessible
          accessibilityLabel={`${t('picker.selected')}: ${current}`}
          className="mx-4 mb-3 flex-row items-center gap-2 rounded-lg bg-accent px-3 py-2"
          style={{ borderCurve: 'continuous' }}>
          <Icon name="check" tone="primary" size={16} weight="semibold" />
          <Text variant="footnote" tone="muted">
            {t('picker.selected')}
          </Text>
          <Text variant="callout" className="flex-1 font-semibold" numberOfLines={1}>
            {current}
          </Text>
        </View>
      ) : null}

      {showSearch ? (
        <View className="px-4 pb-2">
          <SearchField
            value={search ? search.value : local}
            onChange={search ? search.onChange : setLocal}
            placeholder={searchPlaceholder ?? t('picker.search')}
            autoFocus={autoFocusSearch}
          />
        </View>
      ) : null}

      <FlatList
        data={rows}
        keyExtractor={(r) => r.key}
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        onEndReachedThreshold={0.5}
        onEndReached={onEndReached}
        contentContainerStyle={{ paddingBottom: insets.bottom + 24 }}
        ListEmptyComponent={
          loading ? (
            <View className="gap-2 px-4 pt-2">
              {Array.from({ length: 6 }, (_, i) => (
                <Skeleton key={i} className="h-[52px] rounded-lg" />
              ))}
            </View>
          ) : error ? (
            <View className="items-center gap-3 px-6 py-10">
              <Icon name="error" tone="muted-foreground" />
              <Text variant="callout" tone="muted" className="text-center">
                {errorMessage ?? errorText(error)}
              </Text>
              {onRetry ? <Button title={t('retry')} variant="secondary" onPress={onRetry} /> : null}
            </View>
          ) : (
            <View className="items-center gap-2 px-6 py-10">
              <Icon name="search" tone="muted-foreground" />
              <Text variant="callout" tone="muted" className="text-center">
                {emptyText ?? t('picker.empty')}
              </Text>
            </View>
          )
        }
        ListFooterComponent={
          <>
            {fetchingMore ? <ActivityIndicator className="py-4" /> : null}
            {footer ? <View className="gap-3 px-4 pt-3">{footer}</View> : null}
          </>
        }
        renderItem={({ item: r, index }) => {
          if (r.kind === 'header') {
            return (
              <Text variant="caption" tone="muted" className="px-4 pb-1.5 pt-4 font-semibold">
                {r.title}
              </Text>
            );
          }
          const o = r.option;
          const on = o.key === selectedKey;
          const next = rows[index + 1];
          return (
            <Pressable
              accessibilityRole="button"
              accessibilityState={{ selected: on, disabled: !!o.disabled }}
              disabled={o.disabled}
              onPress={() => pick(o)}
              className={cn(
                'min-h-[52px] flex-row items-center gap-3 px-4 py-2.5 active:bg-muted',
                on && 'bg-accent',
                o.disabled && 'opacity-50',
                next?.kind === 'option' && 'border-b border-border',
              )}>
              {o.icon ? (
                <View className="size-8 items-center justify-center rounded-md bg-secondary">
                  <Icon name={o.icon} tone="primary" size={16} />
                </View>
              ) : null}
              <View className="min-w-0 flex-1 gap-0.5">
                <Text variant="body" numberOfLines={1} className={on ? 'font-semibold' : undefined}>
                  {o.title}
                </Text>
                {o.subtitle ? (
                  <Text variant="footnote" tone="muted" numberOfLines={2}>
                    {o.subtitle}
                  </Text>
                ) : null}
              </View>
              {on ? <Icon name="check" tone="primary" size={18} weight="semibold" /> : null}
            </Pressable>
          );
        }}
      />
    </KeyboardAvoidingView>
  );
}

/**
 * Form row that opens a PickerSheet: label, the chosen value (or a muted
 * placeholder) and a chevron. Keeps the screen short however long the list is.
 */
export function PickerField({
  label,
  value,
  subtitle,
  placeholder,
  icon,
  error,
  disabled,
  onPress,
}: {
  label?: string;
  value?: string | null;
  subtitle?: string | null;
  placeholder: string;
  icon?: IconName;
  error?: string;
  disabled?: boolean;
  onPress: () => void;
}) {
  return (
    <View className="gap-1.5">
      {label ? (
        <Text variant="footnote" tone="muted" className="font-medium">
          {label}
        </Text>
      ) : null}
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={[label, value || placeholder].filter(Boolean).join(', ')}
        disabled={disabled}
        onPress={onPress}
        className={cn(
          'min-h-[52px] flex-row items-center gap-3 rounded-lg border bg-card px-4 py-2.5 active:bg-muted',
          error ? 'border-destructive' : 'border-border',
          disabled && 'opacity-50',
        )}
        style={{ borderCurve: 'continuous' }}>
        {icon ? <Icon name={icon} tone="primary" size={18} /> : null}
        <View className="min-w-0 flex-1 gap-0.5">
          <Text variant="body" tone={value ? 'default' : 'muted'} numberOfLines={1}>
            {value || placeholder}
          </Text>
          {value && subtitle ? (
            <Text variant="footnote" tone="muted" numberOfLines={1}>
              {subtitle}
            </Text>
          ) : null}
        </View>
        <Icon name="chevronDown" tone="muted-foreground" size={16} />
      </Pressable>
      {error ? (
        <Text variant="footnote" tone="danger">
          {error}
        </Text>
      ) : null}
    </View>
  );
}
