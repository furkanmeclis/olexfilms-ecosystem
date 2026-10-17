import { useCSSVariable } from 'uniwind';

/** Brand primary for the native tab bar's selected item (system blue otherwise). */
export function useTabTint(): string | undefined {
  const primary = useCSSVariable('--color-primary');
  return typeof primary === 'string' ? primary : undefined;
}
