import type { ComponentRef, Ref } from 'react';
import { TextInput, type TextInputProps, View } from 'react-native';

import { cn } from './cn';
import { fontFamily } from './fonts';
import { Text } from './text';

type Props = TextInputProps & {
  label: string;
  error?: string;
  hint?: string;
  className?: string;
  ref?: Ref<ComponentRef<typeof TextInput>>;
};

export function Input({ label, error, hint, className, ref, style, ...props }: Props) {
  const family = fontFamily('sans');
  return (
    <View className="gap-1.5">
      <Text variant="footnote" tone="muted" className="font-medium">
        {label}
      </Text>
      <TextInput
        ref={ref}
        accessibilityLabel={label}
        placeholderTextColorClassName="accent-muted-foreground"
        className={cn(
          'min-h-[52px] rounded-lg border bg-card px-4 text-[17px] text-foreground',
          error ? 'border-destructive' : 'border-border focus:border-ring',
          className,
        )}
        style={family ? [{ fontFamily: family }, style] : style}
        {...props}
      />
      {error ? (
        <Text variant="footnote" tone="danger">
          {error}
        </Text>
      ) : hint ? (
        <Text variant="footnote" tone="muted">
          {hint}
        </Text>
      ) : null}
    </View>
  );
}
