import * as Haptics from 'expo-haptics';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal, Pressable, ScrollView, View } from 'react-native';

import { Check, ChevronDown, Warehouse as WarehouseIcon } from '@/ui/icons';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { List, ListRow } from '@/ui/list';
import { Text } from '@/ui/text';

import type { Warehouse } from './api';
import { useCurrentWarehouse } from './context';

/** Sheet listing the active warehouses; picking one remembers it on this device. */
export function WarehousePickerSheet({
  visible,
  onClose,
  value,
  onPick,
  title,
  exclude,
}: {
  visible: boolean;
  onClose: () => void;
  value?: string;
  onPick: (w: Warehouse) => void;
  title?: string;
  exclude?: string;
}) {
  const { t } = useTranslation('warehouse');
  const { warehouses } = useCurrentWarehouse();
  const list = warehouses.filter((w) => w.uuid !== exclude);
  return (
    <Modal visible={visible} animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <View className="flex-1 bg-background">
        <View className="flex-row items-center justify-between px-4 pb-2 pt-5">
          <Text variant="title">{title ?? t('m.pick_warehouse')}</Text>
          <Button title={t('common:close')} variant="ghost" onPress={onClose} />
        </View>
        <ScrollView contentContainerClassName="px-4 pb-10">
          {list.length === 0 ? (
            <EmptyState icon={WarehouseIcon} title={t('locations.no_warehouses')} />
          ) : (
            <List>
              {list.map((w, i) => (
                <ListRow
                  key={w.uuid}
                  title={w.name}
                  subtitle={[w.code, w.address].filter(Boolean).join(' · ')}
                  last={i === list.length - 1}
                  onPress={() => {
                    void Haptics.selectionAsync();
                    onPick(w);
                    onClose();
                  }}
                  trailing={w.uuid === value ? <Icon as={Check} tone="primary" size={20} /> : <View />}
                />
              ))}
            </List>
          )}
        </ScrollView>
      </View>
    </Modal>
  );
}

/** Header chip with the current warehouse code; opens the picker. */
export function WarehouseChip() {
  const { t } = useTranslation('warehouse');
  const { warehouse, warehouses, select } = useCurrentWarehouse();
  const [open, setOpen] = useState(false);
  if (!warehouse) return null;
  return (
    <>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={t('m.current_warehouse', { name: warehouse.name })}
        hitSlop={8}
        disabled={warehouses.length < 2}
        onPress={() => setOpen(true)}
        className="flex-row items-center gap-1 rounded-full bg-secondary px-2.5 py-1 active:opacity-70">
        <Icon as={WarehouseIcon} tone="primary" size={14} />
        <Text variant="footnote" className="max-w-28 font-semibold" numberOfLines={1}>
          {warehouse.code}
        </Text>
        {warehouses.length > 1 ? <Icon as={ChevronDown} tone="muted-foreground" size={14} /> : null}
      </Pressable>
      <WarehousePickerSheet visible={open} onClose={() => setOpen(false)} value={warehouse.uuid} onPick={(w) => select(w.uuid)} />
    </>
  );
}

/** Form field that opens the picker (create transfer / count / entry). */
export function WarehouseField({
  label,
  value,
  onChange,
  exclude,
  error,
}: {
  label: string;
  value?: string;
  onChange: (w: Warehouse) => void;
  exclude?: string;
  error?: string;
}) {
  const { t } = useTranslation('warehouse');
  const { warehouses } = useCurrentWarehouse();
  const [open, setOpen] = useState(false);
  const current = warehouses.find((w) => w.uuid === value);
  return (
    <View className="gap-1.5">
      <Text variant="footnote" tone="muted" className="font-medium">
        {label}
      </Text>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={label}
        onPress={() => setOpen(true)}
        className={`min-h-[52px] flex-row items-center gap-3 rounded-lg border bg-card px-4 active:opacity-80 ${error ? 'border-destructive' : 'border-border'}`}>
        <Icon as={WarehouseIcon} tone="primary" size={18} />
        <Text variant="body" className="flex-1" numberOfLines={1} tone={current ? 'default' : 'muted'}>
          {current ? `${current.name} (${current.code})` : t('entries.pick_warehouse')}
        </Text>
        <Icon as={ChevronDown} tone="muted-foreground" size={18} />
      </Pressable>
      {error ? (
        <Text variant="footnote" tone="danger">
          {error}
        </Text>
      ) : null}
      <WarehousePickerSheet
        visible={open}
        onClose={() => setOpen(false)}
        value={value}
        exclude={exclude}
        title={label}
        onPick={onChange}
      />
    </View>
  );
}
