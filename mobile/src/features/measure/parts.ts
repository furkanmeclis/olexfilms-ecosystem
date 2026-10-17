/**
 * Feature flags of the measuring tab. SmartPTG (BLE) and plate recognition need
 * native modules that Expo Go does not ship; they arrive with the development build.
 */
export const SMARTPTG_BLE_ENABLED = false;
export const PLATE_RECOGNITION_ENABLED = false;

export type Place = 'left' | 'right' | 'top' | 'back';

/** Body parts of the manual form, with the NexPTG place they sit on. */
export const PARTS: { key: string; place: Place }[] = [
  { key: 'front_bumper', place: 'top' },
  { key: 'hood', place: 'top' },
  { key: 'roof', place: 'top' },
  { key: 'trunk', place: 'back' },
  { key: 'rear_bumper', place: 'back' },
  { key: 'front_left_fender', place: 'left' },
  { key: 'front_left_door', place: 'left' },
  { key: 'rear_left_door', place: 'left' },
  { key: 'rear_left_fender', place: 'left' },
  { key: 'front_right_fender', place: 'right' },
  { key: 'front_right_door', place: 'right' },
  { key: 'rear_right_door', place: 'right' },
  { key: 'rear_right_fender', place: 'right' },
];

/** "120 135 4,5" → [120, 135, 4.5]: values separated by spaces, decimal point or comma. */
export function parseValues(text: string): number[] {
  return text
    .split(/[\s;]+/)
    .map((s) => Number(s.replace(',', '.')))
    .filter((n) => Number.isFinite(n) && n > 0);
}
