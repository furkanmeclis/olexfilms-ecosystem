import type { ComponentRef, ReactNode, Ref } from 'react';
import { TextInput, type TextInputProps, View } from 'react-native';

import { cn } from './cn';
import { fontFamily } from './fonts';
import { Icon } from './icon';
import type { IconName } from './icons';
import { Text } from './text';

type Props = TextInputProps & {
  label: string;
  error?: string;
  hint?: string;
  className?: string;
  /** Symbol shown before the label. */
  icon?: IconName;
  /** Control rendered in the same row before the text box (e.g. a country picker). */
  leading?: ReactNode;
  ref?: Ref<ComponentRef<typeof TextInput>>;
};

export function Input({ label, error, hint, className, icon, leading, ref, style, ...props }: Props) {
  const family = fontFamily('sans');
  const field = (
    <TextInput
      ref={ref}
      accessibilityLabel={label}
      placeholderTextColorClassName="accent-muted-foreground"
      className={cn(
        'min-h-[52px] rounded-lg border bg-card px-4 text-[17px] text-foreground',
        error ? 'border-destructive' : 'border-border focus:border-ring',
        leading && 'flex-1',
        className,
      )}
      style={family ? [{ fontFamily: family }, style] : style}
      {...props}
    />
  );
  return (
    <View className="gap-1.5">
      <View className="flex-row items-center gap-1.5">
        {icon ? <Icon name={icon} tone="muted-foreground" size={14} weight="medium" /> : null}
        <Text variant="footnote" tone="muted" className="font-medium">
          {label}
        </Text>
      </View>
      {leading ? (
        <View className="flex-row gap-2">
          {leading}
          {field}
        </View>
      ) : (
        field
      )}
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
