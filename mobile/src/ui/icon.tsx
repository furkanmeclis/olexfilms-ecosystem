import { createElement } from 'react';
import { Platform } from 'react-native';
import { SymbolView } from 'expo-symbols';
import { useCSSVariable } from 'uniwind';
import { ICONS, lucideFor, type IconName } from '@/ui/icons';

/** `on-media` (token `--color-on-media`) sits over the camera picture, which is never themed. */
export type IconTone =
  | 'foreground'
  | 'muted-foreground'
  | 'primary'
  | 'primary-foreground'
  | 'secondary-foreground'
  | 'destructive'
  | 'brand'
  | 'on-media';

type Props = { name: IconName; tone?: IconTone; size?: number; weight?: 'regular' | 'medium' | 'semibold' };

/** Semantic icon: SF Symbol on iOS, Lucide elsewhere, colored from a theme token. */
export function Icon({ name, tone = 'foreground', size = 22, weight = 'regular' }: Props) {
  const raw = useCSSVariable(`--color-${tone}`);
  const color = typeof raw === 'string' ? raw : undefined;
  const fallback = createElement(lucideFor(name), { size, color, strokeWidth: tone === 'on-media' ? 2 : 1.75 });
  if (Platform.OS !== 'ios') return fallback;
  return <SymbolView name={ICONS[name].sf} size={size} tintColor={color} weight={weight} fallback={fallback} />;
}
