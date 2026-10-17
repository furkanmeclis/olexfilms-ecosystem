import { Platform } from 'react-native';
import { SymbolView } from 'expo-symbols';
import { useCSSVariable } from 'uniwind';
import { ICONS, lucideFor, type IconName, type LucideIcon } from '@/ui/icons';

type Tone = 'foreground' | 'muted-foreground' | 'primary' | 'primary-foreground' | 'destructive' | 'brand';

type Props = { tone?: Tone; size?: number; weight?: 'regular' | 'medium' | 'semibold' } & (
  | { name: IconName; as?: undefined }
  /** Transitional: removed once all call sites use `name` (Task 2). */
  | { as: LucideIcon; name?: undefined }
);

/** Semantic icon: SF Symbol on iOS, Lucide elsewhere, colored from a theme token. */
export function Icon({ name, as, tone = 'foreground', size = 22, weight = 'regular' }: Props) {
  const raw = useCSSVariable(`--color-${tone}`);
  const color = typeof raw === 'string' ? raw : undefined;
  const lucide = (name ? lucideFor(name) : as) as LucideIcon;
  const Glyph = lucide;
  const fallback = <Glyph size={size} color={color} strokeWidth={1.75} />;
  if (Platform.OS !== 'ios' || !name) return fallback;
  return <SymbolView name={ICONS[name].sf} size={size} tintColor={color} weight={weight} fallback={fallback} />;
}
