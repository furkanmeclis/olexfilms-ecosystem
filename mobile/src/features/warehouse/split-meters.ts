/** "12,5" → "12.5"; null when not a positive meter value with at most two decimals (TEC-1230). */
export function parseMeters(raw: string): string | null {
  const v = raw.trim().replace(',', '.');
  if (!/^\d+(\.\d{1,2})?$/.test(v) || Number(v) <= 0) return null;
  return v;
}
