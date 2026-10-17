import { isLoaded } from 'expo-font';

/**
 * Brand fonts, same families as the web (frontend/src/app/layout.tsx):
 * Plus Jakarta Sans for text, Outfit for display headings, JetBrains Mono for
 * codes. React Native cannot pick a weight from a variable font, so every
 * weight is its own static file (mobile/scripts/build-fonts.py) and its own
 * family name. Text picks the family from the weight class instead of
 * setting fontWeight, which would make Android fake-bold an already bold face.
 */
export const fontAssets = {
  'PlusJakartaSans-Regular': require('../../assets/fonts/PlusJakartaSans-Regular.ttf'),
  'PlusJakartaSans-Medium': require('../../assets/fonts/PlusJakartaSans-Medium.ttf'),
  'PlusJakartaSans-SemiBold': require('../../assets/fonts/PlusJakartaSans-SemiBold.ttf'),
  'PlusJakartaSans-Bold': require('../../assets/fonts/PlusJakartaSans-Bold.ttf'),
  'Outfit-SemiBold': require('../../assets/fonts/Outfit-SemiBold.ttf'),
  'Outfit-Bold': require('../../assets/fonts/Outfit-Bold.ttf'),
  'JetBrainsMono-Regular': require('../../assets/fonts/JetBrainsMono-Regular.ttf'),
  'JetBrainsMono-Medium': require('../../assets/fonts/JetBrainsMono-Medium.ttf'),
} as const;

export type FontFamily = keyof typeof fontAssets;
export type FontKind = 'sans' | 'display' | 'mono';
type Weight = 'normal' | 'medium' | 'semibold' | 'bold' | 'extrabold' | 'black';

const families: Record<FontKind, Record<Weight, FontFamily>> = {
  sans: {
    normal: 'PlusJakartaSans-Regular',
    medium: 'PlusJakartaSans-Medium',
    semibold: 'PlusJakartaSans-SemiBold',
    // The web cuts Jakarta at 700: heavier classes use the boldest face.
    bold: 'PlusJakartaSans-Bold',
    extrabold: 'PlusJakartaSans-Bold',
    black: 'PlusJakartaSans-Bold',
  },
  // Outfit is cut at 500-700 on the web; headings are never lighter than 600.
  display: {
    normal: 'Outfit-SemiBold',
    medium: 'Outfit-SemiBold',
    semibold: 'Outfit-SemiBold',
    bold: 'Outfit-Bold',
    extrabold: 'Outfit-Bold',
    black: 'Outfit-Bold',
  },
  mono: {
    normal: 'JetBrainsMono-Regular',
    medium: 'JetBrainsMono-Medium',
    semibold: 'JetBrainsMono-Medium',
    bold: 'JetBrainsMono-Medium',
    extrabold: 'JetBrainsMono-Medium',
    black: 'JetBrainsMono-Medium',
  },
};

const WEIGHT = /^font-(normal|medium|semibold|bold|extrabold|black)$/;

/** Family for a weight; undefined until the font is loaded (system font then). */
export function fontFamily(kind: FontKind, weight: Weight = 'normal'): FontFamily | undefined {
  const family = families[kind][weight];
  return isLoaded(family) ? family : undefined;
}

/**
 * Resolves `font-mono` / `font-<weight>` classes to a static family. Returns
 * the class list without those classes plus the family, or the input
 * unchanged when the font is not loaded (so the system font keeps the weight).
 */
export function resolveFont(className: string, kind: Exclude<FontKind, 'mono'>) {
  const classes = className.split(/\s+/).filter(Boolean);
  let weight: Weight = 'normal';
  let mono = false;
  const rest: string[] = [];
  for (const c of classes) {
    const m = WEIGHT.exec(c);
    if (m) weight = m[1] as Weight;
    else if (c === 'font-mono') mono = true;
    else rest.push(c);
  }
  const family = fontFamily(mono ? 'mono' : kind, weight);
  if (!family) return { className, fontFamily: undefined };
  return { className: rest.join(' '), fontFamily: family };
}
