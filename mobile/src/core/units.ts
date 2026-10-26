import type { ThicknessUnit } from '@/core/preferences';
import { intlLocale } from './digits.ts';

export const UM_PER_MIL = 25.4;

/** Paint thickness in the user's unit: microns as integers, mils with one decimal. */
export function formatThickness(um: number, unit: ThicknessUnit, locale: string) {
  if (unit === 'mil') {
    const mil = um / UM_PER_MIL;
    return `${new Intl.NumberFormat(intlLocale(locale), { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(mil)} mil`;
  }
  return `${new Intl.NumberFormat(intlLocale(locale), { maximumFractionDigits: 0 }).format(um)} µm`;
}

/** Converts a value typed in the user's unit back to microns for the API. */
export function toMicrons(value: number, unit: ThicknessUnit) {
  return unit === 'mil' ? Math.round(value * UM_PER_MIL) : Math.round(value);
}
