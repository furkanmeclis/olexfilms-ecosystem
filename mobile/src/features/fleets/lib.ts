import type { components } from '@/core/api/schema';
import type { StatusTone } from '@/ui/status-pill';

type S = components['schemas'];
export type FleetListItem = S['FleetListItem'];
export type FleetCard = S['FleetCard'];
export type PanelFleetVehicle = S['FleetVehicle'];
export type PanelFleetUser = S['FleetUser'];
export type PanelFleetReport = S['FleetReport'];
export type FleetStatement = S['FleetStatement'];
export type FleetReportKind = PanelFleetReport['period_kind'];

export const FLEET_LINK_FILTERS = ['all', 'active', 'pending', 'ended'] as const;
export type FleetLinkFilter = (typeof FLEET_LINK_FILTERS)[number];

export function fleetUserTone(status: PanelFleetUser['status']): StatusTone {
  return status === 'active' ? 'done' : 'neutral';
}

export function fleetReportTone(status: PanelFleetReport['status']): StatusTone {
  if (status === 'ready') return 'done';
  if (status === 'failed') return 'problem';
  return 'progress';
}

const pad = (n: number) => String(n).padStart(2, '0');

/** Last closed month (`YYYY-MM`) and quarter (`YYYY-Qn`): the default report periods (web: lastClosedPeriods). */
export function lastClosedPeriods(today: Date) {
  const y = today.getFullYear();
  const m = today.getMonth();
  const prev = new Date(y, m - 1, 1);
  const q = Math.floor(m / 3);
  return {
    month: `${prev.getFullYear()}-${pad(prev.getMonth() + 1)}`,
    quarter: q === 0 ? `${y - 1}-Q4` : `${y}-Q${q}`,
  };
}

/** The `count` closed periods before `today`, newest first. */
export function closedPeriods(kind: FleetReportKind, today: Date, count = 6): string[] {
  const out: string[] = [];
  if (kind === 'monthly') {
    for (let i = 1; i <= count; i++) {
      const d = new Date(today.getFullYear(), today.getMonth() - i, 1);
      out.push(`${d.getFullYear()}-${pad(d.getMonth() + 1)}`);
    }
    return out;
  }
  let y = today.getFullYear();
  let q = Math.floor(today.getMonth() / 3);
  for (let i = 0; i < count; i++) {
    if (q === 0) {
      y -= 1;
      q = 4;
    }
    out.push(`${y}-Q${q}`);
    q -= 1;
  }
  return out;
}

/** `YYYY-MM` of a date (local calendar). */
export function monthOf(d: Date) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}`;
}

/** Moves a `YYYY-MM` month by `delta` months. */
export function shiftMonth(month: string, delta: number) {
  const [y, m] = month.split('-').map(Number);
  return monthOf(new Date(y!, (m ?? 1) - 1 + delta, 1));
}

/**
 * Statement period of a month: the first day to the last one, or to today
 * for the current month (web default: the current month up to today).
 */
export function monthRange(month: string, today: Date) {
  const [y, m] = month.split('-').map(Number);
  const last = new Date(y!, m!, 0);
  const end = monthOf(today) === month ? today : last;
  return { from: `${month}-01`, to: `${month}-${pad(end.getDate())}` };
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Invite form: an e-mail and a first name are required. */
export function inviteErrors(v: { email: string; name: string }) {
  const errors: ('email' | 'name')[] = [];
  if (!EMAIL_RE.test(v.email.trim())) errors.push('email');
  if (!v.name.trim()) errors.push('name');
  return errors;
}
