import { Platform } from 'react-native';
import { SymbolView } from 'expo-symbols';
import { useCSSVariable } from 'uniwind';
import { ICONS, lucideGlyph, type IconName } from '@/ui/icons';

type Tone = 'foreground' | 'muted-foreground' | 'primary' | 'primary-foreground' | 'destructive' | 'brand';

type Props = { name: IconName; tone?: Tone; size?: number; weight?: 'regular' | 'medium' | 'semibold' };

/** Semantic icon: SF Symbol on iOS, Lucide elsewhere, colored from a theme token. */
export function Icon({ name, tone = 'foreground', size = 22, weight = 'regular' }: Props) {
  const raw = useCSSVariable(`--color-${tone}`);
  const color = typeof raw === 'string' ? raw : undefined;
  const fallback = lucideGlyph(name, { size, color, strokeWidth: 1.75 });
  if (Platform.OS !== 'ios') return fallback;
  return <SymbolView name={ICONS[name].sf} size={size} tintColor={color} weight={weight} fallback={fallback} />;
}
