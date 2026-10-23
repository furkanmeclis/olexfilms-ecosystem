import { cva, type VariantProps } from 'class-variance-authority';
import * as Haptics from 'expo-haptics';
import type { ReactElement } from 'react';
import { ActivityIndicator, Pressable, type PressableProps } from 'react-native';

import { cn } from './cn';
import { Icon, type IconTone } from './icon';
import type { IconName } from './icons';
import { Text } from './text';

const button = cva('flex-row items-center justify-center gap-2 rounded-md active:opacity-80', {
  variants: {
    variant: {
      primary: 'bg-primary',
      secondary: 'bg-secondary',
      outline: 'border border-border bg-transparent',
      ghost: 'bg-transparent',
      destructive: 'bg-destructive',
    },
    size: {
      md: 'min-h-11 px-4',
      /** Primary actions: 52 pt, reachable with gloves. */
      lg: 'min-h-[52px] px-5 rounded-lg',
    },
  },
  defaultVariants: { variant: 'primary', size: 'md' },
});

const label = cva('font-semibold', {
  variants: {
    variant: {
      primary: 'text-primary-foreground',
      secondary: 'text-secondary-foreground',
      outline: 'text-primary',
      ghost: 'text-primary',
      destructive: 'text-primary-foreground',
    },
    size: { md: 'text-[15px]', lg: 'text-[17px]' },
  },
  defaultVariants: { variant: 'primary', size: 'md' },
});

const ICON_TONE: Record<NonNullable<VariantProps<typeof button>['variant']>, IconTone> = {
  primary: 'primary-foreground',
  secondary: 'secondary-foreground',
  outline: 'primary',
  ghost: 'primary',
  destructive: 'primary-foreground',
};

type Props = Omit<PressableProps, 'children'> &
  VariantProps<typeof button> & {
    title: string;
    loading?: boolean;
    className?: string;
    /**
     * An icon name from `icons.ts` (drawn in the label color) or a ready element.
     * A bare string that is not an icon name is a type error: RN cannot draw text outside <Text>.
     */
    icon?: IconName | ReactElement;
  };

export function Button({ title, variant, size, loading, disabled, className, icon, onPress, ...props }: Props) {
  const off = disabled || loading;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled: !!off, busy: !!loading }}
      disabled={off}
      // Haptic on touch-down: the press feels instant even before work starts.
      onPressIn={() => void Haptics.selectionAsync()}
      onPress={onPress}
      className={cn(button({ variant, size }), off && 'opacity-50', className)}
      {...props}>
      {loading ? (
        <ActivityIndicator
          colorClassName={
            variant === 'primary' || variant === 'destructive' || !variant ? 'accent-primary-foreground' : 'accent-primary'
          }
        />
      ) : typeof icon === 'string' ? (
        <Icon name={icon} tone={ICON_TONE[variant ?? 'primary']} size={size === 'lg' ? 18 : 17} />
      ) : (
        icon
      )}
      <Text className={label({ variant, size })}>{title}</Text>
    </Pressable>
  );
}
