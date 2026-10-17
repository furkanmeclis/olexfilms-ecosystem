import type { LucideIcon } from '@/ui/icons';
import { useCSSVariable } from 'uniwind';

type Tone = 'foreground' | 'muted-foreground' | 'primary' | 'primary-foreground' | 'destructive' | 'brand';

/** Lucide icon (same set as the web) colored from a theme token. */
export function Icon({ as: Glyph, tone = 'foreground', size = 22 }: { as: LucideIcon; tone?: Tone; size?: number }) {
  const color = useCSSVariable(`--color-${tone}`);
  return <Glyph size={size} color={typeof color === 'string' ? color : undefined} strokeWidth={2} />;
}
