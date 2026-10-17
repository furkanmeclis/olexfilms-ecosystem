import { cva, type VariantProps } from 'class-variance-authority';
import { Children, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
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
      /** Uppercased in JS (see `upper`): CSS uppercase ignores the locale (Turkish i → İ). */
      caption: 'text-[11px] leading-[13px] tracking-wide',
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

/** Locale-aware uppercase for string children (RN textTransform is not). */
function upper(children: ReactNode, lang: string): ReactNode {
  return Children.map(children, (c) => (typeof c === 'string' ? c.toLocaleUpperCase(lang) : c));
}

export function Text({
  variant,
  tone,
  className,
  style,
  children,
  ...props
}: TextProps & VariantProps<typeof text> & { className?: string }) {
  const { i18n } = useTranslation();
  const font = resolveFont(
    cn(text({ variant, tone }), className),
    variant && DISPLAY.has(variant) ? 'display' : 'sans',
  );
  return (
    <RNText
      className={font.className}
      style={font.fontFamily ? [{ fontFamily: font.fontFamily }, style] : style}
      {...props}
    >
      {variant === 'caption' ? upper(children, i18n.language) : children}
    </RNText>
  );
}
