import { useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';

import { addDays, type Appointment, ymd } from './lib';

const PAGE = 100;
const MAX_PAGES = 5;

/** Every appointment starting in [from, to), paged by the list contract. */
export async function loadRange(from: Date, to: Date) {
  const items: Appointment[] = [];
  for (let page = 0; page < MAX_PAGES; page++) {
    const res = await unwrap(
      api.GET('/v1/appointments', {
        params: { query: { from: from.toISOString(), to: to.toISOString(), limit: PAGE, offset: page * PAGE } },
      }),
    );
    items.push(...res.items);
    if (items.length >= res.total || res.items.length < PAGE) break;
  }
  return items.sort((a, b) => a.starts_at.localeCompare(b.starts_at));
}

const DAY_MS = 86_400_000;

/** One booking by uuid. The API has no single-get, so a wide window around today is read from the network. */
export async function findAppointment(uuid: string) {
  const now = Date.now();
  const all = await loadRange(new Date(now - 90 * DAY_MS), new Date(now + 365 * DAY_MS));
  return all.find((a) => a.uuid === uuid) ?? null;
}

export const AVAILABILITY_DAYS = 14;

/** Free times of the next two weeks; each day also carries the organization's time zone. */
export function useAvailability() {
  const from = ymd(new Date());
  const to = ymd(addDays(new Date(), AVAILABILITY_DAYS - 1));
  return useQuery({
    queryKey: ['appointments', 'availability', from],
    queryFn: () => unwrap(api.GET('/v1/appointments/availability', { params: { query: { from, to } } })),
  });
}

/** Time zone of the organization (from the availability days), if known. */
export function useOrgTimeZone() {
  return useAvailability().data?.find((d) => d.timezone)?.timezone;
}
