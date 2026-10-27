import { type ComponentRef, type ReactNode, type Ref, useState } from 'react';
import { TextInput, type TextInputProps, View } from 'react-native';

import { toLatinDigits } from '@/core/digits';
import { cn } from '@/ui/cn';
import { fontFamily } from '@/ui/fonts';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

type FieldProps = TextInputProps & {
  label: string;
  error?: string;
  /** Control inside the box before the text (the country picker). */
  leading?: ReactNode;
  /** Control inside the box after the text (show / hide password). */
  trailing?: ReactNode;
  ref?: Ref<ComponentRef<typeof TextInput>>;
};

/** One rounded box with an optional leading control; the border follows focus and errors. */
export function AuthField({ label, error, leading, trailing, ref, style, onChangeText, ...props }: FieldProps) {
  const [focused, setFocused] = useState(false);
  const family = fontFamily('sans');
  return (
    <View className="gap-2">
      <Text variant="footnote" tone="muted" className="font-medium">
        {label}
      </Text>
      <View
        className={cn(
          'min-h-14 flex-row items-center rounded-2xl border-[1.5px] bg-card',
          error ? 'border-destructive' : focused ? 'border-primary' : 'border-border',
        )}
        style={{ borderCurve: 'continuous' }}>
        {leading}
        {leading ? <View className="h-6 w-px bg-border" /> : null}
        <TextInput
          ref={ref}
          accessibilityLabel={label}
          placeholderTextColorClassName="accent-muted-foreground"
          className="min-h-14 flex-1 px-4 text-[17px] text-foreground"
          style={family ? [{ fontFamily: family }, style] : style}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
          // TEC-1009: Arabic-Indic / Persian digits are stored as Latin.
          onChangeText={onChangeText ? (text) => onChangeText(toLatinDigits(text)) : undefined}
          {...props}
        />
        {trailing}
      </View>
      {error ? (
        <View className="flex-row items-center gap-1.5">
          <Icon name="alert" tone="destructive" size={13} weight="medium" />
          <Text variant="footnote" tone="danger" className="shrink">
            {error}
          </Text>
        </View>
      ) : null}
    </View>
  );
}
