import { cva, type VariantProps } from 'class-variance-authority';
import { Text as RNText, type TextProps } from 'react-native';

import { cn } from './cn';
import { resolveFont } from './fonts';

/**
 * Type scale (iOS points) in the web's brand fonts: Outfit for display
 * headings, Plus Jakarta Sans for everything else, `font-mono` for codes.
 * Dynamic Type still scales every variant.
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

const DISPLAY = new Set<TextVariant>(['largeTitle', 'title']);

export function Text({
  variant,
  tone,
  className,
  style,
  ...props
}: TextProps & VariantProps<typeof text> & { className?: string }) {
  const font = resolveFont(
    cn(text({ variant, tone }), className),
    variant && DISPLAY.has(variant) ? 'display' : 'sans',
  );
  return (
    <RNText
      className={font.className}
      style={font.fontFamily ? [{ fontFamily: font.fontFamily }, style] : style}
      {...props}
    />
  );
}
