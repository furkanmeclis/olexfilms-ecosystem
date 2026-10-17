import { DarkTheme, DefaultTheme, type Theme } from 'expo-router';
import { useCSSVariable, useUniwind } from 'uniwind';

import { fontFamily } from '@/ui/fonts';

/** Native headers, tab bars and sheets use the same tokens as the rest of the UI. */
export function useNavigationTheme(fontsLoaded: boolean): Theme {
  const { theme } = useUniwind();
  const [primary, background, card, text, border] = useCSSVariable([
    '--color-primary',
    '--color-background',
    '--color-card',
    '--color-foreground',
    '--color-border',
  ]);
  const base = theme === 'dark' ? DarkTheme : DefaultTheme;
  const str = (v: unknown, fallback: unknown) => (typeof v === 'string' ? v : (fallback as string));
  // Brand faces where React Navigation draws text itself (Android headers,
  // JS tab bars); iOS 26 Liquid Glass headers keep the system font.
  // `fontsLoaded` is an argument so the compiler recomputes once they register.
  const face = (family: string | undefined, fallback: Theme['fonts']['regular']) =>
    fontsLoaded && family ? { fontFamily: family, fontWeight: 'normal' as const } : fallback;
  return {
    ...base,
    fonts: {
      regular: face(fontFamily('sans'), base.fonts.regular),
      medium: face(fontFamily('sans', 'medium'), base.fonts.medium),
      bold: face(fontFamily('sans', 'semibold'), base.fonts.bold),
      heavy: face(fontFamily('display', 'bold'), base.fonts.heavy),
    },
    colors: {
      ...base.colors,
      primary: str(primary, base.colors.primary),
      background: str(background, base.colors.background),
      card: str(card, base.colors.card),
      text: str(text, base.colors.text),
      border: str(border, base.colors.border),
    },
  };
}
