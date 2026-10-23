import * as Haptics from 'expo-haptics';
import { Pressable, TextInput, View } from 'react-native';

import { Text } from '@/ui/text';

/** Whole-number stepper (fixed-barcode quantities); large targets for gloves. */
export function QtyStepper({
  label,
  value,
  onChange,
  min = 1,
  max = 99999,
}: {
  label: string;
  value: number;
  onChange: (n: number) => void;
  min?: number;
  max?: number;
}) {
  const set = (n: number) => {
    const v = Math.max(min, Math.min(max, Math.round(n) || min));
    if (v !== value) void Haptics.selectionAsync();
    onChange(v);
  };
  return (
    <View className="flex-row items-center gap-2 rounded-lg bg-card p-2">
      <Text variant="footnote" tone="muted" className="flex-1 ps-2 font-medium">
        {label}
      </Text>
      <Step label="−" onPress={() => set(value - 1)} />
      <TextInput
        accessibilityLabel={label}
        value={String(value)}
        placeholder={String(min)}
        placeholderTextColorClassName="accent-muted-foreground"
        onChangeText={(s) => set(Number(s.replace(/\D/g, '')))}
        keyboardType="number-pad"
        selectTextOnFocus
        className="min-h-11 w-16 rounded-md border border-border text-center text-[17px] font-semibold text-foreground"
      />
      <Step label="+" onPress={() => set(value + 1)} />
    </View>
  );
}

function Step({ label, onPress }: { label: string; onPress: () => void }) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      onPress={onPress}
      className="size-11 items-center justify-center rounded-md bg-secondary active:opacity-70">
      <Text variant="title">{label}</Text>
    </Pressable>
  );
}
