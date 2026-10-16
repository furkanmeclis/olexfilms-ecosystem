import { DarkTheme, DefaultTheme, type Theme } from 'expo-router';
import { useCSSVariable, useUniwind } from 'uniwind';

/** Native headers, tab bars and sheets use the same tokens as the rest of the UI. */
export function useNavigationTheme(): Theme {
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
  return {
    ...base,
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
