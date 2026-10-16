import { cva, type VariantProps } from 'class-variance-authority';
import { Text as RNText, type TextProps } from 'react-native';

import { cn } from './cn';

/**
 * Type scale (iOS points). System font on both platforms: no font loading on
 * launch, and Dynamic Type scales every variant.
 */
const text = cva('text-foreground', {
  variants: {
    variant: {
      largeTitle: 'text-[34px] leading-[41px] font-bold tracking-tight',
      title: 'text-[22px] leading-[28px] font-semibold',
      headline: 'text-[17px] leading-[22px] font-semibold',
      body: 'text-[17px] leading-[22px]',
      callout: 'text-[15px] leading-[20px]',
      footnote: 'text-[13px] leading-[18px]',
      caption: 'text-[11px] leading-[13px] uppercase tracking-wide',
      /** Live measurement value: tabular digits so the row never shifts. */
      reading: 'text-[56px] leading-[60px] font-bold tracking-tight tabular-nums',
    },
    tone: {
      default: '',
      muted: 'text-muted-foreground',
      primary: 'text-primary',
      danger: 'text-destructive',
      onPrimary: 'text-primary-foreground',
    },
  },
  defaultVariants: { variant: 'body', tone: 'default' },
});

export type TextVariant = NonNullable<VariantProps<typeof text>['variant']>;

export function Text({
  variant,
  tone,
  className,
  ...props
}: TextProps & VariantProps<typeof text> & { className?: string }) {
  return <RNText className={cn(text({ variant, tone }), className)} {...props} />;
}
