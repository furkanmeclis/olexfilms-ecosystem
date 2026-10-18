import type { components } from '@/core/api/schema';
import type { StatusTone } from '@/ui/status-pill';

export type Appointment = components['schemas']['Appointment'];
export type AppointmentStatus = components['schemas']['AppointmentStatus'];
export type AvailabilityDay = components['schemas']['AppointmentAvailabilityDay'];

/** Next status a booking can move to (same rules as the web calendar). */
export function nextStatuses(status: AppointmentStatus): AppointmentStatus[] {
  if (status === 'scheduled') return ['confirmed', 'no_show', 'cancelled'];
  if (status === 'confirmed') return ['arrived', 'no_show', 'cancelled'];
  return [];
}

/** Only scheduled or confirmed bookings move. */
export const canReschedule = (a: Pick<Appointment, 'status'>) => a.status === 'scheduled' || a.status === 'confirmed';

/** Intake opens once the customer has arrived, the booking has a vehicle and no draft is linked. */
export const canStartIntake = (a: Appointment) => a.status === 'arrived' && a.vehicle_id != null && !a.service_uuid;

export function statusTone(s: AppointmentStatus): StatusTone {
  if (s === 'arrived') return 'done';
  if (s === 'cancelled' || s === 'no_show') return 'problem';
  if (s === 'confirmed') return 'progress';
  return 'neutral';
}

const pad = (n: number) => String(n).padStart(2, '0');

/** Local calendar day as YYYY-MM-DD. */
export function ymd(d: Date) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** Local midnight of a YYYY-MM-DD day. */
export function parseDay(day: string) {
  const [y, m, d] = day.split('-').map(Number);
  return new Date(y!, m! - 1, d!);
}

export function addDays(d: Date, n: number) {
  const out = new Date(d);
  out.setDate(out.getDate() + n);
  return out;
}

/** Monday of the week holding `d`, at local midnight. */
export function startOfWeek(d: Date) {
  const out = new Date(d.getFullYear(), d.getMonth(), d.getDate());
  out.setDate(out.getDate() - ((out.getDay() + 6) % 7));
  return out;
}

/** Errors the booking endpoints answer with, mapped to our own keys. */
export function bookingErrorKey(code: string | undefined) {
  switch (code) {
    case 'APPOINTMENT_CAPACITY_FULL':
      return 'error_capacity_full';
    case 'APPOINTMENT_DAY_CLOSED':
      return 'error_day_closed';
    case 'APPOINTMENT_INVALID_TRANSITION':
      return 'error_invalid_transition';
    case 'APPOINTMENT_INTAKE_ALREADY_STARTED':
      return 'error_intake_started';
    default:
      return null;
  }
}
